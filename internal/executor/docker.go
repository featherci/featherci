package executor

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// dockerClient is a narrow interface over the Docker SDK client,
// covering only the methods the executor needs. This enables mock-based testing.
type dockerClient interface {
	ImageInspectWithRaw(ctx context.Context, imageID string) (image.InspectResponse, []byte, error)
	ImagePull(ctx context.Context, refStr string, options image.PullOptions) (io.ReadCloser, error)
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *ocispec.Platform, containerName string) (container.CreateResponse, error)
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerWait(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error)
	ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error)
	ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error
	NetworkCreate(ctx context.Context, name string, options network.CreateOptions) (network.CreateResponse, error)
	NetworkRemove(ctx context.Context, networkID string) error
}

// DockerExecutor runs build steps inside Docker containers.
type DockerExecutor struct {
	client dockerClient
	paths  *pathMapper // translates container paths to host paths for DinD
}

// NewDockerExecutor creates an executor using the Docker client from environment
// configuration (DOCKER_HOST, etc.).
// It auto-detects whether FeatherCI is running inside Docker and sets up
// bind mount path translation for the sibling container pattern.
func NewDockerExecutor() (*DockerExecutor, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("creating docker client: %w", err)
	}
	return &DockerExecutor{
		client: cli,
		paths:  detectPathMapper(cli),
	}, nil
}

// Run executes commands inside a new container and waits for completion.
func (d *DockerExecutor) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
	if err := d.pullImage(ctx, opts.Image); err != nil {
		return nil, fmt.Errorf("pulling image %s: %w", opts.Image, err)
	}

	// Set up Docker network and service containers if services are configured.
	var networkID string
	var serviceIDs []string
	if len(opts.Services) > 0 {
		var err error
		networkID, serviceIDs, err = d.startServices(ctx, opts.Services)
		if err != nil {
			d.cleanupServices(networkID, serviceIDs)
			return nil, fmt.Errorf("starting service containers: %w", err)
		}
	}

	// Wrap commands in a single shell invocation.
	// Use newline separation with set -e instead of && chaining so that
	// multi-line commands (from YAML literal blocks) are preserved correctly.
	// When bash is available, use a login shell so profile scripts (nvm, rvm,
	// pyenv, etc.) are sourced — matching CircleCI cimg image behavior.
	// Falls back to /bin/sh for minimal images like Alpine.
	innerScript := "set -e\n" + strings.Join(opts.Commands, "\n")
	// The outer /bin/sh checks for bash and re-execs as a login shell.
	// Single quotes around the heredoc delimiter prevent variable expansion.
	// We explicitly source ~/.bashrc because login shells only source
	// ~/.bash_profile or ~/.profile, and some images (e.g. cimg/*) set up
	// tools like nvm/rvm/pyenv exclusively in ~/.bashrc.
	bashSetup := "if [ -f ~/.bashrc ]; then . ~/.bashrc; fi\n"
	shellCmd := "if [ -x /bin/bash ]; then exec /bin/bash -l <<'FEATHERCI_SCRIPT'\n" +
		bashSetup + innerScript + "\nFEATHERCI_SCRIPT\nelse\n" + innerScript + "\nfi"
	entrypoint := []string{"/bin/sh", "-c", shellCmd}

	config := &container.Config{
		Image:      opts.Image,
		Entrypoint: entrypoint,
		Env:        mapToEnvSlice(opts.Env),
		WorkingDir: opts.WorkDir,
	}

	hostConfig := &container.HostConfig{
		Binds: d.resolveBindMounts(opts.BindMounts),
	}
	if opts.Memory > 0 {
		hostConfig.Resources.Memory = opts.Memory
	}
	if opts.CPUs > 0 {
		hostConfig.Resources.NanoCPUs = int64(opts.CPUs * 1e9)
	}

	// Attach main container to service network if services exist.
	var networkingConfig *network.NetworkingConfig
	if networkID != "" {
		networkingConfig = &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				networkID: {},
			},
		}
	}

	resp, err := d.client.ContainerCreate(ctx, config, hostConfig, networkingConfig, nil, "")
	if err != nil {
		d.cleanupServices(networkID, serviceIDs)
		return nil, fmt.Errorf("creating container: %w", err)
	}
	containerID := resp.ID

	// Ensure cleanup regardless of outcome.
	defer func() {
		rmCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = d.client.ContainerRemove(rmCtx, containerID, container.RemoveOptions{Force: true})
		d.cleanupServices(networkID, serviceIDs)
	}()

	startedAt := time.Now()
	if err := d.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("starting container: %w", err)
	}

	// Stream container output to the provided writer.
	// Use context.Background() so the log stream isn't interrupted if the
	// caller's context is cancelled — the WaitGroup controls the goroutine
	// lifecycle, and Docker closes the stream on container exit.
	var logWg sync.WaitGroup
	var logBytes int64
	if opts.Output != nil {
		logReader, err := d.client.ContainerLogs(context.Background(), containerID, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
		})
		if err == nil {
			logWg.Add(1)
			go func() {
				defer logWg.Done()
				defer logReader.Close()
				// StdCopy demuxes Docker's multiplexed stdout/stderr stream.
				n, _ := stdcopy.StdCopy(opts.Output, opts.Output, logReader)
				atomic.StoreInt64(&logBytes, n)
			}()
		} else {
			slog.Warn("failed to attach container logs", "container", containerID, "error", err)
		}
	}

	// Apply timeout if specified.
	waitCtx := ctx
	var cancelTimeout context.CancelFunc
	if opts.Timeout > 0 {
		waitCtx, cancelTimeout = context.WithTimeout(ctx, opts.Timeout)
		defer cancelTimeout()
	}

	waitCh, errCh := d.client.ContainerWait(waitCtx, containerID, container.WaitConditionNotRunning)
	select {
	case waitResp := <-waitCh:
		finishedAt := time.Now()
		result := &RunResult{
			ContainerID: containerID,
			ExitCode:    int(waitResp.StatusCode),
			StartedAt:   startedAt,
			FinishedAt:  finishedAt,
		}
		if waitResp.Error != nil && strings.Contains(waitResp.Error.Message, "OOM") {
			result.OOMKilled = true
		}
		logWg.Wait()

		// Fallback: if streaming captured nothing, fetch logs in batch.
		// This handles edge cases where Follow-mode misses output on
		// fast-exiting containers.
		if opts.Output != nil && atomic.LoadInt64(&logBytes) == 0 {
			d.fetchLogsSync(containerID, opts.Output)
		}

		return result, nil

	case err := <-errCh:
		// Timeout or other wait error — stop the container.
		logWg.Wait()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		_ = d.client.ContainerStop(stopCtx, containerID, container.StopOptions{})

		if opts.Output != nil {
			d.fetchLogsSync(containerID, opts.Output)
		}

		if waitCtx.Err() == context.DeadlineExceeded {
			return &RunResult{
				ContainerID: containerID,
				ExitCode:    -1,
				StartedAt:   startedAt,
				FinishedAt:  time.Now(),
			}, fmt.Errorf("step timed out after %s", opts.Timeout)
		}
		return nil, fmt.Errorf("waiting for container: %w", err)
	}
}

