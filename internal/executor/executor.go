// Package executor provides a Docker-based execution engine for running
// build pipeline steps inside containers.
//
// # Deployment
//
// When deploying FeatherCI itself inside a Docker container on a VM,
// you must map the host's Docker socket into the container so the executor
// can create sibling containers:
//
//	docker run -v /var/run/docker.sock:/var/run/docker.sock featherci/featherci
//
// The executor uses the DOCKER_HOST environment variable (defaulting to
// the unix socket at /var/run/docker.sock). Ensure the container user
// has permission to access the socket (typically by adding it to the
// "docker" group or running as root).
//
// For Docker-in-Docker (dind) alternatives, you can instead run a
// dedicated Docker daemon inside the container, but socket mapping
// is simpler and recommended for most deployments.
package executor

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"
)

// Executor runs build commands inside containers.
type Executor interface {
	Run(ctx context.Context, opts RunOptions) (*RunResult, error)
	Stop(ctx context.Context, containerID string) error
}

// RunOptions configures a container execution.
type RunOptions struct {
	Image      string
	Commands   []string
	Env        map[string]string
	WorkDir    string
	BindMounts []BindMount
	Memory     int64   // bytes, 0 = unlimited
	CPUs       float64 // 0 = unlimited
	Timeout    time.Duration
	Output     io.Writer // container stdout/stderr are streamed here during execution
	Services   []ServiceOption
}

// ServiceOption configures a sidecar container to run alongside the main step container.
// The service is accessible from the main container via its hostname (derived from the image name).
type ServiceOption struct {
	Image string
	Env   map[string]string
}

// BindMount maps a host path into the container.
type BindMount struct {
	Source   string
	Target   string
	ReadOnly bool
}

// RunResult captures the outcome of a container execution.
type RunResult struct {
	ContainerID string
	ExitCode    int
	StartedAt   time.Time
	FinishedAt  time.Time
	OOMKilled   bool
}

// mapToEnvSlice converts a map of environment variables to Docker's KEY=VALUE slice format.
// Keys are sorted for deterministic output.
func mapToEnvSlice(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(env))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%s=%s", k, env[k]))
	}
	return out
}

// formatBindMounts converts BindMount structs to Docker bind mount strings.
func formatBindMounts(mounts []BindMount) []string {
	out := make([]string, 0, len(mounts))
	for _, m := range mounts {
		s := fmt.Sprintf("%s:%s", m.Source, m.Target)
		if m.ReadOnly {
			s += ":ro"
		}
		out = append(out, s)
	}
	return out
}
