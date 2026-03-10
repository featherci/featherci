package workflow

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultWorkflowPath is the default location for workflow files.
const DefaultWorkflowPath = ".featherci/workflow.yml"

// Parser parses workflow YAML files.
type Parser struct{}

// NewParser creates a new workflow parser.
func NewParser() *Parser {
	return &Parser{}
}

// Parse parses a workflow from YAML content.
func (p *Parser) Parse(content []byte) (*Workflow, error) {
	var workflow Workflow
	if err := yaml.Unmarshal(content, &workflow); err != nil {
		return nil, fmt.Errorf("failed to parse workflow YAML: %w", err)
	}
	return &workflow, nil
}

// ParseFile parses a workflow from a file path.
func (p *Parser) ParseFile(path string) (*Workflow, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow file: %w", err)
	}
	return p.Parse(content)
}

// ParseFromRepo looks for a workflow file in the repository root.
// It checks for .featherci/workflow.yml first.
func (p *Parser) ParseFromRepo(repoRoot string) (*Workflow, error) {
	path := filepath.Join(repoRoot, DefaultWorkflowPath)
	return p.ParseFile(path)
}

// ParseAndValidate parses a workflow and validates it.
func (p *Parser) ParseAndValidate(content []byte) (*Workflow, error) {
	workflow, err := p.Parse(content)
	if err != nil {
		return nil, err
	}

	v := NewValidator()
	if err := v.Validate(workflow); err != nil {
		return nil, err
	}

	return workflow, nil
}
