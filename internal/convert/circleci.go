package convert

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/featherci/featherci/internal/workflow"
)

// CircleCI YAML structures.

type circleConfig struct {
	Version   interface{}                 `yaml:"version"`
	Orbs      map[string]interface{}      `yaml:"orbs"`
	Jobs      map[string]*circleJob       `yaml:"jobs"`
	Workflows map[string]*circleWorkflow  `yaml:"workflows"`
}

type circleJob struct {
	Docker      []circleDocker        `yaml:"docker"`
	Machine     interface{}           `yaml:"machine"`
	Macos       interface{}           `yaml:"macos"`
	Steps       []circleStep          `yaml:"steps"`
	Environment map[string]string     `yaml:"environment"`
	WorkingDir  string                `yaml:"working_directory"`
	Parallelism int                   `yaml:"parallelism"`
}

type circleDocker struct {
	Image string            `yaml:"image"`
	Env   map[string]string `yaml:"environment"`
}

// circleStep handles CircleCI's flexible step format:
// - "checkout" (string)
// - run: "command" (map with string value)
// - run: { command: "...", name: "..." } (map with object value)
// - save_cache: { ... }
// etc.
type circleStep struct {
	Type string
	// For run steps
	Name    string
	Command string
	// For cache steps
	CacheKey   string
	CachePaths []string
	// Raw for unhandled
	Raw interface{}
}

func (s *circleStep) UnmarshalYAML(node *yaml.Node) error {
	// String form: "checkout", "setup_remote_docker"
	if node.Kind == yaml.ScalarNode {
		s.Type = node.Value
		return nil
	}

	// Map form
	if node.Kind != yaml.MappingNode {
		return nil
	}

	var m map[string]yaml.Node
	if err := node.Decode(&m); err != nil {
		return err
	}

	for key, val := range m {
		s.Type = key
		switch key {
		case "run":
			if val.Kind == yaml.ScalarNode {
				s.Command = val.Value
			} else {
				var runMap struct {
					Command string `yaml:"command"`
					Name    string `yaml:"name"`
				}
				if err := val.Decode(&runMap); err != nil {
					return err
				}
				s.Command = runMap.Command
				s.Name = runMap.Name
			}
		case "save_cache":
			var cache struct {
				Key   string   `yaml:"key"`
				Paths []string `yaml:"paths"`
			}
			if err := val.Decode(&cache); err == nil {
				s.CacheKey = cache.Key
				s.CachePaths = cache.Paths
			}
		case "restore_cache":
			var cache struct {
				Keys []string `yaml:"keys"`
				Key  string   `yaml:"key"`
			}
			if err := val.Decode(&cache); err == nil {
				if cache.Key != "" {
					s.CacheKey = cache.Key
				} else if len(cache.Keys) > 0 {
					s.CacheKey = cache.Keys[0]
				}
			}
		default:
			s.Raw = m
		}
		break // only process first key
	}
	return nil
}

type circleWorkflow struct {
	Jobs []circleWorkflowJob `yaml:"jobs"`
}

// circleWorkflowJob handles CircleCI's workflow job format:
// - "job_name" (string)
// - { job_name: { requires: [...], type: approval } } (map)
type circleWorkflowJob struct {
	Name     string
	Requires []string
	Type     string // "approval"
	Filters  interface{}
}

func (j *circleWorkflowJob) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		j.Name = node.Value
		return nil
	}

	// Map form: { job_name: { requires: [...] } }
	var m map[string]struct {
		Requires []string    `yaml:"requires"`
		Type     string      `yaml:"type"`
		Filters  interface{} `yaml:"filters"`
	}
	if err := node.Decode(&m); err != nil {
		return err
	}
	for name, opts := range m {
		j.Name = name
		j.Requires = opts.Requires
		j.Type = opts.Type
		j.Filters = opts.Filters
		break
	}
	return nil
}

