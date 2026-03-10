// Package templates provides embedded HTML templates for the web UI.
package templates

import "embed"

// Files contains all HTML templates embedded at compile time.
// Templates are organized into:
// - layouts/: Base page layouts
// - components/: Reusable UI components
// - pages/: Full page templates (including subdirectories)
//
//go:embed layouts/*.html components/*.html pages/*.html pages/**/*.html
var Files embed.FS
