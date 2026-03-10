---
model: opus
---

# Step 08: HTML Templates and Base Layout

## Objective
Create the HTML template system with a modern, GitHub-like design and base layout.

## Tasks

### 8.1 Set Up Template Embedding
Create `web/templates/embed.go`:
```go
package templates

import "embed"

//go:embed *.html **/*.html
var Files embed.FS
```

### 8.2 Create Template Engine
```go
type Engine struct {
    templates *template.Template
    funcs     template.FuncMap
}

func NewEngine() (*Engine, error)
func (e *Engine) Render(w io.Writer, name string, data any) error

// Template functions
func templateFuncs() template.FuncMap {
    return template.FuncMap{
        "timeAgo":     formatTimeAgo,
        "formatTime":  formatTime,
        "duration":    formatDuration,
        "statusColor": statusToColor,
        "statusIcon":  statusToIcon,
        "json":        toJSON,
        "safeHTML":    safeHTML,
        "truncate":    truncateString,
        "add":         func(a, b int) int { return a + b },
    }
}
```

### 8.3 Create Base Layout
Create `web/templates/layouts/base.html`:
```html
<!DOCTYPE html>
<html lang="en" class="h-full bg-gray-50">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{block "title" .}}FeatherCI{{end}}</title>
    <link rel="stylesheet" href="/static/css/main.css">
    <script src="/static/js/htmx.min.js"></script>
</head>
<body class="h-full">
    {{template "nav" .}}
    <main class="py-10">
        <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
            {{block "content" .}}{{end}}
        </div>
    </main>
    {{template "footer" .}}
</body>
</html>
```

### 8.4 Create Navigation Component
Create `web/templates/components/nav.html`:
```html
{{define "nav"}}
<nav class="bg-gray-900">
    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div class="flex h-16 items-center justify-between">
            <div class="flex items-center">
                <a href="/" class="flex-shrink-0">
                    <span class="text-white text-xl font-bold">FeatherCI</span>
                </a>
                {{if .User}}
                <div class="ml-10 flex items-baseline space-x-4">
                    <a href="/projects" class="text-gray-300 hover:bg-gray-700 hover:text-white rounded-md px-3 py-2 text-sm font-medium">
                        Projects
                    </a>
                    {{if .User.IsAdmin}}
                    <a href="/admin/users" class="text-gray-300 hover:bg-gray-700 hover:text-white rounded-md px-3 py-2 text-sm font-medium">
                        Admin
                    </a>
                    {{end}}
                </div>
                {{end}}
            </div>
            <div class="flex items-center">
                {{if .User}}
                <div class="relative ml-3">
                    <div class="flex items-center space-x-3">
                        <img class="h-8 w-8 rounded-full" src="{{.User.AvatarURL}}" alt="{{.User.Username}}">
                        <span class="text-gray-300 text-sm">{{.User.Username}}</span>
                        <form action="/auth/logout" method="POST">
                            <button type="submit" class="text-gray-400 hover:text-white text-sm">
                                Sign out
                            </button>
                        </form>
                    </div>
                </div>
                {{else}}
                <div class="flex space-x-2">
                    {{range .Providers}}
                    <a href="/auth/{{.}}" class="btn btn-primary text-sm">
                        Sign in with {{. | title}}
                    </a>
                    {{end}}
                </div>
                {{end}}
            </div>
        </div>
    </div>
</nav>
{{end}}
```

### 8.5 Create Footer Component
Create `web/templates/components/footer.html`:
```html
{{define "footer"}}
<footer class="bg-white border-t border-gray-200 mt-auto">
    <div class="max-w-7xl mx-auto py-6 px-4 sm:px-6 lg:px-8">
        <p class="text-center text-gray-500 text-sm">
            FeatherCI - Lightweight CI/CD
        </p>
    </div>
</footer>
{{end}}
```

### 8.6 Create Common Components
Create status badges, buttons, cards:

`web/templates/components/status-badge.html`:
```html
{{define "status-badge"}}
{{$status := .}}
<span class="badge {{if eq $status "success"}}badge-success{{else if eq $status "failure"}}badge-failure{{else if eq $status "running"}}badge-running{{else}}badge-pending{{end}}">
    {{if eq $status "success"}}
        <svg class="w-3 h-3 mr-1" fill="currentColor" viewBox="0 0 20 20">
            <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd"/>
        </svg>
    {{else if eq $status "failure"}}
        <svg class="w-3 h-3 mr-1" fill="currentColor" viewBox="0 0 20 20">
            <path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd"/>
        </svg>
    {{else if eq $status "running"}}
        <svg class="w-3 h-3 mr-1 animate-spin" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
        </svg>
    {{else}}
        <svg class="w-3 h-3 mr-1" fill="currentColor" viewBox="0 0 20 20">
            <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm1-12a1 1 0 10-2 0v4a1 1 0 00.293.707l2.828 2.829a1 1 0 101.415-1.415L11 9.586V6z" clip-rule="evenodd"/>
        </svg>
    {{end}}
    {{$status}}
</span>
{{end}}
```

### 8.7 Create Login Page
`web/templates/pages/login.html`:
```html
{{template "base" .}}

{{define "title"}}Sign In - FeatherCI{{end}}

{{define "content"}}
<div class="min-h-full flex items-center justify-center py-12 px-4 sm:px-6 lg:px-8">
    <div class="max-w-md w-full space-y-8">
        <div>
            <h2 class="mt-6 text-center text-3xl font-extrabold text-gray-900">
                Sign in to FeatherCI
            </h2>
            <p class="mt-2 text-center text-sm text-gray-600">
                Use your code hosting account to sign in
            </p>
        </div>
        <div class="mt-8 space-y-4">
            {{if .GitHubEnabled}}
            <a href="/auth/github" class="w-full flex justify-center py-3 px-4 border border-gray-300 rounded-md shadow-sm bg-white text-sm font-medium text-gray-700 hover:bg-gray-50">
                <svg class="w-5 h-5 mr-2" fill="currentColor" viewBox="0 0 24 24">
                    <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z"/>
                </svg>
                Continue with GitHub
            </a>
            {{end}}
            {{if .GitLabEnabled}}
            <a href="/auth/gitlab" class="w-full flex justify-center py-3 px-4 border border-gray-300 rounded-md shadow-sm bg-white text-sm font-medium text-gray-700 hover:bg-gray-50">
                <!-- GitLab SVG icon -->
                Continue with GitLab
            </a>
            {{end}}
            {{if .GiteaEnabled}}
            <a href="/auth/gitea" class="w-full flex justify-center py-3 px-4 border border-gray-300 rounded-md shadow-sm bg-white text-sm font-medium text-gray-700 hover:bg-gray-50">
                <!-- Gitea SVG icon -->
                Continue with Gitea
            </a>
            {{end}}
        </div>
    </div>
</div>
{{end}}
```

### 8.8 Create Dashboard Page
`web/templates/pages/dashboard.html` - Shows recent builds across all user's projects.

## Deliverables
- [ ] `web/templates/embed.go` - Template embedding
- [ ] `internal/templates/engine.go` - Template engine
- [ ] `web/templates/layouts/base.html` - Base layout
- [ ] `web/templates/components/*.html` - Reusable components
- [ ] `web/templates/pages/login.html` - Login page
- [ ] `web/templates/pages/dashboard.html` - Dashboard
- [ ] Templates render correctly with live data

## Dependencies
- Step 06: Web server
- Step 07: Tailwind CSS

## Estimated Effort
Medium - UI foundation
