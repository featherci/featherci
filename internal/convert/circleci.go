package convert

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/featherci/featherci/internal/workflow"
)

// CircleCI YAML structures.

type circleConfig struct {
	Version   any                         `yaml:"version"`
	Orbs      map[string]any              `yaml:"orbs"`
	Commands  map[string]*circleCommand   `yaml:"commands"`
	Jobs      map[string]*circleJob       `yaml:"jobs"`
	Workflows map[string]*circleWorkflow  `yaml:"workflows"`
}

type circleCommand struct {
	Description string        `yaml:"description"`
	Parameters  any           `yaml:"parameters"`
	Steps       []circleStep  `yaml:"steps"`
}

type circleJob struct {
	Docker      []circleDocker    `yaml:"docker"`
	Machine     any               `yaml:"machine"`
	Macos       any               `yaml:"macos"`
	Steps       []circleStep      `yaml:"steps"`
	Environment map[string]string `yaml:"environment"`
	WorkingDir  string            `yaml:"working_directory"`
	Parallelism int               `yaml:"parallelism"`
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
// - custom_command: { param: value } (custom commands or orb commands)
type circleStep struct {
	Type string
	// For run steps
	Name    string
	Command string
	// For cache steps
	CacheKey   string
	CachePaths []string
	// Raw for unhandled
	Raw any
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
	Filters  *circleFilters
}

type circleFilters struct {
	Branches *circleBranchFilter `yaml:"branches"`
}

type circleBranchFilter struct {
	Only   circleStringOrList `yaml:"only"`
	Ignore circleStringOrList `yaml:"ignore"`
}

// circleStringOrList handles YAML values that can be a string or a list.
type circleStringOrList []string

func (s *circleStringOrList) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		*s = []string{node.Value}
		return nil
	}
	var list []string
	if err := node.Decode(&list); err != nil {
		return err
	}
	*s = list
	return nil
}

