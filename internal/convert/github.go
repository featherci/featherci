package convert

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/featherci/featherci/internal/workflow"
)

// GitHub Actions YAML structures (subset needed for conversion).

type ghWorkflow struct {
	Name string              `yaml:"name"`
	On   ghOn                `yaml:"on"`
	Env  map[string]string   `yaml:"env"`
	Jobs map[string]*ghJob   `yaml:"jobs"`
}

// ghOn handles GitHub's flexible "on" syntax: string, list, or map.
type ghOn struct {
	Push        *ghPushTrigger `yaml:"-"`
	PullRequest *ghPRTrigger   `yaml:"-"`
	Raw         interface{}    `yaml:"-"`
}

func (o *ghOn) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		// on: push
		switch node.Value {
		case "push":
			o.Push = &ghPushTrigger{}
		case "pull_request":
			o.PullRequest = &ghPRTrigger{}
		}
	case yaml.SequenceNode:
		// on: [push, pull_request]
		var events []string
		if err := node.Decode(&events); err != nil {
			return err
		}
		for _, e := range events {
			switch e {
			case "push":
				o.Push = &ghPushTrigger{}
			case "pull_request":
				o.PullRequest = &ghPRTrigger{}
			}
		}
	case yaml.MappingNode:
		// on: { push: { branches: [...] }, ... }
		var m map[string]yaml.Node
		if err := node.Decode(&m); err != nil {
			return err
		}
		if pushNode, ok := m["push"]; ok {
			o.Push = &ghPushTrigger{}
			if pushNode.Kind != yaml.ScalarNode || pushNode.Tag != "!!null" {
				if err := pushNode.Decode(o.Push); err != nil {
					return err
				}
			}
		}
		if prNode, ok := m["pull_request"]; ok {
			o.PullRequest = &ghPRTrigger{}
			if prNode.Kind != yaml.ScalarNode || prNode.Tag != "!!null" {
				if err := prNode.Decode(o.PullRequest); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

type ghPushTrigger struct {
	Branches []string `yaml:"branches"`
	Tags     []string `yaml:"tags"`
}

type ghPRTrigger struct {
	Branches []string `yaml:"branches"`
}

type ghJob struct {
	Name            string            `yaml:"name"`
	RunsOn          interface{}       `yaml:"runs-on"`
	Container       ghContainer       `yaml:"container"`
	Steps           []ghStep          `yaml:"steps"`
	Needs           ghNeeds           `yaml:"needs"`
	Env             map[string]string `yaml:"env"`
	TimeoutMinutes  int               `yaml:"timeout-minutes"`
	ContinueOnError bool              `yaml:"continue-on-error"`
	If              string            `yaml:"if"`
	Strategy        *ghStrategy       `yaml:"strategy"`
	Services        map[string]interface{} `yaml:"services"`
}

// ghNeeds handles both string and []string forms.
type ghNeeds []string

func (n *ghNeeds) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		*n = []string{node.Value}
		return nil
	}
	var list []string
	if err := node.Decode(&list); err != nil {
		return err
	}
	*n = list
	return nil
}

type ghContainer struct {
	Image string `yaml:"image"`
}

func (c *ghContainer) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		c.Image = node.Value
		return nil
	}
	type plain ghContainer
	return node.Decode((*plain)(c))
}

type ghStep struct {
	Name            string            `yaml:"name"`
	Uses            string            `yaml:"uses"`
	Run             string            `yaml:"run"`
	With            map[string]string `yaml:"with"`
	Env             map[string]string `yaml:"env"`
	If              string            `yaml:"if"`
	ContinueOnError bool              `yaml:"continue-on-error"`
	WorkingDirectory string           `yaml:"working-directory"`
}

type ghStrategy struct {
	Matrix interface{} `yaml:"matrix"`
}

var secretRefRegex = regexp.MustCompile(`\$\{\{\s*secrets\.(\w+)\s*\}\}`)

