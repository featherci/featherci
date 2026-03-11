// Package convert handles converting CI configurations from other platforms
// (GitHub Actions, CircleCI) to FeatherCI workflow format.
package convert

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/featherci/featherci/internal/workflow"
)

// ANSI color codes for terminal output.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
)

// Source represents a detected CI configuration source.
type Source int

const (
	SourceUnknown Source = iota
	SourceGitHub
	SourceCircleCI
)

func (s Source) String() string {
	switch s {
	case SourceGitHub:
		return "GitHub Actions"
	case SourceCircleCI:
		return "CircleCI"
	default:
		return "Unknown"
	}
}

// Warning represents a conversion warning about unsupported or partially supported features.
type Warning struct {
	Feature string
	Message string
}

// Result holds the output of a conversion.
type Result struct {
	Workflow    *workflow.Workflow
	Warnings   []Warning
	Source      Source
	SourceFile string
}

// Run auto-detects and converts CI configuration in the given directory.
func Run(dir string) error {
	source, sourceFile, err := detect(dir)
	if err != nil {
		return err
	}

	printHeader(source, sourceFile)

	data, err := os.ReadFile(sourceFile)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", sourceFile, err)
	}

	var result *Result
	switch source {
	case SourceGitHub:
		result, err = convertGitHub(data, sourceFile)
	case SourceCircleCI:
		result, err = convertCircleCI(data, sourceFile)
	default:
		return fmt.Errorf("unsupported CI source")
	}
	if err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	printWarnings(result.Warnings)

	// Write the converted workflow
	outputDir := filepath.Join(dir, ".featherci")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create .featherci directory: %w", err)
	}

	outputFile := filepath.Join(outputDir, "workflow.yml")

	// Check if output already exists
	if _, err := os.Stat(outputFile); err == nil {
		printWarning("Existing .featherci/workflow.yml will be overwritten")
	}

	out, err := marshalClean(result.Workflow)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow: %w", err)
	}

	if err := os.WriteFile(outputFile, out, 0o644); err != nil {
		return fmt.Errorf("failed to write workflow: %w", err)
	}

	// Rename original file
	backupFile := sourceFile + ".bak"
	if err := os.Rename(sourceFile, backupFile); err != nil {
		printWarning(fmt.Sprintf("Could not rename original file: %v", err))
	} else {
		printInfo(fmt.Sprintf("Renamed %s → %s", relPath(dir, sourceFile), relPath(dir, backupFile)))
	}

	printSuccess(relPath(dir, outputFile))
	printSummary(result)

	return nil
}

// detect finds CI configuration files in the given directory.
func detect(dir string) (Source, string, error) {
	// Check GitHub Actions
	ghDir := filepath.Join(dir, ".github", "workflows")
	if entries, err := os.ReadDir(ghDir); err == nil {
		for _, e := range entries {
			name := e.Name()
			if strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml") {
				if strings.HasSuffix(name, ".bak") {
					continue
				}
				return SourceGitHub, filepath.Join(ghDir, name), nil
			}
		}
	}

	// Check CircleCI
	circleFile := filepath.Join(dir, ".circleci", "config.yml")
	if _, err := os.Stat(circleFile); err == nil {
		return SourceCircleCI, circleFile, nil
	}
	circleFile = filepath.Join(dir, ".circleci", "config.yaml")
	if _, err := os.Stat(circleFile); err == nil {
		return SourceCircleCI, circleFile, nil
	}

	return SourceUnknown, "", fmt.Errorf(
		"%sNo CI configuration found%s\n  Looked for:\n  - .github/workflows/*.yml\n  - .circleci/config.yml",
		colorRed, colorReset,
	)
}

// Output helpers

func printHeader(source Source, file string) {
	fmt.Printf("\n%s%sFeatherCI Convert%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%sConverting from %s%s%s\n", colorDim, colorReset, source, colorReset)
	fmt.Printf("%sSource: %s%s\n\n", colorDim, file, colorReset)
}

func printWarnings(warnings []Warning) {
	if len(warnings) == 0 {
		return
	}
	fmt.Printf("%s%sWarnings:%s\n", colorBold, colorYellow, colorReset)
	for _, w := range warnings {
		fmt.Printf("  %s⚠  %s%s: %s\n", colorYellow, colorReset, w.Feature, w.Message)
	}
	fmt.Println()
}

