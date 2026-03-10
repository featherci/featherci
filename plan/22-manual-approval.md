---
model: sonnet
---

# Step 22: Manual Approval Gates

## Objective
Implement manual approval steps that block pipeline progression until a user approves.

## Tasks

### 22.1 Approval Step Detection
Already handled in workflow parsing (Step 11) with `type: approval`.

### 22.2 Approval State Management
```go
func (o *BuildOrchestrator) updateReadySteps(ctx context.Context) {
    // ... existing logic ...
    
    // Handle approval steps - when dependencies are met, set to waiting_approval
    query := `
        UPDATE build_steps
        SET status = 'waiting_approval', updated_at = CURRENT_TIMESTAMP
        WHERE status = 'waiting'
          AND requires_approval = true
          AND id NOT IN (
              SELECT sd.step_id
              FROM step_dependencies sd
              JOIN build_steps dep ON sd.depends_on_step_id = dep.id
              WHERE dep.status != 'success'
          )
    `
    o.db.Exec(query)
}
```

### 22.3 Approval Handler
```go
func (h *BuildHandler) ApproveStep(w http.ResponseWriter, r *http.Request) {
    user := getUserFromContext(r)
    stepID := parseStepID(r)
    
    step, err := h.steps.GetByID(r.Context(), stepID)
    if err != nil {
        http.Error(w, "step not found", http.StatusNotFound)
        return
    }
    
    // Verify step is waiting for approval
    if step.Status != StepStatusWaitingApproval {
        http.Error(w, "step is not waiting for approval", http.StatusBadRequest)
        return
    }
    
    // Verify user has access to project
    build, _ := h.builds.GetByID(r.Context(), step.BuildID)
    canAccess, _ := h.projectUsers.CanUserAccess(r.Context(), build.ProjectID, user.ID)
    if !canAccess {
        http.Error(w, "forbidden", http.StatusForbidden)
        return
    }
    
    // Record approval
    now := time.Now()
    step.Status = StepStatusSuccess
    step.ApprovedBy = &user.ID
    step.ApprovedAt = &now
    step.StartedAt = &now
    step.FinishedAt = &now
    
    if err := h.steps.Update(r.Context(), step); err != nil {
        http.Error(w, "failed to approve", http.StatusInternalServerError)
        return
    }
    
    // Trigger orchestrator to update ready steps
    h.orchestrator.TriggerUpdate()
    
    // Redirect back to build page
    project, _ := h.projects.GetByID(r.Context(), build.ProjectID)
    http.Redirect(w, r, 
        fmt.Sprintf("/projects/%s/builds/%d", project.FullName, build.BuildNumber),
        http.StatusSeeOther)
}
```

### 22.4 Approval UI Component
In the build detail page, show approval button for waiting steps:
```html
{{if eq .Status "waiting_approval"}}
<div class="flex items-center space-x-4 mt-4 p-4 bg-yellow-50 rounded-lg border border-yellow-200">
    <svg class="w-6 h-6 text-yellow-500" fill="currentColor" viewBox="0 0 20 20">
        <path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7 4a1 1 0 11-2 0 1 1 0 012 0zm-1-9a1 1 0 00-1 1v4a1 1 0 102 0V6a1 1 0 00-1-1z" clip-rule="evenodd"/>
    </svg>
    <div class="flex-1">
        <p class="text-sm font-medium text-yellow-800">
            This step requires manual approval to proceed
        </p>
    </div>
    <form action="/projects/{{$.Project.Namespace}}/{{$.Project.Name}}/builds/{{$.Build.BuildNumber}}/steps/{{.ID}}/approve" method="POST">
        <button type="submit" class="btn btn-primary">
            Approve & Continue
        </button>
    </form>
</div>
{{end}}

{{if .ApprovedBy}}
<div class="mt-2 text-sm text-gray-500">
    <span class="inline-flex items-center">
        <svg class="w-4 h-4 mr-1 text-green-500" fill="currentColor" viewBox="0 0 20 20">
            <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd"/>
        </svg>
        Approved by <strong>{{.ApprovedByUser.Username}}</strong> 
        at {{.ApprovedAt | formatTime}}
    </span>
</div>
{{end}}
```

### 22.5 Notification (Optional Enhancement)
```go
// Could add webhook notifications when approval is needed
type ApprovalNotifier struct {
    webhookURL string
}

func (n *ApprovalNotifier) NotifyApprovalNeeded(build *Build, step *BuildStep, project *Project) {
    if n.webhookURL == "" {
        return
    }
    
    payload := map[string]any{
        "event":   "approval_required",
        "project": project.FullName,
        "build":   build.BuildNumber,
        "step":    step.Name,
        "url":     fmt.Sprintf("%s/projects/%s/builds/%d", baseURL, project.FullName, build.BuildNumber),
    }
    
    // POST to webhook
}
```

### 22.6 Approval Timeout (Optional)
```go
// Auto-cancel builds waiting too long for approval
func (o *BuildOrchestrator) checkApprovalTimeouts(ctx context.Context) {
    timeout := 24 * time.Hour // Configurable
    
    query := `
        UPDATE build_steps
        SET status = 'cancelled'
        WHERE status = 'waiting_approval'
          AND updated_at < ?
    `
    o.db.Exec(query, time.Now().Add(-timeout))
}
```

### 22.7 Add Tests
- Test approval state transitions
- Test approval recording
- Test authorization checks
- Test dependent steps unblock after approval

## Deliverables
- [ ] Approval step status management
- [ ] `POST /projects/{owner}/{repo}/builds/{number}/steps/{step}/approve` endpoint
- [ ] Approval UI with user attribution
- [ ] Tests

## Dependencies
- Step 11: Workflow parsing
- Step 16: Build orchestration
- Step 17: Build status UI

## Estimated Effort
Small - Building on existing infrastructure
