package executor

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// mockDockerClient implements the dockerClient interface for testing.
type mockDockerClient struct {
	imageInspectFn    func(ctx context.Context, imageID string) (image.InspectResponse, []byte, error)
	imagePullFn       func(ctx context.Context, refStr string, options image.PullOptions) (io.ReadCloser, error)
	containerCreateFn func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *ocispec.Platform, containerName string) (container.CreateResponse, error)
	containerStartFn  func(ctx context.Context, containerID string, options container.StartOptions) error
	containerWaitFn   func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error)
	containerLogsFn   func(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error)
	containerStopFn   func(ctx context.Context, containerID string, options container.StopOptions) error
	containerRemoveFn func(ctx context.Context, containerID string, options container.RemoveOptions) error
	networkCreateFn   func(ctx context.Context, name string, options network.CreateOptions) (network.CreateResponse, error)
	networkRemoveFn   func(ctx context.Context, networkID string) error
}

func (m *mockDockerClient) ImageInspectWithRaw(ctx context.Context, imageID string) (image.InspectResponse, []byte, error) {
	if m.imageInspectFn != nil {
		return m.imageInspectFn(ctx, imageID)
	}
	return image.InspectResponse{}, nil, nil
}

func (m *mockDockerClient) ImagePull(ctx context.Context, refStr string, options image.PullOptions) (io.ReadCloser, error) {
	if m.imagePullFn != nil {
		return m.imagePullFn(ctx, refStr, options)
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func (m *mockDockerClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *ocispec.Platform, containerName string) (container.CreateResponse, error) {
	if m.containerCreateFn != nil {
		return m.containerCreateFn(ctx, config, hostConfig, networkingConfig, platform, containerName)
	}
	return container.CreateResponse{ID: "test-container-123"}, nil
}

func (m *mockDockerClient) ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error {
	if m.containerStartFn != nil {
		return m.containerStartFn(ctx, containerID, options)
	}
	return nil
}

func (m *mockDockerClient) ContainerWait(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
	if m.containerWaitFn != nil {
		return m.containerWaitFn(ctx, containerID, condition)
	}
	waitCh := make(chan container.WaitResponse, 1)
	waitCh <- container.WaitResponse{StatusCode: 0}
	errCh := make(chan error, 1)
	return waitCh, errCh
}

func (m *mockDockerClient) ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	if m.containerLogsFn != nil {
		return m.containerLogsFn(ctx, containerID, options)
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func (m *mockDockerClient) ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error {
	if m.containerStopFn != nil {
		return m.containerStopFn(ctx, containerID, options)
	}
	return nil
}

func (m *mockDockerClient) ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error {
	if m.containerRemoveFn != nil {
		return m.containerRemoveFn(ctx, containerID, options)
	}
	return nil
}

func (m *mockDockerClient) NetworkCreate(ctx context.Context, name string, options network.CreateOptions) (network.CreateResponse, error) {
	if m.networkCreateFn != nil {
		return m.networkCreateFn(ctx, name, options)
	}
	return network.CreateResponse{ID: "net-" + name}, nil
}

func (m *mockDockerClient) NetworkRemove(ctx context.Context, networkID string) error {
	if m.networkRemoveFn != nil {
		return m.networkRemoveFn(ctx, networkID)
	}
	return nil
}

func TestDockerExecutor_Run_Success(t *testing.T) {
	mock := &mockDockerClient{}
	exec := &DockerExecutor{client: mock}

	result, err := exec.Run(context.Background(), RunOptions{
		Image:    "golang:1.22",
		Commands: []string{"go test ./..."},
		Env:      map[string]string{"CI": "true"},
		WorkDir:  "/workspace",
		BindMounts: []BindMount{
			{Source: "/tmp/build", Target: "/workspace"},
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.ContainerID != "test-container-123" {
		t.Errorf("expected container ID test-container-123, got %s", result.ContainerID)
	}
	if result.StartedAt.IsZero() || result.FinishedAt.IsZero() {
		t.Error("expected non-zero timestamps")
	}
}

func TestDockerExecutor_Run_ContainerConfig(t *testing.T) {
	var capturedConfig *container.Config
	var capturedHostConfig *container.HostConfig

	mock := &mockDockerClient{
		containerCreateFn: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, _ *network.NetworkingConfig, _ *ocispec.Platform, _ string) (container.CreateResponse, error) {
			capturedConfig = config
			capturedHostConfig = hostConfig
			return container.CreateResponse{ID: "cfg-test"}, nil
		},
	}
	exec := &DockerExecutor{client: mock}

	_, err := exec.Run(context.Background(), RunOptions{
		Image:    "node:20",
		Commands: []string{"npm install", "npm test"},
		Env:      map[string]string{"NODE_ENV": "test", "CI": "true"},
		WorkDir:  "/app",
		BindMounts: []BindMount{
			{Source: "/src", Target: "/app"},
			{Source: "/cache", Target: "/cache", ReadOnly: true},
		},
		Memory: 512 * 1024 * 1024, // 512MB
		CPUs:   1.5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check image.
	if capturedConfig.Image != "node:20" {
		t.Errorf("expected image node:20, got %s", capturedConfig.Image)
	}

	// Check entrypoint wraps commands with shell.
	expectedCmd := "/bin/sh"
	if capturedConfig.Entrypoint[0] != expectedCmd {
		t.Errorf("expected entrypoint[0] %s, got %s", expectedCmd, capturedConfig.Entrypoint[0])
	}
	if !strings.Contains(capturedConfig.Entrypoint[2], "npm install && npm test") {
		t.Errorf("expected commands joined with &&, got %s", capturedConfig.Entrypoint[2])
	}

	// Check env (sorted).
	if len(capturedConfig.Env) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(capturedConfig.Env))
	}
	if capturedConfig.Env[0] != "CI=true" {
		t.Errorf("expected first env CI=true, got %s", capturedConfig.Env[0])
	}

	// Check working dir.
	if capturedConfig.WorkingDir != "/app" {
		t.Errorf("expected workdir /app, got %s", capturedConfig.WorkingDir)
	}

	// Check bind mounts.
	if len(capturedHostConfig.Binds) != 2 {
		t.Fatalf("expected 2 bind mounts, got %d", len(capturedHostConfig.Binds))
	}
	if capturedHostConfig.Binds[0] != "/src:/app" {
		t.Errorf("expected bind /src:/app, got %s", capturedHostConfig.Binds[0])
	}
	if capturedHostConfig.Binds[1] != "/cache:/cache:ro" {
		t.Errorf("expected bind /cache:/cache:ro, got %s", capturedHostConfig.Binds[1])
	}

	// Check resource limits.
	if capturedHostConfig.Resources.Memory != 512*1024*1024 {
		t.Errorf("expected 512MB memory, got %d", capturedHostConfig.Resources.Memory)
	}
	if capturedHostConfig.Resources.NanoCPUs != 1500000000 {
		t.Errorf("expected 1.5 CPU in nanoCPUs, got %d", capturedHostConfig.Resources.NanoCPUs)
	}
}

func TestDockerExecutor_Run_ImagePull(t *testing.T) {
	pullCalled := false
	mock := &mockDockerClient{
		imageInspectFn: func(ctx context.Context, imageID string) (image.InspectResponse, []byte, error) {
			return image.InspectResponse{}, nil, errors.New("not found")
		},
		imagePullFn: func(ctx context.Context, refStr string, options image.PullOptions) (io.ReadCloser, error) {
			pullCalled = true
			if refStr != "alpine:latest" {
				t.Errorf("expected pull alpine:latest, got %s", refStr)
			}
			return io.NopCloser(strings.NewReader(`{"status":"pulling"}`)), nil
		},
	}
	exec := &DockerExecutor{client: mock}

	_, err := exec.Run(context.Background(), RunOptions{
		Image:    "alpine:latest",
		Commands: []string{"echo hello"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pullCalled {
		t.Error("expected image pull to be called")
	}
}

func TestDockerExecutor_Run_NonZeroExit(t *testing.T) {
	mock := &mockDockerClient{
		containerWaitFn: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
			waitCh := make(chan container.WaitResponse, 1)
			waitCh <- container.WaitResponse{StatusCode: 1}
			return waitCh, make(chan error, 1)
		},
	}
	exec := &DockerExecutor{client: mock}

	result, err := exec.Run(context.Background(), RunOptions{
		Image:    "alpine",
		Commands: []string{"false"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", result.ExitCode)
	}
}

func TestDockerExecutor_Run_CreateError(t *testing.T) {
	mock := &mockDockerClient{
		containerCreateFn: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, _ *network.NetworkingConfig, _ *ocispec.Platform, _ string) (container.CreateResponse, error) {
			return container.CreateResponse{}, errors.New("disk full")
		},
	}
	exec := &DockerExecutor{client: mock}

	_, err := exec.Run(context.Background(), RunOptions{
		Image:    "alpine",
		Commands: []string{"echo hi"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "creating container") {
		t.Errorf("expected creating container error, got: %v", err)
	}
}

func TestDockerExecutor_Run_OOMKilled(t *testing.T) {
	mock := &mockDockerClient{
		containerWaitFn: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
			waitCh := make(chan container.WaitResponse, 1)
			waitCh <- container.WaitResponse{
				StatusCode: 137,
				Error:      &container.WaitExitError{Message: "OOM killed"},
			}
			return waitCh, make(chan error, 1)
		},
	}
	exec := &DockerExecutor{client: mock}

	result, err := exec.Run(context.Background(), RunOptions{
		Image:    "alpine",
		Commands: []string{"stress --vm 1"},
		Memory:   64 * 1024 * 1024,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.OOMKilled {
		t.Error("expected OOMKilled to be true")
	}
	if result.ExitCode != 137 {
		t.Errorf("expected exit code 137, got %d", result.ExitCode)
	}
}

func TestDockerExecutor_Stop(t *testing.T) {
	stopCalled := false
	mock := &mockDockerClient{
		containerStopFn: func(ctx context.Context, containerID string, options container.StopOptions) error {
			stopCalled = true
			if containerID != "abc123" {
				t.Errorf("expected container abc123, got %s", containerID)
			}
			if options.Timeout == nil || *options.Timeout != 10 {
				t.Error("expected 10 second timeout")
			}
			return nil
		},
	}
	exec := &DockerExecutor{client: mock}

	err := exec.Stop(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !stopCalled {
		t.Error("expected stop to be called")
	}
}

func TestMapToEnvSlice(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want []string
	}{
		{"nil", nil, nil},
		{"empty", map[string]string{}, nil},
		{"single", map[string]string{"A": "1"}, []string{"A=1"}},
		{"sorted", map[string]string{"Z": "3", "A": "1", "M": "2"}, []string{"A=1", "M=2", "Z=3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapToEnvSlice(tt.env)
			if len(got) != len(tt.want) {
				t.Fatalf("len: got %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d]=%s, want %s", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestFormatBindMounts(t *testing.T) {
	mounts := []BindMount{
		{Source: "/host/src", Target: "/container/src"},
		{Source: "/host/cache", Target: "/cache", ReadOnly: true},
	}
	got := formatBindMounts(mounts)
	if len(got) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(got))
	}
	if got[0] != "/host/src:/container/src" {
		t.Errorf("got %s", got[0])
	}
	if got[1] != "/host/cache:/cache:ro" {
		t.Errorf("got %s", got[1])
	}
}

func TestDockerExecutor_Run_OutputCapture(t *testing.T) {
	var buf bytes.Buffer
	mock := &mockDockerClient{
		containerLogsFn: func(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
			if !options.ShowStdout || !options.ShowStderr || !options.Follow {
				t.Error("expected ShowStdout, ShowStderr, and Follow to be true")
			}
			// Docker multiplexed format: 8 byte header per frame
			// header[0] = stream type (1=stdout, 2=stderr)
			// header[4:8] = uint32 big-endian payload size
			var data bytes.Buffer
			payload := []byte("hello from container\n")
			header := make([]byte, 8)
			header[0] = 1 // stdout
			binary.BigEndian.PutUint32(header[4:], uint32(len(payload)))
			data.Write(header)
			data.Write(payload)
			return io.NopCloser(&data), nil
		},
	}
	exec := &DockerExecutor{client: mock}

	result, err := exec.Run(context.Background(), RunOptions{
		Image:    "alpine",
		Commands: []string{"echo hello"},
		Output:   &buf,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	// Give goroutine time to finish
	time.Sleep(50 * time.Millisecond)
	if !strings.Contains(buf.String(), "hello from container") {
		t.Errorf("expected output to contain 'hello from container', got: %q", buf.String())
	}
}

func TestDockerExecutor_Run_Timeout(t *testing.T) {
	mock := &mockDockerClient{
		containerWaitFn: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
			waitCh := make(chan container.WaitResponse)
			errCh := make(chan error, 1)
			go func() {
				<-ctx.Done()
				errCh <- ctx.Err()
			}()
			return waitCh, errCh
		},
	}
	exec := &DockerExecutor{client: mock}

	_, err := exec.Run(context.Background(), RunOptions{
		Image:    "alpine",
		Commands: []string{"sleep 3600"},
		Timeout:  50 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout message, got: %v", err)
	}
}

func TestServiceHostname(t *testing.T) {
	tests := []struct {
		image string
		want  string
	}{
		{"mysql:8", "mysql"},
		{"redis:7-alpine", "redis"},
		{"postgres", "postgres"},
		{"circleci/postgres:14", "postgres"},
		{"ghcr.io/org/custom-db:latest", "custom-db"},
	}
	for _, tt := range tests {
		got := serviceHostname(tt.image)
		if got != tt.want {
			t.Errorf("serviceHostname(%q) = %q, want %q", tt.image, got, tt.want)
		}
	}
}

func TestDockerExecutor_Run_WithServices(t *testing.T) {
	var createdContainers []struct {
		config     *container.Config
		netConfig  *network.NetworkingConfig
	}
	var startedIDs []string
	var networkCreated bool
	var networkName string

	mock := &mockDockerClient{
		networkCreateFn: func(ctx context.Context, name string, options network.CreateOptions) (network.CreateResponse, error) {
			networkCreated = true
			networkName = name
			return network.CreateResponse{ID: "net-123"}, nil
		},
		containerCreateFn: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, netConfig *network.NetworkingConfig, platform *ocispec.Platform, containerName string) (container.CreateResponse, error) {
			createdContainers = append(createdContainers, struct {
				config    *container.Config
				netConfig *network.NetworkingConfig
			}{config, netConfig})
			id := fmt.Sprintf("container-%d", len(createdContainers))
			return container.CreateResponse{ID: id}, nil
		},
		containerStartFn: func(ctx context.Context, containerID string, options container.StartOptions) error {
			startedIDs = append(startedIDs, containerID)
			return nil
		},
	}
	exec := &DockerExecutor{client: mock}

	result, err := exec.Run(context.Background(), RunOptions{
		Image:    "ruby:3.4",
		Commands: []string{"bundle exec rspec"},
		Services: []ServiceOption{
			{Image: "mysql:8.0", Env: map[string]string{"MYSQL_ROOT_PASSWORD": "test"}},
			{Image: "redis:7"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}

	// Network should be created
	if !networkCreated {
		t.Error("expected network to be created")
	}
	if !strings.HasPrefix(networkName, "featherci-") {
		t.Errorf("expected network name to start with featherci-, got %s", networkName)
	}

	// Should have created 3 containers: 2 services + 1 main
	if len(createdContainers) != 3 {
		t.Fatalf("expected 3 containers created, got %d", len(createdContainers))
	}

	// First container: mysql service
	mysqlConfig := createdContainers[0].config
	if mysqlConfig.Image != "mysql:8.0" {
		t.Errorf("expected mysql:8.0, got %s", mysqlConfig.Image)
	}
	if mysqlConfig.Hostname != "mysql" {
		t.Errorf("expected hostname mysql, got %s", mysqlConfig.Hostname)
	}

	// Second container: redis service
	redisConfig := createdContainers[1].config
	if redisConfig.Image != "redis:7" {
		t.Errorf("expected redis:7, got %s", redisConfig.Image)
	}

	// Third container: main container (should be on the network)
	mainNetConfig := createdContainers[2].netConfig
	if mainNetConfig == nil || mainNetConfig.EndpointsConfig["net-123"] == nil {
		t.Error("expected main container to be attached to service network")
	}

	// All 3 containers should be started (2 services + 1 main)
	if len(startedIDs) != 3 {
		t.Errorf("expected 3 containers started, got %d", len(startedIDs))
	}
}
