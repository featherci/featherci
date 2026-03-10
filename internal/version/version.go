// Package version provides build version information.
package version

import (
	"fmt"
	"runtime"
)

// These variables are set at build time via -ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Info returns formatted version information.
func Info() string {
	return fmt.Sprintf("FeatherCI %s (%s) built %s with %s",
		Version, Commit, BuildDate, runtime.Version())
}

// Short returns just the version string.
func Short() string {
	return Version
}