func convertGitHub(data []byte, sourceFile string) (*Result, error) {
	var gh ghWorkflow
	if err := yaml.Unmarshal(data, &gh); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub Actions workflow: %w", err)
	}

	result := &Result{
		Source:     SourceGitHub,
		SourceFile: sourceFile,
		Workflow:   &workflow.Workflow{},
	}

	// Name
	if gh.Name != "" {
		result.Workflow.Name = gh.Name
	} else {
		result.Workflow.Name = "Converted Pipeline"
	}

	// Triggers
	if gh.On.Push != nil {
		result.Workflow.On.Push = &workflow.PushTrigger{
			Branches: gh.On.Push.Branches,
			Tags:     gh.On.Push.Tags,
		}
	}
	if gh.On.PullRequest != nil {
		result.Workflow.On.PullRequest = &workflow.PullRequestTrigger{
			Branches: gh.On.PullRequest.Branches,
		}
	}

	// Convert jobs → steps
	for jobID, job := range gh.Jobs {
		step := convertGitHubJob(jobID, job, gh.Env, result)
		result.Workflow.Steps = append(result.Workflow.Steps, step)
	}

	// Sort steps to put dependencies first (simple: order by dependency count)
	sortStepsByDeps(result.Workflow.Steps)

	return result, nil
}

func convertGitHubJob(jobID string, job *ghJob, globalEnv map[string]string, result *Result) workflow.Step {
	step := workflow.Step{
		Name: sanitizeName(jobID),
	}

	// Image: prefer container, fall back to a default with warning
	if job.Container.Image != "" {
		step.Image = job.Container.Image
	} else {
		// Try to infer from runs-on
		step.Image = "ubuntu:latest"
		runsOn := fmt.Sprintf("%v", job.RunsOn)
		result.Warnings = append(result.Warnings, Warning{
			Feature: fmt.Sprintf("runs-on: %s", runsOn),
			Message: fmt.Sprintf("FeatherCI runs steps in Docker containers. Using 'ubuntu:latest' as default image for job '%s'. You may want to choose a more specific image.", jobID),
		})
	}

	// Commands: collect all `run` steps
	var commands []string
	var usedSecrets []string
	seenSecrets := make(map[string]bool)

	for _, ghStep := range job.Steps {
		if ghStep.Uses != "" {
			handleUsesAction(ghStep, &commands, result, &step)
			continue
		}
		if ghStep.Run != "" {
			// Split multi-line run commands
			lines := strings.Split(strings.TrimSpace(ghStep.Run), "\n")
			commands = append(commands, lines...)
		}

		// Collect secrets from step env
		for _, v := range ghStep.Env {
			for _, match := range secretRefRegex.FindAllStringSubmatch(v, -1) {
				if !seenSecrets[match[1]] {
					usedSecrets = append(usedSecrets, match[1])
					seenSecrets[match[1]] = true
				}
			}
		}
	}

	step.Commands = commands

	// Dependencies
	if len(job.Needs) > 0 {
		for _, need := range job.Needs {
			step.DependsOn = append(step.DependsOn, sanitizeName(need))
		}
	}

	// Environment variables (merge global + job-level)
	env := make(map[string]string)
	for k, v := range globalEnv {
		if !secretRefRegex.MatchString(v) {
			env[k] = v
		} else {
			for _, match := range secretRefRegex.FindAllStringSubmatch(v, -1) {
				if !seenSecrets[match[1]] {
					usedSecrets = append(usedSecrets, match[1])
					seenSecrets[match[1]] = true
				}
			}
			env[k] = "$" + secretRefRegex.FindStringSubmatch(v)[1]
		}
	}
	for k, v := range job.Env {
		if !secretRefRegex.MatchString(v) {
			env[k] = v
		} else {
			for _, match := range secretRefRegex.FindAllStringSubmatch(v, -1) {
				if !seenSecrets[match[1]] {
					usedSecrets = append(usedSecrets, match[1])
					seenSecrets[match[1]] = true
				}
			}
			env[k] = "$" + secretRefRegex.FindStringSubmatch(v)[1]
		}
	}
	if len(env) > 0 {
		step.Env = env
	}

	// Secrets
	if len(usedSecrets) > 0 {
		step.Secrets = usedSecrets
	}

	// Timeout
	if job.TimeoutMinutes > 0 {
		step.TimeoutMinutes = job.TimeoutMinutes
	}

	// Continue on error
	step.ContinueOnError = job.ContinueOnError

	// Condition (best effort)
	if job.If != "" {
		converted := convertGitHubCondition(job.If)
		if converted != "" {
			step.If = converted
		} else {
			result.Warnings = append(result.Warnings, Warning{
				Feature: "if condition",
				Message: fmt.Sprintf("Could not convert condition '%s' — GitHub Actions expressions are not fully supported. Please set the 'if' field manually.", job.If),
			})
		}
	}

	// Strategy/matrix warning
	if job.Strategy != nil {
		result.Warnings = append(result.Warnings, Warning{
			Feature: "strategy/matrix",
			Message: fmt.Sprintf("Job '%s' uses a build matrix. FeatherCI does not support matrix builds — you'll need to create separate steps for each variant.", jobID),
		})
	}

	// Services warning
	if len(job.Services) > 0 {
		result.Warnings = append(result.Warnings, Warning{
			Feature: "services",
			Message: fmt.Sprintf("Job '%s' uses service containers. FeatherCI does not support service containers — consider using Docker Compose in your commands instead.", jobID),
		})
	}

	return step
}