func convertCircleCI(data []byte, sourceFile string) (*Result, error) {
	var cc circleConfig
	if err := yaml.Unmarshal(data, &cc); err != nil {
		return nil, fmt.Errorf("failed to parse CircleCI config: %w", err)
	}

	result := &Result{
		Source:     SourceCircleCI,
		SourceFile: sourceFile,
		Workflow:   &workflow.Workflow{},
	}

	// Orbs warning
	if len(cc.Orbs) > 0 {
		orbNames := make([]string, 0, len(cc.Orbs))
		for name := range cc.Orbs {
			orbNames = append(orbNames, name)
		}
		result.Warnings = append(result.Warnings, Warning{
			Feature: "orbs",
			Message: fmt.Sprintf("CircleCI orbs are not supported: %s. Replace with equivalent shell commands.", strings.Join(orbNames, ", ")),
		})
	}

	// Pick the first workflow (or "build" if only jobs exist)
	var wfName string
	var wfJobs []circleWorkflowJob

	if len(cc.Workflows) > 0 {
		for name, wf := range cc.Workflows {
			if name == "version" {
				continue
			}
			wfName = name
			wfJobs = wf.Jobs
			break
		}
	} else if len(cc.Jobs) > 0 {
		// No workflows section — just convert all jobs
		wfName = "build"
		for jobName := range cc.Jobs {
			wfJobs = append(wfJobs, circleWorkflowJob{Name: jobName})
		}
	}

	result.Workflow.Name = wfName
	if result.Workflow.Name == "" {
		result.Workflow.Name = "Converted Pipeline"
	}

	// Default triggers (CircleCI triggers are in filters, not easily mappable)
	result.Workflow.On.Push = &workflow.PushTrigger{}
	result.Warnings = append(result.Warnings, Warning{
		Feature: "triggers",
		Message: "CircleCI trigger filters don't map directly to FeatherCI. Defaulting to trigger on all pushes — adjust the 'on' section as needed.",
	})

	// Convert each job in the workflow
	for _, wfJob := range wfJobs {
		jobDef := cc.Jobs[wfJob.Name]

		// Approval gate
		if wfJob.Type == "approval" {
			step := workflow.Step{
				Name: sanitizeName(wfJob.Name),
				Type: workflow.StepTypeApproval,
			}
			if len(wfJob.Requires) > 0 {
				for _, req := range wfJob.Requires {
					step.DependsOn = append(step.DependsOn, sanitizeName(req))
				}
			}
			result.Workflow.Steps = append(result.Workflow.Steps, step)
			continue
		}

		if jobDef == nil {
			result.Warnings = append(result.Warnings, Warning{
				Feature: fmt.Sprintf("job '%s'", wfJob.Name),
				Message: "Job referenced in workflow but not defined in jobs section. Skipped.",
			})
			continue
		}

		step := convertCircleCIJob(wfJob.Name, jobDef, wfJob, result)
		result.Workflow.Steps = append(result.Workflow.Steps, step)
	}

	return result, nil
}

