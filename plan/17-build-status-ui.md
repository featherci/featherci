---
model: opus
---

# Step 17: Build Status UI

## Objective
Create the build listing and status pages with real-time updates.

## Tasks

### 17.1 Create Build List Template
`web/templates/pages/builds/list.html`:
```html
{{template "base" .}}

{{define "title"}}Builds - {{.Project.FullName}} - FeatherCI{{end}}

{{define "content"}}
<div class="space-y-6">
    <div class="flex justify-between items-center">
        <h1 class="text-2xl font-bold text-gray-900">
            {{.Project.FullName}}
        </h1>
        <form action="/projects/{{.Project.Namespace}}/{{.Project.Name}}/builds" method="POST">
            <button type="submit" class="btn btn-primary">
                Trigger Build
            </button>
        </form>
    </div>
    
    <div class="card divide-y divide-gray-200">
        {{range .Builds}}
        <a href="/projects/{{$.Project.Namespace}}/{{$.Project.Name}}/builds/{{.BuildNumber}}" 
           class="block hover:bg-gray-50 p-4">
            <div class="flex items-center justify-between">
                <div class="flex items-center space-x-4">
                    {{template "status-icon" .Status}}
                    <div>
                        <p class="text-sm font-medium text-gray-900">
                            #{{.BuildNumber}} - {{.CommitMessage | truncate 60}}
                        </p>
                        <p class="text-sm text-gray-500">
                            {{.Branch}} - {{.CommitSHA | truncate 7}} by {{.CommitAuthor}}
                        </p>
                    </div>
                </div>
                <div class="text-right">
                    <p class="text-sm text-gray-500">
                        {{.CreatedAt | timeAgo}}
                    </p>
                    {{if .FinishedAt}}
                    <p class="text-xs text-gray-400">
                        Duration: {{duration .StartedAt .FinishedAt}}
                    </p>
                    {{end}}
                </div>
            </div>
        </a>
        {{else}}
        <div class="p-8 text-center text-gray-500">
            No builds yet. Push a commit to trigger a build.
        </div>
        {{end}}
    </div>
    
    {{template "pagination" .Pagination}}
</div>
{{end}}
```

### 17.2 Create Build Detail Template
`web/templates/pages/builds/show.html`:
```html
{{template "base" .}}

{{define "title"}}Build #{{.Build.BuildNumber}} - {{.Project.FullName}} - FeatherCI{{end}}

{{define "content"}}
<div class="space-y-6">
    <!-- Build Header -->
    <div class="card p-6">
        <div class="flex items-start justify-between">
            <div>
                <div class="flex items-center space-x-3">
                    {{template "status-icon-large" .Build.Status}}
                    <h1 class="text-2xl font-bold text-gray-900">
                        Build #{{.Build.BuildNumber}}
                    </h1>
                    {{template "status-badge" .Build.Status}}
                </div>
                <p class="mt-2 text-gray-600">
                    {{.Build.CommitMessage}}
                </p>
            </div>
            {{if eq .Build.Status "running"}}
            <form action="/projects/{{.Project.Namespace}}/{{.Project.Name}}/builds/{{.Build.BuildNumber}}/cancel" method="POST">
                <button type="submit" class="btn btn-danger">
                    Cancel Build
                </button>
            </form>
            {{end}}
        </div>
        
        <dl class="mt-4 grid grid-cols-4 gap-4 text-sm">
            <div>
                <dt class="text-gray-500">Branch</dt>
                <dd class="font-medium">{{.Build.Branch}}</dd>
            </div>
            <div>
                <dt class="text-gray-500">Commit</dt>
                <dd class="font-mono">{{.Build.CommitSHA | truncate 7}}</dd>
            </div>
            <div>
                <dt class="text-gray-500">Author</dt>
                <dd>{{.Build.CommitAuthor}}</dd>
            </div>
            <div>
                <dt class="text-gray-500">Duration</dt>
                <dd>{{if .Build.FinishedAt}}{{duration .Build.StartedAt .Build.FinishedAt}}{{else}}Running...{{end}}</dd>
            </div>
        </dl>
    </div>
    
    <!-- Pipeline Graph -->
    <div class="card p-6">
        <h2 class="text-lg font-medium text-gray-900 mb-4">Pipeline</h2>
        <div id="pipeline-graph" 
             hx-get="/projects/{{.Project.Namespace}}/{{.Project.Name}}/builds/{{.Build.BuildNumber}}/graph"
             hx-trigger="load, every 5s"
             hx-swap="innerHTML">
            <!-- SVG graph rendered here -->
        </div>
    </div>
    
    <!-- Step Details -->
    <div class="card divide-y divide-gray-200">
        <h2 class="text-lg font-medium text-gray-900 p-4">Steps</h2>
        {{range .Build.Steps}}
        <div class="p-4" id="step-{{.ID}}">
            <div class="flex items-center justify-between">
                <div class="flex items-center space-x-3">
                    {{template "status-icon" .Status}}
                    <span class="font-medium">{{.Name}}</span>
                    {{if .Image}}
                    <span class="text-xs text-gray-500 font-mono">{{.Image}}</span>
                    {{end}}
                </div>
                <div class="flex items-center space-x-4">
                    {{if .FinishedAt}}
                    <span class="text-sm text-gray-500">{{duration .StartedAt .FinishedAt}}</span>
                    {{end}}
                    {{if eq .Status "waiting_approval"}}
                    <form action="/projects/{{$.Project.Namespace}}/{{$.Project.Name}}/builds/{{$.Build.BuildNumber}}/steps/{{.ID}}/approve" method="POST">
                        <button type="submit" class="btn btn-primary text-sm">
                            Approve
                        </button>
                    </form>
                    {{end}}
                </div>
            </div>
            
            {{if .ApprovedBy}}
            <p class="mt-2 text-sm text-gray-500">
                Approved by {{.ApprovedByUser.Username}} at {{.ApprovedAt | formatTime}}
            </p>
            {{end}}
            
            <!-- Expandable log viewer -->
            {{if or (eq .Status "running") (eq .Status "success") (eq .Status "failure")}}
            <details class="mt-4">
                <summary class="cursor-pointer text-sm text-feather-600 hover:text-feather-700">
                    View Logs
                </summary>
                <div class="mt-2 bg-gray-900 rounded-md p-4 overflow-x-auto"
                     hx-get="/projects/{{$.Project.Namespace}}/{{$.Project.Name}}/builds/{{$.Build.BuildNumber}}/steps/{{.ID}}/logs"
                     hx-trigger="revealed, every 2s[{{if eq .Status "running"}}true{{else}}false{{end}}]"
                     hx-swap="innerHTML">
                    <pre class="text-sm text-gray-100 font-mono"><code>Loading logs...</code></pre>
                </div>
            </details>
            {{end}}
        </div>
        {{end}}
    </div>
</div>
{{end}}
```