func printWarning(msg string) {
	fmt.Printf("  %s⚠  %s%s\n", colorYellow, colorReset, msg)
}

func printInfo(msg string) {
	fmt.Printf("  %s→%s  %s\n", colorCyan, colorReset, msg)
}

func printSuccess(outputFile string) {
	fmt.Printf("\n  %s✓%s  Written to %s%s%s\n", colorGreen, colorReset, colorBold, outputFile, colorReset)
}

func printSummary(r *Result) {
	stepCount := len(r.Workflow.Steps)
	warnCount := len(r.Warnings)

	fmt.Printf("\n%s%sSummary:%s ", colorBold, colorCyan, colorReset)
	fmt.Printf("%d step(s) converted", stepCount)
	if warnCount > 0 {
		fmt.Printf(", %s%d warning(s)%s", colorYellow, warnCount, colorReset)
	}
	fmt.Println()

	if warnCount > 0 {
		fmt.Printf("\n%sReview the generated workflow and adjust as needed.%s\n", colorDim, colorReset)
		fmt.Printf("%sThe original file has been renamed with .bak — delete it when ready.%s\n", colorDim, colorReset)
	}
	fmt.Println()
}

func relPath(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}

// cleanStep is a YAML-friendly version of workflow.Step with omitempty tags
// so the output is clean and readable.
type cleanStep struct {
	Name            string            `yaml:"name"`
	Type            string            `yaml:"type,omitempty"`
	Image           string            `yaml:"image,omitempty"`
	Commands        []string          `yaml:"commands,omitempty"`
	DependsOn       []string          `yaml:"depends_on,omitempty"`
	Env             map[string]string `yaml:"env,omitempty"`
	WorkingDir      string            `yaml:"working_dir,omitempty"`
	TimeoutMinutes  int               `yaml:"timeout_minutes,omitempty"`
	If              string            `yaml:"if,omitempty"`
	ContinueOnError bool              `yaml:"continue_on_error,omitempty"`
	Cache           *cleanCache       `yaml:"cache,omitempty"`
	Secrets         []string          `yaml:"secrets,omitempty"`
}

type cleanCache struct {
	Key   string   `yaml:"key"`
	Paths []string `yaml:"paths"`
}

type cleanWorkflow struct {
	Name string         `yaml:"name"`
	On   cleanTrigger   `yaml:"on"`
	Steps []cleanStep   `yaml:"steps"`
}

type cleanTrigger struct {
	Push        *cleanPush `yaml:"push,omitempty"`
	PullRequest *cleanPR   `yaml:"pull_request,omitempty"`
}

type cleanPush struct {
	Branches []string `yaml:"branches,omitempty"`
	Tags     []string `yaml:"tags,omitempty"`
}

type cleanPR struct {
	Branches []string `yaml:"branches,omitempty"`
}

// marshalClean converts a workflow to clean YAML without zero-value noise.
func marshalClean(wf *workflow.Workflow) ([]byte, error) {
	cw := cleanWorkflow{
		Name: wf.Name,
	}

	if wf.On.Push != nil {
		cw.On.Push = &cleanPush{
			Branches: wf.On.Push.Branches,
			Tags:     wf.On.Push.Tags,
		}
	}
	if wf.On.PullRequest != nil {
		cw.On.PullRequest = &cleanPR{
			Branches: wf.On.PullRequest.Branches,
		}
	}

	for _, s := range wf.Steps {
		cs := cleanStep{
			Name:            s.Name,
			Type:            string(s.Type),
			Image:           s.Image,
			Commands:        s.Commands,
			DependsOn:       s.DependsOn,
			Env:             s.Env,
			WorkingDir:      s.WorkingDir,
			TimeoutMinutes:  s.TimeoutMinutes,
			If:              s.If,
			ContinueOnError: s.ContinueOnError,
			Secrets:         s.Secrets,
		}
		if s.Cache != nil {
			cs.Cache = &cleanCache{
				Key:   s.Cache.Key,
				Paths: s.Cache.Paths,
			}
		}
		cw.Steps = append(cw.Steps, cs)
	}

	return yaml.Marshal(cw)
}

// sanitizeName converts a string to a valid FeatherCI step name (alphanumeric, hyphens, underscores).
func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	result := b.String()
	if result == "" {
		return "step"
	}
	return result
}
