package executor

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
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
}

// NewDockerExecutor creates an executor using the Docker client from environment
// configuration (DOCKER_HOST, etc.).
func NewDockerExecutor() (*DockerExecutor, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("creating docker client: %w", err)
	}
	return &DockerExecutor{client: cli}, nil
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
	shellCmd := strings.Join(opts.Commands, " && ")
	entrypoint := []string{"/bin/sh", "-c", shellCmd}

	config := &container.Config{
		Image:      opts.Image,
		Entrypoint: entrypoint,
		Env:        mapToEnvSlice(opts.Env),
		WorkingDir: opts.WorkDir,
	}

	hostConfig := &container.HostConfig{
		Binds: formatBindMounts(opts.BindMounts),
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
	var logWg sync.WaitGroup
	if opts.Output != nil {
		logReader, err := d.client.ContainerLogs(ctx, containerID, container.LogsOptions{
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
				_, _ = stdcopy.StdCopy(opts.Output, opts.Output, logReader)
			}()
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
		return result, nil

	case err := <-errCh:
		// Timeout or other wait error — stop the container.
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		_ = d.client.ContainerStop(stopCtx, containerID, container.StopOptions{})

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