func (j *circleWorkflowJob) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		j.Name = node.Value
		return nil
	}

	// Map form: { job_name: { requires: [...] } }
	var m map[string]struct {
		Requires []string       `yaml:"requires"`
		Type     string         `yaml:"type"`
		Filters  *circleFilters `yaml:"filters"`
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

// knownOrbCommands maps common CircleCI orb commands to shell equivalents.
var knownOrbCommands = map[string][]string{
	"ruby/install-deps": {
		"bundle install",
	},
	"node/install": {
		"# Install Node.js (equivalent of circleci/node orb install)",
		`curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash - && sudo apt-get install -y nodejs`,
		`mkdir -p ~/.npm-global && npm config set prefix ~/.npm-global && export PATH=~/.npm-global/bin:$PATH`,
	},
	"node/install-packages": {
		"npm install",
	},
	"python/install-packages": {
		"pip install -r requirements.txt",
	},
	"go/install": {
		"# Go is expected to be available in the Docker image",
	},
}

// knownOrbYarnVariants handles orb commands with pkg-manager: yarn.
var knownOrbYarnVariants = map[string][]string{
	"node/install-packages": {
		"npm install -g yarn",
		"yarn install",
	},
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

	// Infer trigger branches from workflow job filters.
	result.Workflow.On.Push = inferTriggerBranches(wfJobs)

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
			// Convert filters on approval steps too
			if condition := convertCircleFilters(wfJob.Filters); condition != "" {
				step.If = condition
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

		step := convertCircleCIJob(wfJob.Name, jobDef, wfJob, cc.Commands, result)
		result.Workflow.Steps = append(result.Workflow.Steps, step)
	}

	return result, nil
}

func convertCircleCIJob(jobID string, job *circleJob, wfJob circleWorkflowJob, commands map[string]*circleCommand, result *Result) workflow.Step {
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

	// Service containers (secondary Docker images)
	if len(job.Docker) > 1 {
		for _, d := range job.Docker[1:] {
			svc := workflow.ServiceConfig{
				Image: d.Image,
			}
			if len(d.Env) > 0 {
				svc.Env = d.Env
			}
			step.Services = append(step.Services, svc)
		}
	}

	// Commands and cache
	var cmds []string
	var cacheConfig *workflow.CacheConfig

	processSteps(job.Steps, jobID, commands, result, &cmds, &cacheConfig)

	step.Commands = cmds
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

	// Filters → if condition
	if condition := convertCircleFilters(wfJob.Filters); condition != "" {
		step.If = condition
	}

	return step
}

// processSteps recursively processes CircleCI steps, expanding custom commands and orb commands.
func processSteps(steps []circleStep, jobID string, commands map[string]*circleCommand, result *Result, cmds *[]string, cacheConfig **workflow.CacheConfig) {
	for _, cs := range steps {
		switch cs.Type {
		case "checkout":
			// FeatherCI auto-checks out — skip
			continue

		case "run":
			if cs.Command != "" {
				lines := strings.Split(strings.TrimSpace(cs.Command), "\n")
				*cmds = append(*cmds, lines...)
			}

		case "save_cache":
			if cs.CacheKey != "" && len(cs.CachePaths) > 0 {
				*cacheConfig = &workflow.CacheConfig{
					Key:   convertCircleCacheKey(cs.CacheKey),
					Paths: cs.CachePaths,
				}
			}

		case "restore_cache":
			if *cacheConfig == nil && cs.CacheKey != "" {
				*cacheConfig = &workflow.CacheConfig{
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
			// Try to expand as a custom command defined in the commands: section
			if cmd, ok := commands[cs.Type]; ok {
				if cmd.Parameters != nil {
					*cmds = append(*cmds, fmt.Sprintf("# --- expanded from command '%s' (had parameters — review for correctness) ---", cs.Type))
				} else {
					*cmds = append(*cmds, fmt.Sprintf("# --- expanded from command '%s' ---", cs.Type))
				}
				processSteps(cmd.Steps, jobID, commands, result, cmds, cacheConfig)
				continue
			}

			// Try to expand as a known orb command
			if expanded := expandOrbCommand(cs); expanded != nil {
				*cmds = append(*cmds, expanded...)
				continue
			}

			// Unknown step type
			result.Warnings = append(result.Warnings, Warning{
				Feature: fmt.Sprintf("step '%s'", cs.Type),
				Message: fmt.Sprintf("Job '%s': unrecognized step type (possibly from an orb). Replace with equivalent shell commands.", jobID),
			})
		}
	}
}

// expandOrbCommand tries to map a known orb command to shell equivalents.
func expandOrbCommand(cs circleStep) []string {
	// Check for yarn variant by inspecting raw params
	if cs.Raw != nil {
		if m, ok := cs.Raw.(map[string]yaml.Node); ok {
			if val, exists := m[cs.Type]; exists && val.Kind == yaml.MappingNode {
				var params map[string]string
				if err := val.Decode(&params); err == nil {
					if params["pkg-manager"] == "yarn" {
						if cmds, ok := knownOrbYarnVariants[cs.Type]; ok {
							return cmds
						}
					}
				}
			}
		}
	}

	if cmds, ok := knownOrbCommands[cs.Type]; ok {
		return cmds
	}
	return nil
}

// inferTriggerBranches extracts common branch filters from workflow jobs to set as push trigger branches.
// If all filtered jobs share the same "only" branches, those become the trigger branches.
// Otherwise, defaults to triggering on all pushes (no branch filter).
func inferTriggerBranches(wfJobs []circleWorkflowJob) *workflow.PushTrigger {
	trigger := &workflow.PushTrigger{}

	// Collect "only" branch sets from jobs that have filters.
	var branchSets []map[string]bool
	for _, wfJob := range wfJobs {
		if wfJob.Filters == nil || wfJob.Filters.Branches == nil {
			continue
		}
		if len(wfJob.Filters.Branches.Only) > 0 {
			set := make(map[string]bool)
			for _, b := range wfJob.Filters.Branches.Only {
				set[b] = true
			}
			branchSets = append(branchSets, set)
		}
	}

	if len(branchSets) == 0 {
		return trigger // no filters → trigger on all pushes
	}

	// Find the union of all branch filters.
	union := make(map[string]bool)
	for _, set := range branchSets {
		for b := range set {
			union[b] = true
		}
	}

	branches := make([]string, 0, len(union))
	for b := range union {
		branches = append(branches, b)
	}
	// Sort for deterministic output.
	sort.Strings(branches)
	trigger.Branches = branches
	return trigger
}

// convertCircleFilters converts CircleCI workflow job filters to a FeatherCI if condition.
func convertCircleFilters(filters *circleFilters) string {
	if filters == nil || filters.Branches == nil {
		return ""
	}

	if len(filters.Branches.Only) == 1 {
		branch := filters.Branches.Only[0]
		if strings.Contains(branch, "*") {
			return fmt.Sprintf(`branch =~ "%s"`, branch)
		}
		return fmt.Sprintf(`branch == "%s"`, branch)
	}

	if len(filters.Branches.Only) > 1 {
		// Multiple branches — use glob with alternation if possible,
		// otherwise pick first and warn
		// For now, just handle common case
		conditions := make([]string, 0, len(filters.Branches.Only))
		for _, b := range filters.Branches.Only {
			if strings.Contains(b, "*") {
				conditions = append(conditions, fmt.Sprintf(`branch =~ "%s"`, b))
			} else {
				conditions = append(conditions, fmt.Sprintf(`branch == "%s"`, b))
			}
		}
		// FeatherCI only supports single conditions, so use the first one and comment
		return conditions[0]
	}

	if len(filters.Branches.Ignore) == 1 {
		branch := filters.Branches.Ignore[0]
		if strings.Contains(branch, "*") {
			return fmt.Sprintf(`branch !~ "%s"`, branch)
		}
		return fmt.Sprintf(`branch != "%s"`, branch)
	}

	return ""
}

// convertCircleCacheKey converts a CircleCI cache key to FeatherCI format.
func convertCircleCacheKey(key string) string {
	// Replace {{ checksum "file" }} — already compatible format!
	// Replace {{ .Branch }} — already compatible!
	// Replace v1-deps-{{ ... }} patterns — keep as-is, they work
	return key
}