// serviceHostname extracts a hostname from a Docker image reference.
// "mysql:8" → "mysql", "redis/redis-stack:latest" → "redis-stack"
func serviceHostname(img string) string {
	// Strip tag
	name := img
	if idx := strings.LastIndex(name, ":"); idx != -1 {
		name = name[:idx]
	}
	// Use last path component
	if idx := strings.LastIndex(name, "/"); idx != -1 {
		name = name[idx+1:]
	}
	return name
}

// startServices creates a Docker network and starts all service containers on it.
// Returns the network ID, list of service container IDs, and any error.
func (d *DockerExecutor) startServices(ctx context.Context, services []ServiceOption) (string, []string, error) {
	// Create a unique network for this step execution.
	netName := fmt.Sprintf("featherci-%d", time.Now().UnixNano())
	netResp, err := d.client.NetworkCreate(ctx, netName, network.CreateOptions{Driver: "bridge"})
	if err != nil {
		return "", nil, fmt.Errorf("creating network: %w", err)
	}
	networkID := netResp.ID

	var containerIDs []string
	for _, svc := range services {
		if err := d.pullImage(ctx, svc.Image); err != nil {
			return networkID, containerIDs, fmt.Errorf("pulling service image %s: %w", svc.Image, err)
		}

		hostname := serviceHostname(svc.Image)

		svcConfig := &container.Config{
			Image:    svc.Image,
			Env:      mapToEnvSlice(svc.Env),
			Hostname: hostname,
		}

		svcNetConfig := &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				networkID: {
					Aliases: []string{hostname},
				},
			},
		}

		resp, err := d.client.ContainerCreate(ctx, svcConfig, nil, svcNetConfig, nil, "")
		if err != nil {
			return networkID, containerIDs, fmt.Errorf("creating service container %s: %w", svc.Image, err)
		}
		containerIDs = append(containerIDs, resp.ID)

		if err := d.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
			return networkID, containerIDs, fmt.Errorf("starting service container %s: %w", svc.Image, err)
		}
	}

	return networkID, containerIDs, nil
}

// cleanupServices stops and removes service containers and the network.
func (d *DockerExecutor) cleanupServices(networkID string, containerIDs []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, id := range containerIDs {
		_ = d.client.ContainerStop(ctx, id, container.StopOptions{Timeout: intPtr(5)})
		_ = d.client.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
	}

	if networkID != "" {
		_ = d.client.NetworkRemove(ctx, networkID)
	}
}

// resolveBindMounts translates bind mount source paths through the path mapper
// (for Docker-in-Docker) and formats them as Docker bind strings.
func (d *DockerExecutor) resolveBindMounts(mounts []BindMount) []string {
	if d.paths == nil {
		return formatBindMounts(mounts)
	}
	resolved := make([]BindMount, len(mounts))
	for i, m := range mounts {
		resolved[i] = BindMount{
			Source:   d.paths.Map(m.Source),
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		}
	}
	return formatBindMounts(resolved)
}

// fetchLogsSync fetches all container logs without Follow (batch mode).
// Used as a fallback when streaming didn't capture any output.
func (d *DockerExecutor) fetchLogsSync(containerID string, output io.Writer) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	logReader, err := d.client.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		slog.Warn("fallback log fetch failed", "container", containerID, "error", err)
		return
	}
	defer logReader.Close()
	_, _ = stdcopy.StdCopy(output, output, logReader)
}

// Stop gracefully stops a running container with a 10-second grace period.
func (d *DockerExecutor) Stop(ctx context.Context, containerID string) error {
	return d.client.ContainerStop(ctx, containerID, container.StopOptions{
		Timeout: intPtr(10),
	})
}

// pullImage pulls the image if it is not already present locally.
func (d *DockerExecutor) pullImage(ctx context.Context, img string) error {
	_, _, err := d.client.ImageInspectWithRaw(ctx, img)
	if err == nil {
		return nil // already present
	}

	reader, err := d.client.ImagePull(ctx, img, image.PullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()
	// Consume the pull output to completion.
	_, err = io.Copy(io.Discard, reader)
	return err
}

func intPtr(v int) *int { return &v }