func convertCircleCIJob(jobID string, job *circleJob, wfJob circleWorkflowJob, result *Result) workflow.Step {
	step := workflow.Step{
		Name: sanitizeName(jobID),
	}

	// Image
	if len(job.Docker) > 0 {
		step.Image = job.Docker[0].Image
	} else if job.Machine != nil {
		step.Image = "ubuntu:latest"
		result.Warnings = append(result.Warnings, Warning{
			Feature: "machine executor",
			Message: fmt.Sprintf("Job '%s' uses a machine executor. FeatherCI uses Docker containers — using 'ubuntu:latest' as default. Choose an appropriate image.", jobID),
		})
	} else if job.Macos != nil {
		step.Image = "ubuntu:latest"
		result.Warnings = append(result.Warnings, Warning{
			Feature: "macos executor",
			Message: fmt.Sprintf("Job '%s' uses a macOS executor. FeatherCI only supports Linux Docker containers — using 'ubuntu:latest' as default.", jobID),
		})
	}

	// Commands and cache
	var commands []string
	var cacheConfig *workflow.CacheConfig

	for _, cs := range job.Steps {
		switch cs.Type {
		case "checkout":
			// FeatherCI auto-checks out — skip
			continue

		case "run":
			if cs.Command != "" {
				lines := strings.Split(strings.TrimSpace(cs.Command), "\n")
				commands = append(commands, lines...)
			}

		case "save_cache":
			if cs.CacheKey != "" && len(cs.CachePaths) > 0 {
				cacheConfig = &workflow.CacheConfig{
					Key:   convertCircleCacheKey(cs.CacheKey),
					Paths: cs.CachePaths,
				}
			}

		case "restore_cache":
			// Handled via save_cache — if we only see restore, still try to capture key
			if cacheConfig == nil && cs.CacheKey != "" {
				cacheConfig = &workflow.CacheConfig{
					Key: convertCircleCacheKey(cs.CacheKey),
				}
			}

		case "persist_to_workspace":
			result.Warnings = append(result.Warnings, Warning{
				Feature: "persist_to_workspace",
				Message: fmt.Sprintf("Job '%s': workspace persistence is not supported. Consider using cache or restructuring your pipeline.", jobID),
			})

		case "attach_workspace":
			result.Warnings = append(result.Warnings, Warning{
				Feature: "attach_workspace",
				Message: fmt.Sprintf("Job '%s': workspace attachment is not supported. Consider using cache or restructuring your pipeline.", jobID),
			})

		case "store_artifacts":
			result.Warnings = append(result.Warnings, Warning{
				Feature: "store_artifacts",
				Message: fmt.Sprintf("Job '%s': artifact storage is not supported in FeatherCI.", jobID),
			})

		case "store_test_results":
			result.Warnings = append(result.Warnings, Warning{
				Feature: "store_test_results",
				Message: fmt.Sprintf("Job '%s': test result storage is not supported in FeatherCI.", jobID),
			})

		case "setup_remote_docker":
			// Skip — FeatherCI handles Docker differently
			continue

		default:
			// Unknown step type — might be an orb command
			result.Warnings = append(result.Warnings, Warning{
				Feature: fmt.Sprintf("step '%s'", cs.Type),
				Message: fmt.Sprintf("Job '%s': unrecognized step type (possibly from an orb). Replace with equivalent shell commands.", jobID),
			})
		}
	}

	step.Commands = commands
	step.Cache = cacheConfig

	// Dependencies
	if len(wfJob.Requires) > 0 {
		for _, req := range wfJob.Requires {
			step.DependsOn = append(step.DependsOn, sanitizeName(req))
		}
	}

	// Environment
	env := make(map[string]string)
	// From Docker image env
	if len(job.Docker) > 0 && job.Docker[0].Env != nil {
		for k, v := range job.Docker[0].Env {
			env[k] = v
		}
	}
	// From job environment
	for k, v := range job.Environment {
		env[k] = v
	}
	if len(env) > 0 {
		step.Env = env
	}

	// Working directory
	if job.WorkingDir != "" {
		wd := job.WorkingDir
		// CircleCI uses ~/project as default — strip it
		wd = strings.TrimPrefix(wd, "~/project/")
		wd = strings.TrimPrefix(wd, "~/project")
		if wd != "" && wd != "." {
			step.WorkingDir = wd
		}
	}

	// Parallelism warning
	if job.Parallelism > 0 {
		result.Warnings = append(result.Warnings, Warning{
			Feature: "parallelism",
			Message: fmt.Sprintf("Job '%s' uses parallelism (%d). FeatherCI does not support test splitting — run tests in a single step or create separate steps manually.", jobID, job.Parallelism),
		})
	}

	// Filters warning
	if wfJob.Filters != nil {
		result.Warnings = append(result.Warnings, Warning{
			Feature: "filters",
			Message: fmt.Sprintf("Job '%s' has workflow filters. Convert these to FeatherCI 'if' conditions manually.", jobID),
		})
	}

	return step
}

// convertCircleCacheKey converts a CircleCI cache key to FeatherCI format.
func convertCircleCacheKey(key string) string {
	// Replace {{ checksum "file" }} — already compatible format!
	// Replace {{ .Branch }} — already compatible!
	// Replace v1-deps-{{ ... }} patterns — keep as-is, they work
	return key
}