// handleUsesAction attempts to convert common GitHub Actions to equivalent commands.
func handleUsesAction(ghStep ghStep, _ *[]string, result *Result, step *workflow.Step) {
	action := ghStep.Uses

	switch {
	case strings.HasPrefix(action, "actions/checkout"):
		// FeatherCI automatically checks out the repo — skip
		return

	case strings.HasPrefix(action, "actions/cache"):
		// Try to convert to FeatherCI cache
		if path, ok := ghStep.With["path"]; ok {
			if key, ok := ghStep.With["key"]; ok {
				step.Cache = &workflow.CacheConfig{
					Key:   convertCacheKey(key),
					Paths: strings.Split(path, "\n"),
				}
				return
			}
		}
		result.Warnings = append(result.Warnings, Warning{
			Feature: "actions/cache",
			Message: "Could not fully convert cache action. Set up caching manually in the generated workflow.",
		})

	case strings.HasPrefix(action, "actions/setup-"):
		// setup-node, setup-go, setup-python etc.
		tool := strings.TrimPrefix(action, "actions/setup-")
		tool = strings.Split(tool, "@")[0]
		result.Warnings = append(result.Warnings, Warning{
			Feature: fmt.Sprintf("actions/setup-%s", tool),
			Message: fmt.Sprintf("FeatherCI uses Docker images instead of setup actions. Choose an image that includes %s (e.g., 'node:20', 'golang:1.22', 'python:3.12').", tool),
		})

	default:
		actionName := strings.Split(action, "@")[0]
		result.Warnings = append(result.Warnings, Warning{
			Feature: fmt.Sprintf("uses: %s", actionName),
			Message: "GitHub Actions are not supported in FeatherCI. Replace with equivalent shell commands.",
		})
	}
}

// convertCacheKey does a best-effort conversion of GitHub Actions cache key to FeatherCI format.
func convertCacheKey(ghKey string) string {
	// Replace ${{ runner.os }} with a static value
	key := strings.ReplaceAll(ghKey, "${{ runner.os }}", "linux")

	// Replace ${{ hashFiles('...') }} with {{ checksum "..." }}
	hashFilesRegex := regexp.MustCompile(`\$\{\{\s*hashFiles\('([^']+)'\)\s*\}\}`)
	key = hashFilesRegex.ReplaceAllString(key, `{{ checksum "$1" }}`)

	return key
}

// convertGitHubCondition attempts to convert a GitHub Actions `if` expression to FeatherCI format.
func convertGitHubCondition(ghIf string) string {
	ghIf = strings.TrimSpace(ghIf)

	// github.ref == 'refs/heads/main' → branch == "main"
	refRegex := regexp.MustCompile(`github\.ref\s*==\s*'refs/heads/([^']+)'`)
	if m := refRegex.FindStringSubmatch(ghIf); len(m) == 2 {
		return fmt.Sprintf(`branch == "%s"`, m[1])
	}

	// github.ref != 'refs/heads/main' → branch != "main"
	refNeqRegex := regexp.MustCompile(`github\.ref\s*!=\s*'refs/heads/([^']+)'`)
	if m := refNeqRegex.FindStringSubmatch(ghIf); len(m) == 2 {
		return fmt.Sprintf(`branch != "%s"`, m[1])
	}

	// github.event_name == 'push' or similar — can't convert
	return ""
}

// sortStepsByDeps orders steps so that dependencies come first.
func sortStepsByDeps(steps []workflow.Step) {
	// Simple insertion sort by dependency count (good enough for typical workflow sizes)
	for i := 1; i < len(steps); i++ {
		for j := i; j > 0 && len(steps[j].DependsOn) < len(steps[j-1].DependsOn); j-- {
			steps[j], steps[j-1] = steps[j-1], steps[j]
		}
	}
}
