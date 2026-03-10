package workflow

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ValidationError represents a workflow validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return e.Message
}

// Validator validates workflow configurations.
type Validator struct{}

// NewValidator creates a new workflow validator.
func NewValidator() *Validator {
	return &Validator{}
}

// Validate performs all validation checks on a workflow.
func (v *Validator) Validate(w *Workflow) error {
	if err := v.validateBasic(w); err != nil {
		return err
	}

	if err := v.validateStepNames(w); err != nil {
		return err
	}

	if err := v.validateDependencies(w); err != nil {
		return err
	}

	if err := v.validateCycles(w); err != nil {
		return err
	}

	if err := v.validateStepConfigs(w); err != nil {
		return err
	}

	return nil
}

// validateBasic checks basic workflow requirements.
func (v *Validator) validateBasic(w *Workflow) error {
	if len(w.Steps) == 0 {
		return &ValidationError{
			Field:   "steps",
			Message: "workflow must have at least one step",
		}
	}
	return nil
}

// validateStepNames ensures all step names are unique and valid.
func (v *Validator) validateStepNames(w *Workflow) error {
	names := make(map[string]bool)

	for i, step := range w.Steps {
		if step.Name == "" {
			return &ValidationError{
				Field:   fmt.Sprintf("steps[%d].name", i),
				Message: "step name is required",
			}
		}

		// Check for valid characters in step name
		if !isValidStepName(step.Name) {
			return &ValidationError{
				Field:   fmt.Sprintf("steps[%d].name", i),
				Message: fmt.Sprintf("step name %q contains invalid characters; use only letters, numbers, hyphens, and underscores", step.Name),
			}
		}

		if names[step.Name] {
			return &ValidationError{
				Field:   fmt.Sprintf("steps[%d].name", i),
				Message: fmt.Sprintf("duplicate step name: %s", step.Name),
			}
		}
		names[step.Name] = true
	}

	return nil
}

// validateDependencies ensures all dependencies reference existing steps.
func (v *Validator) validateDependencies(w *Workflow) error {
	// Build set of all step names
	names := make(map[string]bool)
	for _, step := range w.Steps {
		names[step.Name] = true
	}

	// Check all dependencies reference existing steps
	for _, step := range w.Steps {
		for _, dep := range step.DependsOn {
			if !names[dep] {
				return &ValidationError{
					Field:   fmt.Sprintf("steps.%s.depends_on", step.Name),
					Message: fmt.Sprintf("depends on unknown step: %s", dep),
				}
			}
			if dep == step.Name {
				return &ValidationError{
					Field:   fmt.Sprintf("steps.%s.depends_on", step.Name),
					Message: "step cannot depend on itself",
				}
			}
		}
	}

	return nil
}

// validateCycles detects circular dependencies in the workflow.
func (v *Validator) validateCycles(w *Workflow) error {
	// Build adjacency list
	graph := make(map[string][]string)
	for _, step := range w.Steps {
		graph[step.Name] = step.DependsOn
	}

	// Track state for DFS: 0 = unvisited, 1 = visiting, 2 = visited
	state := make(map[string]int)

	// Track the path for error reporting
	var path []string

	var dfs func(node string) error
	dfs = func(node string) error {
		state[node] = 1 // visiting
		path = append(path, node)

		for _, dep := range graph[node] {
			if state[dep] == 1 {
				// Found a cycle - find where it starts in the path
				cycleStart := 0
				for i, n := range path {
					if n == dep {
						cycleStart = i
						break
					}
				}
				cycle := append(path[cycleStart:], dep)
				return &ValidationError{
					Message: fmt.Sprintf("circular dependency detected: %s", strings.Join(cycle, " -> ")),
				}
			}
			if state[dep] == 0 {
				if err := dfs(dep); err != nil {
					return err
				}
			}
		}

		path = path[:len(path)-1]
		state[node] = 2 // visited
		return nil
	}

	for _, step := range w.Steps {
		if state[step.Name] == 0 {
			if err := dfs(step.Name); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateStepConfigs validates individual step configurations.
func (v *Validator) validateStepConfigs(w *Workflow) error {
	for _, step := range w.Steps {
		if err := v.validateStep(&step); err != nil {
			return err
		}
	}
	return nil
}

// validateStep validates a single step's configuration.
func (v *Validator) validateStep(step *Step) error {
	// Command steps require an image
	if step.IsCommand() {
		if step.Image == "" {
			return &ValidationError{
				Field:   fmt.Sprintf("steps.%s.image", step.Name),
				Message: "command steps require an image",
			}
		}

		// Commands should be non-empty for command steps
		if len(step.Commands) == 0 {
			return &ValidationError{
				Field:   fmt.Sprintf("steps.%s.commands", step.Name),
				Message: "command steps require at least one command",
			}
		}
	}

	// Approval steps should not have commands or image
	if step.IsApproval() {
		if step.Image != "" {
			return &ValidationError{
				Field:   fmt.Sprintf("steps.%s.image", step.Name),
				Message: "approval steps should not have an image",
			}
		}
		if len(step.Commands) > 0 {
			return &ValidationError{
				Field:   fmt.Sprintf("steps.%s.commands", step.Name),
				Message: "approval steps should not have commands",
			}
		}
	}

	// Validate timeout
	if step.TimeoutMinutes < 0 {
		return &ValidationError{
			Field:   fmt.Sprintf("steps.%s.timeout_minutes", step.Name),
			Message: "timeout cannot be negative",
		}
	}

	// Validate working directory (must be relative)
	if step.WorkingDir != "" && filepath.IsAbs(step.WorkingDir) {
		return &ValidationError{
			Field:   fmt.Sprintf("steps.%s.working_dir", step.Name),
			Message: "working directory must be a relative path",
		}
	}

	// Validate cache config
	if step.Cache != nil {
		if len(step.Cache.Paths) == 0 {
			return &ValidationError{
				Field:   fmt.Sprintf("steps.%s.cache.paths", step.Name),
				Message: "cache requires at least one path",
			}
		}
		if step.Cache.Key == "" {
			return &ValidationError{
				Field:   fmt.Sprintf("steps.%s.cache.key", step.Name),
				Message: "cache requires a key",
			}
		}
	}

	return nil
}

// isValidStepName checks if a step name contains only valid characters.
func isValidStepName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// ErrWorkflowNotFound is returned when a workflow file is not found.
var ErrWorkflowNotFound = errors.New("workflow file not found")