### 17.3 Create Build Handlers
```go
type BuildHandler struct {
    builds   BuildRepository
    steps    BuildStepRepository
    projects ProjectRepository
    users    UserRepository
}

func (h *BuildHandler) List(w http.ResponseWriter, r *http.Request) {
    owner := r.PathValue("owner")
    repo := r.PathValue("repo")
    
    project, err := h.projects.GetByFullName(r.Context(), owner+"/"+repo)
    // ...
    
    builds, _ := h.builds.ListByProject(r.Context(), project.ID, 20, 0)
    
    render(w, "builds/list.html", map[string]any{
        "Project": project,
        "Builds":  builds,
    })
}

func (h *BuildHandler) Show(w http.ResponseWriter, r *http.Request) {
    // Load build with steps
    // Render detail page
}

func (h *BuildHandler) Cancel(w http.ResponseWriter, r *http.Request) {
    // Cancel build
    // Redirect back
}
```

### 17.4 Create Real-time Updates with HTMX
Use `hx-trigger="every 5s"` for automatic polling of build status.

### 17.5 Create Status Icon Components
`web/templates/components/status-icon.html`:
```html
{{define "status-icon"}}
{{if eq . "success"}}
<svg class="w-5 h-5 text-green-500" fill="currentColor" viewBox="0 0 20 20">
    <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd"/>
</svg>
{{else if eq . "failure"}}
<svg class="w-5 h-5 text-red-500" fill="currentColor" viewBox="0 0 20 20">
    <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd"/>
</svg>
{{else if eq . "running"}}
<svg class="w-5 h-5 text-blue-500 animate-spin" fill="none" viewBox="0 0 24 24">
    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
    <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
</svg>
{{else if eq . "waiting_approval"}}
<svg class="w-5 h-5 text-yellow-500" fill="currentColor" viewBox="0 0 20 20">
    <path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7 4a1 1 0 11-2 0 1 1 0 012 0zm-1-9a1 1 0 00-1 1v4a1 1 0 102 0V6a1 1 0 00-1-1z" clip-rule="evenodd"/>
</svg>
{{else}}
<svg class="w-5 h-5 text-gray-400" fill="currentColor" viewBox="0 0 20 20">
    <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm1-12a1 1 0 10-2 0v4a1 1 0 00.293.707l2.828 2.829a1 1 0 101.415-1.415L11 9.586V6z" clip-rule="evenodd"/>
</svg>
{{end}}
{{end}}
```

### 17.6 Add Tests
- Test build list rendering
- Test build detail rendering
- Test status icons
- Test HTMX updates

## Deliverables
- [ ] `web/templates/pages/builds/list.html` - Build list page
- [ ] `web/templates/pages/builds/show.html` - Build detail page
- [ ] `web/templates/components/status-icon.html` - Status icons
- [ ] `internal/handlers/build.go` - Build handlers
- [ ] Real-time updates working

## Dependencies
- Step 08: Templates
- Step 12: Build model

## Estimated Effort
Medium - UI implementation
