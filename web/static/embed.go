// Package static provides embedded static assets for the web UI.
package static

import "embed"

// Files contains all static assets (CSS, JS, images) embedded at compile time.
// These files are served by the web server and include:
// - css/main.css: Compiled Tailwind CSS
// - js/htmx.min.js: HTMX library for dynamic interactions
// - images/*: Logo and other static images
//
//go:embed css js images
var Files embed.FS
