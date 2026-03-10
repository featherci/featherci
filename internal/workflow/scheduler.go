package workflow

import (
	"path/filepath"
	"strings"
)

// ExecutionGroup represents a group of steps that can run in parallel.
type ExecutionGroup struct {
	Steps []Step
}

// ExecutionGroups returns steps grouped by execution order.
// Steps in the same group have all their dependencies satisfied
// and can potentially run in parallel.
func (w *Workflow) ExecutionGroups() []ExecutionGroup {
	if len(w.Steps) == 0 {
		return nil
	}

	// Build a map for quick step lookup
	stepMap := make(map[string]*Step)
	for i := range w.Steps {
		stepMap[w.Steps[i].Name] = &w.Steps[i]
	}

	// Track which steps have been scheduled
	scheduled := make(map[string]bool)
	var groups []ExecutionGroup

	for len(scheduled) < len(w.Steps) {
		var group ExecutionGroup

		for _, step := range w.Steps {
			if scheduled[step.Name] {
				continue
			}

			// Check if all dependencies are scheduled
			allDepsScheduled := true
			for _, dep := range step.DependsOn {
				if !scheduled[dep] {
					allDepsScheduled = false
					break
				}
			}

			if allDepsScheduled {
				group.Steps = append(group.Steps, step)
			}
		}

		// Mark all steps in this group as scheduled
		for _, step := range group.Steps {
			scheduled[step.Name] = true
		}

		if len(group.Steps) > 0 {
			groups = append(groups, group)
		}
	}

	return groups
}

// ReadySteps returns steps that are ready to run given a set of completed steps.
// This is useful for dynamic scheduling during execution.
func (w *Workflow) ReadySteps(completed map[string]bool) []Step {
	var ready []Step

	for _, step := range w.Steps {
		// Skip already completed steps
		if completed[step.Name] {
			continue
		}

		// Check if all dependencies are completed
		allDepsComplete := true
		for _, dep := range step.DependsOn {
			if !completed[dep] {
				allDepsComplete = false
				break
			}
		}

		if allDepsComplete {
			ready = append(ready, step)
		}
	}

	return ready
}

// RootSteps returns steps that have no dependencies.
func (w *Workflow) RootSteps() []Step {
	var roots []Step
	for _, step := range w.Steps {
		if len(step.DependsOn) == 0 {
			roots = append(roots, step)
		}
	}
	return roots
}

// GetStep returns a step by name, or nil if not found.
func (w *Workflow) GetStep(name string) *Step {
	for i := range w.Steps {
		if w.Steps[i].Name == name {
			return &w.Steps[i]
		}
	}
	return nil
}

// Dependents returns the steps that depend on the given step.
func (w *Workflow) Dependents(stepName string) []Step {
	var dependents []Step
	for _, step := range w.Steps {
		for _, dep := range step.DependsOn {
			if dep == stepName {
				dependents = append(dependents, step)
				break
			}
		}
	}
	return dependents
}

// ShouldTrigger determines if this workflow should run for the given event.
func (w *Workflow) ShouldTrigger(eventType, branch, tag string) bool {
	switch eventType {
	case "push":
		return w.shouldTriggerOnPush(branch, tag)
	case "pull_request", "merge_request":
		return w.shouldTriggerOnPullRequest(branch)
	default:
		return false
	}
}

// shouldTriggerOnPush checks if the workflow should trigger on a push event.
func (w *Workflow) shouldTriggerOnPush(branch, tag string) bool {
	// If no trigger config, default to triggering on all pushes
	if w.On.Push == nil && w.On.PullRequest == nil {
		return true
	}

	// If push trigger is explicitly nil but PR trigger exists,
	// don't trigger on push
	if w.On.Push == nil {
		return false
	}

	// Check tag triggers
	if tag != "" {
		if len(w.On.Push.Tags) == 0 {
			// No tag filter means trigger on all tags
			return true
		}
		for _, pattern := range w.On.Push.Tags {
			if MatchGlob(pattern, tag) {
				return true
			}
		}
		return false
	}

	// Check branch triggers
	if branch != "" {
		if len(w.On.Push.Branches) == 0 {
			// No branch filter means trigger on all branches
			return true
		}
		for _, pattern := range w.On.Push.Branches {
			if MatchGlob(pattern, branch) {
				return true
			}
		}
		return false
	}

	return true
}

// shouldTriggerOnPullRequest checks if the workflow should trigger on a PR event.
func (w *Workflow) shouldTriggerOnPullRequest(targetBranch string) bool {
	// If no PR trigger configured, don't trigger
	if w.On.PullRequest == nil {
		return false
	}

	// If no branch filter, trigger on all target branches
	if len(w.On.PullRequest.Branches) == 0 {
		return true
	}

	// Check if target branch matches any pattern
	for _, pattern := range w.On.PullRequest.Branches {
		if MatchGlob(pattern, targetBranch) {
			return true
		}
	}

	return false
}

// MatchGlob performs simple glob pattern matching.
// Supports * for any characters and ** for any path segments.
func MatchGlob(pattern, value string) bool {
	// Handle exact match
	if pattern == value {
		return true
	}

	// Use filepath.Match for basic glob support
	// Note: filepath.Match doesn't support ** properly, so we handle it specially
	if strings.Contains(pattern, "**") {
		// Convert ** to a regex-like match
		// For now, treat ** as matching everything
		parts := strings.Split(pattern, "**")
		if len(parts) == 2 {
			prefix := parts[0]
			suffix := parts[1]
			return strings.HasPrefix(value, prefix) && strings.HasSuffix(value, suffix)
		}
	}

	// Use filepath.Match for single * patterns
	matched, err := filepath.Match(pattern, value)
	if err != nil {
		return false
	}
	return matched
}

// TopologicalOrder returns steps in topological order (dependencies first).
func (w *Workflow) TopologicalOrder() []Step {
	var result []Step
	visited := make(map[string]bool)

	var visit func(name string)
	visit = func(name string) {
		if visited[name] {
			return
		}
		step := w.GetStep(name)
		if step == nil {
			return
		}

		// Visit dependencies first
		for _, dep := range step.DependsOn {
			visit(dep)
		}

		visited[name] = true
		result = append(result, *step)
	}

	for _, step := range w.Steps {
		visit(step.Name)
	}

	return result
}
