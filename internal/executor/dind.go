package executor

import (
	"context"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
)

// mountMapping represents a path mapping from a container mount point to
// its source on the Docker host.
type mountMapping struct {
	containerPath string // e.g. "/data"
	hostPath      string // e.g. "/var/lib/docker/volumes/featherci-data/_data"
}

// pathMapper translates container-internal paths to Docker host paths.
// This is needed when FeatherCI runs inside Docker (sibling container
// pattern): bind mount sources must reference HOST paths, but the
// workspace paths known to FeatherCI are container-internal paths.
type pathMapper struct {
	mappings []mountMapping
}

// Map translates a container-internal path to the corresponding host path.
// If no mount matches, the path is returned unchanged.
func (pm *pathMapper) Map(containerPath string) string {
	if pm == nil {
		return containerPath
	}
	for _, m := range pm.mappings {
		if containerPath == m.containerPath || strings.HasPrefix(containerPath, m.containerPath+"/") {
			return m.hostPath + containerPath[len(m.containerPath):]
		}
	}
	return containerPath
}

// detectPathMapper checks if FeatherCI is running inside a Docker container.
// If so, it inspects the container's mounts to build a path translation table
// so that bind mount sources reference host paths instead of container paths.
//
// Returns nil if not running in Docker or if detection fails (non-fatal).
func detectPathMapper(cli *client.Client) *pathMapper {
	// Check for Docker container marker file.
	if _, err := os.Stat("/.dockerenv"); err != nil {
		return nil
	}

	// The container hostname is typically the short container ID.
	hostname, err := os.Hostname()
	if err != nil {
		slog.Debug("dind: could not get hostname", "error", err)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info, err := cli.ContainerInspect(ctx, hostname)
	if err != nil {
		slog.Debug("dind: could not inspect own container", "hostname", hostname, "error", err)
		return nil
	}

	var mappings []mountMapping
	for _, m := range info.Mounts {
		// Only map volume and bind mounts that have a host source path.
		if m.Type != mount.TypeVolume && m.Type != mount.TypeBind {
			continue
		}
		if m.Source == "" || m.Destination == "" {
			continue
		}
		mappings = append(mappings, mountMapping{
			containerPath: m.Destination,
			hostPath:      m.Source,
		})
	}

	if len(mappings) == 0 {
		return nil
	}

	// Sort longest prefix first for correct matching.
	sort.Slice(mappings, func(i, j int) bool {
		return len(mappings[i].containerPath) > len(mappings[j].containerPath)
	})

	slog.Info("dind: detected Docker-in-Docker, translating bind mount paths",
		"mappings", len(mappings))

	return &pathMapper{mappings: mappings}
}
