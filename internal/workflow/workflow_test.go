package workflow

import (
	"testing"
)

func TestParser_Parse(t *testing.T) {
	p := NewParser()

	yaml := `
name: CI Pipeline

on:
  push:
    branches: [main, develop]
  pull_request:

steps:
  - name: lint
    image: golangci/golangci-lint:latest
    commands:
      - golangci-lint run

  - name: test
    image: golang:1.22
    commands:
      - go test -v ./...
    env:
      CGO_ENABLED: "0"

  - name: build
    image: golang:1.22
    depends_on: [lint, test]
    commands:
      - go build -o app ./cmd/app
`

	w, err := p.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if w.Name != "CI Pipeline" {
		t.Errorf("Name = %q, want %q", w.Name, "CI Pipeline")
	}

	if len(w.Steps) != 3 {
		t.Errorf("len(Steps) = %d, want %d", len(w.Steps), 3)
	}

	// Check push trigger
	if w.On.Push == nil {
		t.Fatal("On.Push is nil")
	}
	if len(w.On.Push.Branches) != 2 {
		t.Errorf("len(On.Push.Branches) = %d, want %d", len(w.On.Push.Branches), 2)
	}

	// Check PR trigger - in YAML, `pull_request:` with no value should create an empty struct
	// (enabling PR triggering on all branches). Our custom UnmarshalYAML handles this.
	if w.On.PullRequest == nil {
		t.Fatal("On.PullRequest is nil, expected empty struct")
	}

	// Check step details
	lint := w.GetStep("lint")
	if lint == nil {
		t.Fatal("Step 'lint' not found")
	}
	if lint.Image != "golangci/golangci-lint:latest" {
		t.Errorf("lint.Image = %q, want %q", lint.Image, "golangci/golangci-lint:latest")
	}

	test := w.GetStep("test")
	if test == nil {
		t.Fatal("Step 'test' not found")
	}
	if test.Env["CGO_ENABLED"] != "0" {
		t.Errorf("test.Env[CGO_ENABLED] = %q, want %q", test.Env["CGO_ENABLED"], "0")
	}

	build := w.GetStep("build")
	if build == nil {
		t.Fatal("Step 'build' not found")
	}
	if len(build.DependsOn) != 2 {
		t.Errorf("len(build.DependsOn) = %d, want %d", len(build.DependsOn), 2)
	}
}

