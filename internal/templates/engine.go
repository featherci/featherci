// Package templates provides the HTML template engine for the web UI.
package templates

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"strings"
	"time"

	"github.com/featherci/featherci/internal/graph"
	"github.com/featherci/featherci/internal/models"
	"github.com/featherci/featherci/internal/notify"
	"github.com/featherci/featherci/internal/version"
	webtemplates "github.com/featherci/featherci/web/templates"
)

// Engine handles HTML template rendering.
type Engine struct {
	base      *template.Template
	pages     map[string]*template.Template
	funcs     template.FuncMap
	templates *template.Template
}

// New creates a new template engine, parsing all templates from the embedded filesystem.
func New() (*Engine, error) {
	funcs := templateFuncs()

	// Parse all non-page templates (layouts, components) into a base template
	base := template.New("").Funcs(funcs)

	// First pass: load layouts and components
	err := fs.WalkDir(webtemplates.Files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		// Skip page templates on first pass
		if strings.HasPrefix(path, "pages/") {
			return nil
		}

		content, err := fs.ReadFile(webtemplates.Files, path)
		if err != nil {
			return fmt.Errorf("reading template %s: %w", path, err)
		}

		_, err = base.New(path).Parse(string(content))
		if err != nil {
			return fmt.Errorf("parsing template %s: %w", path, err)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("loading base templates: %w", err)
	}

	// Second pass: create a cloned template for each page
	pages := make(map[string]*template.Template)

	err = fs.WalkDir(webtemplates.Files, "pages", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}

		content, err := fs.ReadFile(webtemplates.Files, path)
		if err != nil {
			return fmt.Errorf("reading template %s: %w", path, err)
		}

		// Clone the base template so each page has its own definitions
		pageTemplate, err := base.Clone()
		if err != nil {
			return fmt.Errorf("cloning base for %s: %w", path, err)
		}

		_, err = pageTemplate.New(path).Parse(string(content))
		if err != nil {
			return fmt.Errorf("parsing template %s: %w", path, err)
		}

		pages[path] = pageTemplate
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("loading page templates: %w", err)
	}

	return &Engine{
		base:  base,
		pages: pages,
		funcs: funcs,
	}, nil
}

// Render executes the named template with the given data.
func (e *Engine) Render(w io.Writer, name string, data any) error {
	// Look up the page template
	pageTemplate, ok := e.pages[name]
	if !ok {
		return fmt.Errorf("template not found: %s", name)
	}

	// Execute the page template - it will invoke "base" which includes the page's definitions
	return pageTemplate.ExecuteTemplate(w, name, data)
}

// RenderComponent renders a named define block from the base templates (layouts/components).
// Used for HTMX fragment responses that return partial HTML.
func (e *Engine) RenderComponent(w io.Writer, name string, data any) error {
	return e.base.ExecuteTemplate(w, name, data)
}

// templateFuncs returns the template function map.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		// Time formatting
		"timeAgo":    formatTimeAgo,
		"formatTime": formatTime,
		"duration":   formatDuration,

		// String manipulation
		"title":    strings.Title,
		"lower":    strings.ToLower,
		"upper":    strings.ToUpper,
		"truncate": truncateString,
		"join":     strings.Join,

		// Build status helpers
		"statusColor": statusToColor,
		"statusIcon":  statusToIcon,

		// Data conversion
		"json":     toJSON,
		"safeHTML": safeHTML,

		// Math
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"mul": func(a, b int) int { return a * b },

		// Conditionals
		"eq": func(a, b any) bool { return a == b },
		"ne": func(a, b any) bool { return a != b },

		// Pointer helpers
		"deref": derefString,
		"derefInt64": func(p *int64) int64 {
			if p == nil {
				return 0
			}
			return *p
		},

		// Pipeline graph
		"pipelineGraph": func(steps []*models.BuildStep) *graph.Layout {
			return graph.Calculate(steps)
		},
		"edgePath": func(e graph.Edge) template.HTML {
			return template.HTML(graph.EdgePath(e)) //nolint:gosec // trusted internal content
		},

		// Version helpers
		"version":       version.Short,
		"versionCommit": func() string { return version.Commit },

		// Notification helpers
		"channelTypeLabel": notify.ChannelTypeLabel,

		// Iteration helpers
		"seq": func(n int) []int {
			s := make([]int, n)
			for i := range s {
				s[i] = i
			}
			return s
		},
	}
}

// formatTimeAgo returns a human-readable relative time string.
func formatTimeAgo(t any) string {
	var tm time.Time
	switch v := t.(type) {
	case time.Time:
		tm = v
	case *time.Time:
		if v == nil {
			return "never"
		}
		tm = *v
	default:
		return "never"
	}

	if tm.IsZero() {
		return "never"
	}

	d := time.Since(tm)

	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "yesterday"
		}
		return fmt.Sprintf("%d days ago", days)
	case d < 30*24*time.Hour:
		weeks := int(d.Hours() / 24 / 7)
		if weeks == 1 {
			return "1 week ago"
		}
		return fmt.Sprintf("%d weeks ago", weeks)
	case d < 365*24*time.Hour:
		months := int(d.Hours() / 24 / 30)
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	default:
		years := int(d.Hours() / 24 / 365)
		if years == 1 {
			return "1 year ago"
		}
		return fmt.Sprintf("%d years ago", years)
	}
}

// formatTime formats a time in a standard format.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("Jan 2, 2006 at 3:04 PM")
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return "<1s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		if secs == 0 {
			return fmt.Sprintf("%dm", mins)
		}
		return fmt.Sprintf("%dm %ds", mins, secs)
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", hours, mins)
}

// statusToColor returns a Tailwind color class for a build status.
func statusToColor(status string) string {
	switch strings.ToLower(status) {
	case "success", "passed":
		return "green"
	case "failure", "failed", "error":
		return "red"
	case "running", "in_progress":
		return "blue"
	case "pending", "queued":
		return "yellow"
	case "cancelled", "canceled", "skipped":
		return "gray"
	case "waiting", "waiting_approval":
		return "purple"
	default:
		return "gray"
	}
}

// statusToIcon returns an icon name for a build status.
func statusToIcon(status string) string {
	switch strings.ToLower(status) {
	case "success", "passed":
		return "check"
	case "failure", "failed", "error":
		return "x"
	case "running", "in_progress":
		return "spinner"
	case "pending", "queued":
		return "clock"
	case "cancelled", "canceled", "skipped":
		return "slash"
	case "waiting", "waiting_approval":
		return "pause"
	default:
		return "question"
	}
}

// toJSON converts a value to a JSON string.
func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// safeHTML marks a string as safe HTML that should not be escaped.
func safeHTML(s string) template.HTML {
	return template.HTML(s) //nolint:gosec // intentional for trusted content
}

// truncateString truncates a string to the given length, adding "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// derefString safely dereferences a *string, returning empty string if nil.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
