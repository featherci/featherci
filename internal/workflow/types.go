// Package workflow handles parsing and validation of FeatherCI workflow files.
package workflow

import "gopkg.in/yaml.v3"

// Workflow represents a parsed .featherci/workflow.yml file.
type Workflow struct {
	// Name is the display name of the workflow.
	Name string `yaml:"name"`

	// On defines when the workflow should trigger.
	On TriggerConfig `yaml:"on"`

	// Steps defines the steps to execute.
	Steps []Step `yaml:"steps"`
}

// TriggerConfig defines when a workflow should be triggered.
type TriggerConfig struct {
	// Push configures push event triggers.
	Push *PushTrigger `yaml:"push"`

	// PullRequest configures pull request event triggers.
	PullRequest *PullRequestTrigger `yaml:"pull_request"`
}

// UnmarshalYAML implements custom unmarshaling to handle empty trigger values.
// In YAML, `pull_request:` with no value should enable PR triggering (empty struct),
// but the default yaml parser sets pointer fields to nil when the value is empty.
func (tc *TriggerConfig) UnmarshalYAML(node *yaml.Node) error {
	// MappingNode content is interleaved key-value pairs: [key1, val1, key2, val2, ...]
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		val := node.Content[i+1]

		switch key.Value {
		case "push":
			tc.Push = &PushTrigger{}
			if !isEmptyNode(val) {
				if err := val.Decode(tc.Push); err != nil {
					return err
				}
			}
		case "pull_request":
			tc.PullRequest = &PullRequestTrigger{}
			if !isEmptyNode(val) {
				if err := val.Decode(tc.PullRequest); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// isEmptyNode checks if a YAML node represents an empty/null value.
func isEmptyNode(node *yaml.Node) bool {
	// A null/empty value in YAML is represented as a scalar node with tag "!!null"
	return node.Kind == yaml.ScalarNode && node.Tag == "!!null"
}

// PushTrigger configures which push events trigger the workflow.
type PushTrigger struct {
	// Branches is a list of branch patterns that trigger the workflow.
	// Supports glob patterns like "feature/*".
	// If empty, all branches trigger the workflow.
	Branches []string `yaml:"branches"`

	// Tags is a list of tag patterns that trigger the workflow.
	// Supports glob patterns like "v*".
	Tags []string `yaml:"tags"`
}

// PullRequestTrigger configures which pull request events trigger the workflow.
type PullRequestTrigger struct {
	// Branches is a list of target branch patterns that trigger the workflow.
	// If empty, all target branches trigger the workflow.
	Branches []string `yaml:"branches"`
}

// StepType represents the type of a workflow step.
type StepType string

const (
	// StepTypeCommand is the default step type that runs commands in a container.
	StepTypeCommand StepType = ""

	// StepTypeApproval is a step that waits for manual approval.
	StepTypeApproval StepType = "approval"
)

// Step represents a single step in the workflow.
type Step struct {
	// Name is the unique identifier for this step.
	Name string `yaml:"name"`

	// Type is the step type: "" (command, default) or "approval".
	Type StepType `yaml:"type"`

	// Image is the Docker image to use for command steps.
	// Required for command steps, ignored for approval steps.
	Image string `yaml:"image"`

	// Commands is the list of shell commands to execute.
	Commands []string `yaml:"commands"`

	// DependsOn is the list of step names that must complete before this step.
	DependsOn []string `yaml:"depends_on"`

	// Env is a map of environment variables to set.
	Env map[string]string `yaml:"env"`

	// WorkingDir is the working directory for commands.
	// Defaults to the repository root.
	WorkingDir string `yaml:"working_dir"`

	// TimeoutMinutes is the maximum time this step can run.
	// Defaults to 60 minutes.
	TimeoutMinutes int `yaml:"timeout_minutes"`

	// ContinueOnError allows the workflow to continue even if this step fails.
	ContinueOnError bool `yaml:"continue_on_error"`

	// Cache configures caching for this step.
	Cache *CacheConfig `yaml:"cache"`

	// Secrets is a list of secret names to make available to this step.
	Secrets []string `yaml:"secrets"`
}

// CacheConfig defines caching configuration for a step.
type CacheConfig struct {
	// Paths is a list of paths to cache.
	Paths []string `yaml:"paths"`

	// Key is the cache key template.
	// Supports variables like {{ .Branch }}, {{ checksum "go.sum" }}
	Key string `yaml:"key"`
}

// IsApproval returns true if this is an approval step.
func (s *Step) IsApproval() bool {
	return s.Type == StepTypeApproval
}

// IsCommand returns true if this is a command step.
func (s *Step) IsCommand() bool {
	return s.Type == StepTypeCommand || s.Type == ""
}

// GetTimeout returns the timeout in minutes, defaulting to 60 if not set.
func (s *Step) GetTimeout() int {
	if s.TimeoutMinutes <= 0 {
		return 60
	}
	return s.TimeoutMinutes
}

// HasDependencies returns true if this step has dependencies.
func (s *Step) HasDependencies() bool {
	return len(s.DependsOn) > 0
}