func TestValidator_ValidWorkflow(t *testing.T) {
	v := NewValidator()

	w := &Workflow{
		Name: "Test",
		Steps: []Step{
			{Name: "step1", Image: "alpine", Commands: []string{"echo hello"}},
			{Name: "step2", Image: "alpine", Commands: []string{"echo world"}, DependsOn: []string{"step1"}},
		},
	}

	if err := v.Validate(w); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidator_NoSteps(t *testing.T) {
	v := NewValidator()

	w := &Workflow{Name: "Test", Steps: []Step{}}

	err := v.Validate(w)
	if err == nil {
		t.Error("Validate() expected error for empty steps")
	}
}

func TestValidator_DuplicateStepNames(t *testing.T) {
	v := NewValidator()

	w := &Workflow{
		Steps: []Step{
			{Name: "step1", Image: "alpine", Commands: []string{"echo 1"}},
			{Name: "step1", Image: "alpine", Commands: []string{"echo 2"}},
		},
	}

	err := v.Validate(w)
	if err == nil {
		t.Error("Validate() expected error for duplicate step names")
	}
}

func TestValidator_UnknownDependency(t *testing.T) {
	v := NewValidator()

	w := &Workflow{
		Steps: []Step{
			{Name: "step1", Image: "alpine", Commands: []string{"echo 1"}, DependsOn: []string{"unknown"}},
		},
	}

	err := v.Validate(w)
	if err == nil {
		t.Error("Validate() expected error for unknown dependency")
	}
}

func TestValidator_SelfDependency(t *testing.T) {
	v := NewValidator()

	w := &Workflow{
		Steps: []Step{
			{Name: "step1", Image: "alpine", Commands: []string{"echo 1"}, DependsOn: []string{"step1"}},
		},
	}

	err := v.Validate(w)
	if err == nil {
		t.Error("Validate() expected error for self dependency")
	}
}

func TestValidator_CircularDependency(t *testing.T) {
	v := NewValidator()

	w := &Workflow{
		Steps: []Step{
			{Name: "step1", Image: "alpine", Commands: []string{"echo 1"}, DependsOn: []string{"step3"}},
			{Name: "step2", Image: "alpine", Commands: []string{"echo 2"}, DependsOn: []string{"step1"}},
			{Name: "step3", Image: "alpine", Commands: []string{"echo 3"}, DependsOn: []string{"step2"}},
		},
	}

	err := v.Validate(w)
	if err == nil {
		t.Error("Validate() expected error for circular dependency")
	}
}

func TestValidator_MissingImage(t *testing.T) {
	v := NewValidator()

	w := &Workflow{
		Steps: []Step{
			{Name: "step1", Commands: []string{"echo hello"}},
		},
	}

	err := v.Validate(w)
	if err == nil {
		t.Error("Validate() expected error for missing image")
	}
}

func TestValidator_MissingCommands(t *testing.T) {
	v := NewValidator()

	w := &Workflow{
		Steps: []Step{
			{Name: "step1", Image: "alpine"},
		},
	}

	err := v.Validate(w)
	if err == nil {
		t.Error("Validate() expected error for missing commands")
	}
}

func TestValidator_ApprovalStep(t *testing.T) {
	v := NewValidator()

	w := &Workflow{
		Steps: []Step{
			{Name: "build", Image: "alpine", Commands: []string{"echo build"}},
			{Name: "approve", Type: StepTypeApproval, DependsOn: []string{"build"}},
			{Name: "deploy", Image: "alpine", Commands: []string{"echo deploy"}, DependsOn: []string{"approve"}},
		},
	}

	if err := v.Validate(w); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidator_ApprovalWithImage(t *testing.T) {
	v := NewValidator()

	w := &Workflow{
		Steps: []Step{
			{Name: "approve", Type: StepTypeApproval, Image: "alpine"},
		},
	}

	err := v.Validate(w)
	if err == nil {
		t.Error("Validate() expected error for approval step with image")
	}
}

func TestValidator_ApprovalWithCommands(t *testing.T) {
	v := NewValidator()

	w := &Workflow{
		Steps: []Step{
			{Name: "approve", Type: StepTypeApproval, Commands: []string{"echo"}},
		},
	}

	err := v.Validate(w)
	if err == nil {
		t.Error("Validate() expected error for approval step with commands")
	}
}

func TestValidator_InvalidStepName(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		wantErr bool
	}{
		{"valid-name", false},
		{"valid_name", false},
		{"ValidName123", false},
		{"invalid name", true},
		{"invalid.name", true},
		{"invalid/name", true},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &Workflow{
				Steps: []Step{
					{Name: tt.name, Image: "alpine", Commands: []string{"echo"}},
				},
			}
			err := v.Validate(w)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_NegativeTimeout(t *testing.T) {
	v := NewValidator()

	w := &Workflow{
		Steps: []Step{
			{Name: "step1", Image: "alpine", Commands: []string{"echo"}, TimeoutMinutes: -1},
		},
	}

	err := v.Validate(w)
	if err == nil {
		t.Error("Validate() expected error for negative timeout")
	}
}

func TestValidator_AbsoluteWorkingDir(t *testing.T) {
	v := NewValidator()

	w := &Workflow{
		Steps: []Step{
			{Name: "step1", Image: "alpine", Commands: []string{"echo"}, WorkingDir: "/absolute/path"},
		},
	}

	err := v.Validate(w)
	if err == nil {
		t.Error("Validate() expected error for absolute working dir")
	}
}

func TestValidator_CacheMissingPaths(t *testing.T) {
	v := NewValidator()

	w := &Workflow{
		Steps: []Step{
			{Name: "step1", Image: "alpine", Commands: []string{"echo"}, Cache: &CacheConfig{Key: "test"}},
		},
	}

	err := v.Validate(w)
	if err == nil {
		t.Error("Validate() expected error for cache without paths")
	}
}

func TestValidator_CacheMissingKey(t *testing.T) {
	v := NewValidator()

	w := &Workflow{
		Steps: []Step{
			{Name: "step1", Image: "alpine", Commands: []string{"echo"}, Cache: &CacheConfig{Paths: []string{".cache"}}},
		},
	}

	err := v.Validate(w)
	if err == nil {
		t.Error("Validate() expected error for cache without key")
	}
}

func TestWorkflow_ExecutionGroups(t *testing.T) {
	w := &Workflow{
		Steps: []Step{
			{Name: "lint", Image: "alpine", Commands: []string{"lint"}},
			{Name: "test", Image: "alpine", Commands: []string{"test"}},
			{Name: "build", Image: "alpine", Commands: []string{"build"}, DependsOn: []string{"lint", "test"}},
			{Name: "deploy", Image: "alpine", Commands: []string{"deploy"}, DependsOn: []string{"build"}},
		},
	}

	groups := w.ExecutionGroups()

	if len(groups) != 3 {
		t.Fatalf("len(ExecutionGroups()) = %d, want %d", len(groups), 3)
	}

	// First group: lint and test (no deps)
	if len(groups[0].Steps) != 2 {
		t.Errorf("len(groups[0].Steps) = %d, want %d", len(groups[0].Steps), 2)
	}

	// Second group: build
	if len(groups[1].Steps) != 1 {
		t.Errorf("len(groups[1].Steps) = %d, want %d", len(groups[1].Steps), 1)
	}
	if groups[1].Steps[0].Name != "build" {
		t.Errorf("groups[1].Steps[0].Name = %q, want %q", groups[1].Steps[0].Name, "build")
	}

	// Third group: deploy
	if len(groups[2].Steps) != 1 {
		t.Errorf("len(groups[2].Steps) = %d, want %d", len(groups[2].Steps), 1)
	}
	if groups[2].Steps[0].Name != "deploy" {
		t.Errorf("groups[2].Steps[0].Name = %q, want %q", groups[2].Steps[0].Name, "deploy")
	}
}

func TestWorkflow_ReadySteps(t *testing.T) {
	w := &Workflow{
		Steps: []Step{
			{Name: "lint", Image: "alpine", Commands: []string{"lint"}},
			{Name: "test", Image: "alpine", Commands: []string{"test"}},
			{Name: "build", Image: "alpine", Commands: []string{"build"}, DependsOn: []string{"lint", "test"}},
		},
	}

	// Initially, lint and test should be ready
	ready := w.ReadySteps(map[string]bool{})
	if len(ready) != 2 {
		t.Errorf("ReadySteps({}) = %d steps, want 2", len(ready))
	}

	// After lint completes, only test should still be running, build not ready
	ready = w.ReadySteps(map[string]bool{"lint": true})
	if len(ready) != 1 {
		t.Errorf("ReadySteps({lint}) = %d steps, want 1", len(ready))
	}

	// After both complete, build should be ready
	ready = w.ReadySteps(map[string]bool{"lint": true, "test": true})
	if len(ready) != 1 {
		t.Errorf("ReadySteps({lint, test}) = %d steps, want 1", len(ready))
	}
	if ready[0].Name != "build" {
		t.Errorf("ReadySteps({lint, test})[0].Name = %q, want %q", ready[0].Name, "build")
	}
}

func TestWorkflow_RootSteps(t *testing.T) {
	w := &Workflow{
		Steps: []Step{
			{Name: "lint", Image: "alpine", Commands: []string{"lint"}},
			{Name: "test", Image: "alpine", Commands: []string{"test"}},
			{Name: "build", Image: "alpine", Commands: []string{"build"}, DependsOn: []string{"lint", "test"}},
		},
	}

	roots := w.RootSteps()
	if len(roots) != 2 {
		t.Errorf("len(RootSteps()) = %d, want 2", len(roots))
	}
}

func TestWorkflow_Dependents(t *testing.T) {
	w := &Workflow{
		Steps: []Step{
			{Name: "lint", Image: "alpine", Commands: []string{"lint"}},
			{Name: "build", Image: "alpine", Commands: []string{"build"}, DependsOn: []string{"lint"}},
			{Name: "deploy", Image: "alpine", Commands: []string{"deploy"}, DependsOn: []string{"lint"}},
		},
	}

	deps := w.Dependents("lint")
	if len(deps) != 2 {
		t.Errorf("len(Dependents(lint)) = %d, want 2", len(deps))
	}
}

func TestWorkflow_TopologicalOrder(t *testing.T) {
	w := &Workflow{
		Steps: []Step{
			{Name: "deploy", Image: "alpine", Commands: []string{"deploy"}, DependsOn: []string{"build"}},
			{Name: "build", Image: "alpine", Commands: []string{"build"}, DependsOn: []string{"test"}},
			{Name: "test", Image: "alpine", Commands: []string{"test"}},
		},
	}

	order := w.TopologicalOrder()
	if len(order) != 3 {
		t.Fatalf("len(TopologicalOrder()) = %d, want 3", len(order))
	}

	// test should come before build, build before deploy
	indexOf := func(name string) int {
		for i, s := range order {
			if s.Name == name {
				return i
			}
		}
		return -1
	}

	if indexOf("test") > indexOf("build") {
		t.Error("test should come before build in topological order")
	}
	if indexOf("build") > indexOf("deploy") {
		t.Error("build should come before deploy in topological order")
	}
}

func TestWorkflow_ShouldTrigger_Push(t *testing.T) {
	tests := []struct {
		name     string
		workflow *Workflow
		branch   string
		tag      string
		want     bool
	}{
		{
			name:     "no trigger config - triggers on all",
			workflow: &Workflow{},
			branch:   "main",
			want:     true,
		},
		{
			name: "push trigger matches branch",
			workflow: &Workflow{
				On: TriggerConfig{
					Push: &PushTrigger{Branches: []string{"main", "develop"}},
				},
			},
			branch: "main",
			want:   true,
		},
		{
			name: "push trigger doesn't match branch",
			workflow: &Workflow{
				On: TriggerConfig{
					Push: &PushTrigger{Branches: []string{"main"}},
				},
			},
			branch: "feature/test",
			want:   false,
		},
		{
			name: "push trigger with glob pattern",
			workflow: &Workflow{
				On: TriggerConfig{
					Push: &PushTrigger{Branches: []string{"feature/*"}},
				},
			},
			branch: "feature/test",
			want:   true,
		},
		{
			name: "tag trigger matches",
			workflow: &Workflow{
				On: TriggerConfig{
					Push: &PushTrigger{Tags: []string{"v*"}},
				},
			},
			tag:  "v1.0.0",
			want: true,
		},
		{
			name: "tag trigger doesn't match",
			workflow: &Workflow{
				On: TriggerConfig{
					Push: &PushTrigger{Tags: []string{"release-*"}},
				},
			},
			tag:  "v1.0.0",
			want: false,
		},
		{
			name: "only PR trigger - no push",
			workflow: &Workflow{
				On: TriggerConfig{
					PullRequest: &PullRequestTrigger{},
				},
			},
			branch: "main",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.workflow.ShouldTrigger("push", tt.branch, tt.tag)
			if got != tt.want {
				t.Errorf("ShouldTrigger(push) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorkflow_ShouldTrigger_PullRequest(t *testing.T) {
	tests := []struct {
		name         string
		workflow     *Workflow
		targetBranch string
		want         bool
	}{
		{
			name:         "no PR trigger",
			workflow:     &Workflow{},
			targetBranch: "main",
			want:         false,
		},
		{
			name: "PR trigger no branch filter",
			workflow: &Workflow{
				On: TriggerConfig{
					PullRequest: &PullRequestTrigger{},
				},
			},
			targetBranch: "main",
			want:         true,
		},
		{
			name: "PR trigger matches branch",
			workflow: &Workflow{
				On: TriggerConfig{
					PullRequest: &PullRequestTrigger{Branches: []string{"main"}},
				},
			},
			targetBranch: "main",
			want:         true,
		},
		{
			name: "PR trigger doesn't match branch",
			workflow: &Workflow{
				On: TriggerConfig{
					PullRequest: &PullRequestTrigger{Branches: []string{"main"}},
				},
			},
			targetBranch: "develop",
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.workflow.ShouldTrigger("pull_request", tt.targetBranch, "")
			if got != tt.want {
				t.Errorf("ShouldTrigger(pull_request) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		value   string
		want    bool
	}{
		// Exact matches
		{"main", "main", true},
		{"main", "develop", false},

		// Single wildcard
		{"feature/*", "feature/test", true},
		{"feature/*", "feature/foo/bar", false}, // * doesn't match /
		{"v*", "v1.0.0", true},
		{"v*", "version", true}, // v* matches anything starting with v

		// Double wildcard
		{"feature/**", "feature/test", true},
		{"feature/**", "feature/foo/bar", true},
		{"**/test", "foo/bar/test", true},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.value, func(t *testing.T) {
			got := matchGlob(tt.pattern, tt.value)
			if got != tt.want {
				t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.pattern, tt.value, got, tt.want)
			}
		})
	}
}

func TestStep_Methods(t *testing.T) {
	// Test IsApproval
	approval := Step{Type: StepTypeApproval}
	if !approval.IsApproval() {
		t.Error("IsApproval() = false, want true")
	}

	// Test IsCommand
	command := Step{Type: StepTypeCommand}
	if !command.IsCommand() {
		t.Error("IsCommand() = false, want true for StepTypeCommand")
	}

	defaultStep := Step{}
	if !defaultStep.IsCommand() {
		t.Error("IsCommand() = false, want true for empty type")
	}

	// Test GetTimeout
	step := Step{}
	if step.GetTimeout() != 60 {
		t.Errorf("GetTimeout() = %d, want 60 (default)", step.GetTimeout())
	}

	step.TimeoutMinutes = 30
	if step.GetTimeout() != 30 {
		t.Errorf("GetTimeout() = %d, want 30", step.GetTimeout())
	}

	// Test HasDependencies
	noDeps := Step{}
	if noDeps.HasDependencies() {
		t.Error("HasDependencies() = true, want false")
	}

	withDeps := Step{DependsOn: []string{"other"}}
	if !withDeps.HasDependencies() {
		t.Error("HasDependencies() = false, want true")
	}
}

func TestParser_ParseAndValidate(t *testing.T) {
	p := NewParser()

	validYAML := `
name: Test
steps:
  - name: test
    image: alpine
    commands:
      - echo hello
`

	w, err := p.ParseAndValidate([]byte(validYAML))
	if err != nil {
		t.Fatalf("ParseAndValidate() error = %v", err)
	}
	if w.Name != "Test" {
		t.Errorf("Name = %q, want %q", w.Name, "Test")
	}

	// Test invalid YAML
	invalidYAML := `
name: Test
steps:
  - name: test
`
	_, err = p.ParseAndValidate([]byte(invalidYAML))
	if err == nil {
		t.Error("ParseAndValidate() expected error for missing image")
	}
}
