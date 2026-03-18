package convert

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/featherci/featherci/internal/workflow"
)

func TestDetect_GitHub(t *testing.T) {
	dir := t.TempDir()
	ghDir := filepath.Join(dir, ".github", "workflows")
	os.MkdirAll(ghDir, 0o755)
	os.WriteFile(filepath.Join(ghDir, "ci.yml"), []byte("name: CI"), 0o644)

	source, file, err := detect(dir)
	if err != nil {
		t.Fatal(err)
	}
	if source != SourceGitHub {
		t.Errorf("expected SourceGitHub, got %v", source)
	}
	if filepath.Base(file) != "ci.yml" {
		t.Errorf("expected ci.yml, got %s", filepath.Base(file))
	}
}

func TestDetect_CircleCI(t *testing.T) {
	dir := t.TempDir()
	circleDir := filepath.Join(dir, ".circleci")
	os.MkdirAll(circleDir, 0o755)
	os.WriteFile(filepath.Join(circleDir, "config.yml"), []byte("version: 2.1"), 0o644)

	source, file, err := detect(dir)
	if err != nil {
		t.Fatal(err)
	}
	if source != SourceCircleCI {
		t.Errorf("expected SourceCircleCI, got %v", source)
	}
	if filepath.Base(file) != "config.yml" {
		t.Errorf("expected config.yml, got %s", filepath.Base(file))
	}
}

func TestDetect_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, _, err := detect(dir)
	if err == nil {
		t.Fatal("expected error for missing CI config")
	}
}

func TestDetect_SkipsBakFiles(t *testing.T) {
	dir := t.TempDir()
	ghDir := filepath.Join(dir, ".github", "workflows")
	os.MkdirAll(ghDir, 0o755)
	os.WriteFile(filepath.Join(ghDir, "ci.yml.bak"), []byte("name: CI"), 0o644)

	_, _, err := detect(dir)
	if err == nil {
		t.Fatal("expected error — .bak files should be skipped")
	}
}

func TestConvertGitHub_Basic(t *testing.T) {
	input := `
name: CI
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    container:
      image: golang:1.22
    steps:
      - uses: actions/checkout@v4
      - run: go test ./...
      - run: go vet ./...

  build:
    runs-on: ubuntu-latest
    container: golang:1.22
    needs: [test]
    steps:
      - uses: actions/checkout@v4
      - run: go build -o app .
`

	result, err := convertGitHub([]byte(input), "test.yml")
	if err != nil {
		t.Fatal(err)
	}

	if result.Workflow.Name != "CI" {
		t.Errorf("expected name 'CI', got '%s'", result.Workflow.Name)
	}

	if result.Workflow.On.Push == nil {
		t.Fatal("expected push trigger")
	}
	if len(result.Workflow.On.Push.Branches) != 1 || result.Workflow.On.Push.Branches[0] != "main" {
		t.Errorf("unexpected push branches: %v", result.Workflow.On.Push.Branches)
	}

	if result.Workflow.On.PullRequest == nil {
		t.Fatal("expected pull_request trigger")
	}

	// Find the test step
	var testStep, buildStep *workflow.Step
	for i := range result.Workflow.Steps {
		switch result.Workflow.Steps[i].Name {
		case "test":
			testStep = &result.Workflow.Steps[i]
		case "build":
			buildStep = &result.Workflow.Steps[i]
		}
	}

	if testStep == nil {
		t.Fatal("expected 'test' step")
	}
	if testStep.Image != "golang:1.22" {
		t.Errorf("expected image 'golang:1.22', got '%s'", testStep.Image)
	}
	if len(testStep.Commands) != 2 {
		t.Errorf("expected 2 commands, got %d", len(testStep.Commands))
	}

	if buildStep == nil {
		t.Fatal("expected 'build' step")
	}
	if len(buildStep.DependsOn) != 1 || buildStep.DependsOn[0] != "test" {
		t.Errorf("expected depends_on [test], got %v", buildStep.DependsOn)
	}
}

func TestConvertGitHub_WithCache(t *testing.T) {
	input := `
name: CI
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    container: node:20
    steps:
      - uses: actions/checkout@v4
      - uses: actions/cache@v3
        with:
          path: node_modules
          key: ${{ runner.os }}-node-${{ hashFiles('package-lock.json') }}
      - run: npm test
`

	result, err := convertGitHub([]byte(input), "test.yml")
	if err != nil {
		t.Fatal(err)
	}

	step := result.Workflow.Steps[0]
	if step.Cache == nil {
		t.Fatal("expected cache config")
	}
	if step.Cache.Paths[0] != "node_modules" {
		t.Errorf("expected cache path 'node_modules', got '%s'", step.Cache.Paths[0])
	}
	// Check hashFiles was converted to checksum
	expected := `linux-node-{{ checksum "package-lock.json" }}`
	if step.Cache.Key != expected {
		t.Errorf("expected cache key '%s', got '%s'", expected, step.Cache.Key)
	}
}

func TestConvertGitHub_Warnings(t *testing.T) {
	input := `
name: CI
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go: [1.21, 1.22]
    services:
      postgres:
        image: postgres:15
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go }}
      - uses: some-org/custom-action@v1
      - run: go test ./...
`

	result, err := convertGitHub([]byte(input), "test.yml")
	if err != nil {
		t.Fatal(err)
	}

	// Should have warnings for: runs-on, strategy/matrix, services, setup-go, custom-action
	warnFeatures := make(map[string]bool)
	for _, w := range result.Warnings {
		warnFeatures[w.Feature] = true
	}

	for _, expected := range []string{"strategy/matrix", "services"} {
		if !warnFeatures[expected] {
			t.Errorf("expected warning for '%s', warnings: %v", expected, result.Warnings)
		}
	}

	// Check setup-go warning exists
	found := false
	for _, w := range result.Warnings {
		if w.Feature == "actions/setup-go" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected warning for actions/setup-go")
	}
}

func TestConvertGitHub_Secrets(t *testing.T) {
	input := `
name: Deploy
on: push
env:
  API_KEY: ${{ secrets.API_KEY }}
jobs:
  deploy:
    runs-on: ubuntu-latest
    container: alpine
    env:
      DB_PASSWORD: ${{ secrets.DB_PASSWORD }}
    steps:
      - run: deploy.sh
`

	result, err := convertGitHub([]byte(input), "test.yml")
	if err != nil {
		t.Fatal(err)
	}

	step := result.Workflow.Steps[0]
	if len(step.Secrets) < 2 {
		t.Errorf("expected at least 2 secrets, got %d: %v", len(step.Secrets), step.Secrets)
	}

	secretSet := make(map[string]bool)
	for _, s := range step.Secrets {
		secretSet[s] = true
	}
	if !secretSet["API_KEY"] || !secretSet["DB_PASSWORD"] {
		t.Errorf("expected secrets API_KEY and DB_PASSWORD, got %v", step.Secrets)
	}
}

func TestConvertGitHub_Condition(t *testing.T) {
	input := `
name: Deploy
on: push
jobs:
  deploy:
    runs-on: ubuntu-latest
    container: alpine
    if: github.ref == 'refs/heads/main'
    steps:
      - run: deploy.sh
`

	result, err := convertGitHub([]byte(input), "test.yml")
	if err != nil {
		t.Fatal(err)
	}

	step := result.Workflow.Steps[0]
	if step.If != `branch == "main"` {
		t.Errorf("expected condition 'branch == \"main\"', got '%s'", step.If)
	}
}

func TestConvertCircleCI_Basic(t *testing.T) {
	input := `
version: 2.1
jobs:
  test:
    docker:
      - image: golang:1.22
    steps:
      - checkout
      - run: go test ./...
      - run:
          name: Vet
          command: go vet ./...

  build:
    docker:
      - image: golang:1.22
    steps:
      - checkout
      - run: go build -o app .

workflows:
  main:
    jobs:
      - test
      - build:
          requires:
            - test
`

	result, err := convertCircleCI([]byte(input), "config.yml")
	if err != nil {
		t.Fatal(err)
	}

	if result.Workflow.Name != "main" {
		t.Errorf("expected name 'main', got '%s'", result.Workflow.Name)
	}

	// Find steps
	var testStep, buildStep *workflow.Step
	for i := range result.Workflow.Steps {
		switch result.Workflow.Steps[i].Name {
		case "test":
			testStep = &result.Workflow.Steps[i]
		case "build":
			buildStep = &result.Workflow.Steps[i]
		}
	}

	if testStep == nil {
		t.Fatal("expected 'test' step")
	}
	if testStep.Image != "golang:1.22" {
		t.Errorf("expected image 'golang:1.22', got '%s'", testStep.Image)
	}
	if len(testStep.Commands) != 2 {
		t.Errorf("expected 2 commands, got %d: %v", len(testStep.Commands), testStep.Commands)
	}

	if buildStep == nil {
		t.Fatal("expected 'build' step")
	}
	if len(buildStep.DependsOn) != 1 || buildStep.DependsOn[0] != "test" {
		t.Errorf("expected depends_on [test], got %v", buildStep.DependsOn)
	}
}

func TestConvertCircleCI_Approval(t *testing.T) {
	input := `
version: 2.1
jobs:
  test:
    docker:
      - image: golang:1.22
    steps:
      - checkout
      - run: go test ./...

  deploy:
    docker:
      - image: alpine
    steps:
      - run: echo deploying

workflows:
  pipeline:
    jobs:
      - test
      - approve-deploy:
          type: approval
          requires:
            - test
      - deploy:
          requires:
            - approve-deploy
`

	result, err := convertCircleCI([]byte(input), "config.yml")
	if err != nil {
		t.Fatal(err)
	}

	var approvalStep *workflow.Step
	for i := range result.Workflow.Steps {
		if result.Workflow.Steps[i].Name == "approve-deploy" {
			approvalStep = &result.Workflow.Steps[i]
			break
		}
	}

	if approvalStep == nil {
		t.Fatal("expected approval step")
	}
	if approvalStep.Type != workflow.StepTypeApproval {
		t.Errorf("expected type 'approval', got '%s'", approvalStep.Type)
	}
	if len(approvalStep.DependsOn) != 1 || approvalStep.DependsOn[0] != "test" {
		t.Errorf("expected depends_on [test], got %v", approvalStep.DependsOn)
	}
}

func TestConvertCircleCI_Cache(t *testing.T) {
	input := `
version: 2.1
jobs:
  test:
    docker:
      - image: node:20
    steps:
      - checkout
      - restore_cache:
          keys:
            - v1-deps-{{ checksum "package-lock.json" }}
      - run: npm install
      - save_cache:
          key: v1-deps-{{ checksum "package-lock.json" }}
          paths:
            - node_modules
      - run: npm test

workflows:
  build:
    jobs:
      - test
`

	result, err := convertCircleCI([]byte(input), "config.yml")
	if err != nil {
		t.Fatal(err)
	}

	step := result.Workflow.Steps[0]
	if step.Cache == nil {
		t.Fatal("expected cache config")
	}
	if step.Cache.Paths[0] != "node_modules" {
		t.Errorf("expected cache path 'node_modules', got '%s'", step.Cache.Paths[0])
	}
}

func TestConvertCircleCI_Warnings(t *testing.T) {
	input := `
version: 2.1
orbs:
  node: circleci/node@5.0
jobs:
  test:
    machine:
      image: ubuntu-2204:current
    parallelism: 4
    steps:
      - checkout
      - run: go test ./...
      - store_artifacts:
          path: coverage
      - persist_to_workspace:
          root: .
          paths: [bin/]

  deploy:
    docker:
      - image: alpine
    steps:
      - attach_workspace:
          at: .
      - run: echo deploy

workflows:
  main:
    jobs:
      - test
      - deploy:
          requires: [test]
          filters:
            branches:
              only: main
`

	result, err := convertCircleCI([]byte(input), "config.yml")
	if err != nil {
		t.Fatal(err)
	}

	warnFeatures := make(map[string]bool)
	for _, w := range result.Warnings {
		warnFeatures[w.Feature] = true
	}

	for _, expected := range []string{
		"machine executor",
		"parallelism",
		"store_artifacts",
		"persist_to_workspace",
		"attach_workspace",
	} {
		if !warnFeatures[expected] {
			t.Errorf("expected warning for '%s'", expected)
		}
	}

	// Orbs should NOT produce a warning (common orbs are auto-expanded)
	if warnFeatures["orbs"] {
		t.Error("orbs should not produce a warning")
	}

	// Filters should be converted to an if condition, not a warning
	var deployStep *workflow.Step
	for i := range result.Workflow.Steps {
		if result.Workflow.Steps[i].Name == "deploy" {
			deployStep = &result.Workflow.Steps[i]
			break
		}
	}
	if deployStep == nil {
		t.Fatal("expected 'deploy' step")
	}
	if deployStep.If != `branch == "main"` {
		t.Errorf("expected if condition 'branch == \"main\"', got '%s'", deployStep.If)
	}

	// Trigger branches should be inferred from the "only: main" filter
	if result.Workflow.On.Push == nil {
		t.Fatal("expected push trigger")
	}
	if len(result.Workflow.On.Push.Branches) != 1 || result.Workflow.On.Push.Branches[0] != "main" {
		t.Errorf("expected push trigger branches [main], got %v", result.Workflow.On.Push.Branches)
	}
}

func TestConvertCircleCI_CustomCommands(t *testing.T) {
	input := `
version: 2.1
commands:
  setup_creds:
    description: "Write credentials"
    steps:
      - run:
          name: Write creds
          command: |
            echo $TOKEN > config/token.key
            chmod 600 config/token.key
      - run:
          name: SSH key
          command: echo $SSH_KEY | base64 -d > ~/.ssh/id_ed25519

  deploy_sequence:
    description: "Deploy steps"
    steps:
      - run:
          name: Migrate
          command: ./bin/kamal app exec "rails db:migrate"
      - run:
          name: Deploy
          command: ./bin/kamal deploy

jobs:
  deploy:
    docker:
      - image: ruby:3.2
    steps:
      - checkout
      - setup_creds
      - deploy_sequence

workflows:
  main:
    jobs:
      - deploy
`

	result, err := convertCircleCI([]byte(input), "config.yml")
	if err != nil {
		t.Fatal(err)
	}

	step := result.Workflow.Steps[0]

	// Should have inlined all commands from both custom commands
	expectedCommands := []string{
		"# --- expanded from command 'setup_creds' ---",
		"echo $TOKEN > config/token.key",
		"chmod 600 config/token.key",
		"echo $SSH_KEY | base64 -d > ~/.ssh/id_ed25519",
		"# --- expanded from command 'deploy_sequence' ---",
		"./bin/kamal app exec \"rails db:migrate\"",
		"./bin/kamal deploy",
	}

	if len(step.Commands) != len(expectedCommands) {
		t.Fatalf("expected %d commands, got %d: %v", len(expectedCommands), len(step.Commands), step.Commands)
	}

	for i, expected := range expectedCommands {
		if step.Commands[i] != expected {
			t.Errorf("command[%d]: expected %q, got %q", i, expected, step.Commands[i])
		}
	}
}

func TestConvertCircleCI_OrbCommands(t *testing.T) {
	input := `
version: 2.1
orbs:
  ruby: circleci/ruby@2.5
  node: circleci/node@7.1
jobs:
  test:
    docker:
      - image: cimg/ruby:3.4
    steps:
      - checkout
      - ruby/install-deps
      - node/install
      - node/install-packages:
          pkg-manager: yarn
      - run: bundle exec rspec

workflows:
  main:
    jobs:
      - test
`

	result, err := convertCircleCI([]byte(input), "config.yml")
	if err != nil {
		t.Fatal(err)
	}

	step := result.Workflow.Steps[0]

	// Orb commands should be expanded to shell equivalents
	found := map[string]bool{}
	for _, cmd := range step.Commands {
		found[cmd] = true
	}

	if !found["bundle install"] {
		t.Error("expected 'bundle install' from ruby/install-deps expansion")
	}
	if !found[`curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash - && sudo apt-get install -y nodejs`] {
		t.Error("expected node install command from node/install expansion")
	}
	if !found["yarn install"] {
		t.Error("expected 'yarn install' from node/install-packages with yarn")
	}
	if !found["bundle exec rspec"] {
		t.Error("expected 'bundle exec rspec' from run step")
	}

	// Should NOT have warnings for the known orb commands
	for _, w := range result.Warnings {
		if w.Feature == "step 'ruby/install-deps'" || w.Feature == "step 'node/install'" || w.Feature == "step 'node/install-packages'" {
			t.Errorf("should not warn about known orb command: %s", w.Feature)
		}
	}
}

func TestConvertCircleCI_Filters(t *testing.T) {
	input := `
version: 2.1
jobs:
  deploy:
    docker:
      - image: alpine
    steps:
      - run: echo deploy

  staging:
    docker:
      - image: alpine
    steps:
      - run: echo staging

workflows:
  main:
    jobs:
      - deploy:
          filters:
            branches:
              only: master
      - staging:
          filters:
            branches:
              ignore: main
`

	result, err := convertCircleCI([]byte(input), "config.yml")
	if err != nil {
		t.Fatal(err)
	}

	var deployStep, stagingStep *workflow.Step
	for i := range result.Workflow.Steps {
		switch result.Workflow.Steps[i].Name {
		case "deploy":
			deployStep = &result.Workflow.Steps[i]
		case "staging":
			stagingStep = &result.Workflow.Steps[i]
		}
	}

	if deployStep == nil || deployStep.If != `branch == "master"` {
		t.Errorf("deploy: expected if 'branch == \"master\"', got '%s'", deployStep.If)
	}
	if stagingStep == nil || stagingStep.If != `branch != "main"` {
		t.Errorf("staging: expected if 'branch != \"main\"', got '%s'", stagingStep.If)
	}
}

func TestConvertCircleCI_ServiceContainers(t *testing.T) {
	input := `
version: 2.1
jobs:
  test:
    docker:
      - image: cimg/ruby:3.4
      - image: cimg/mysql:8.0
    steps:
      - checkout
      - run: bundle exec rspec

workflows:
  main:
    jobs:
      - test
`

	result, err := convertCircleCI([]byte(input), "config.yml")
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Workflow.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Workflow.Steps))
	}

	step := result.Workflow.Steps[0]
	if len(step.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(step.Services))
	}
	if step.Services[0].Image != "cimg/mysql:8.0" {
		t.Errorf("expected service image cimg/mysql:8.0, got %s", step.Services[0].Image)
	}

	// Should NOT have a service containers warning anymore
	for _, w := range result.Warnings {
		if w.Feature == "service containers" {
			t.Error("unexpected warning for service containers — should emit services instead")
		}
	}
}

func TestRun_EndToEnd_GitHub(t *testing.T) {
	dir := t.TempDir()

	// Create GitHub Actions workflow
	ghDir := filepath.Join(dir, ".github", "workflows")
	os.MkdirAll(ghDir, 0o755)
	input := `
name: CI
on:
  push:
    branches: [main]
jobs:
  test:
    runs-on: ubuntu-latest
    container: golang:1.22
    steps:
      - uses: actions/checkout@v4
      - run: go test ./...
`
	os.WriteFile(filepath.Join(ghDir, "ci.yml"), []byte(input), 0o644)

	// Run conversion
	if err := Run(dir); err != nil {
		t.Fatal(err)
	}

	// Check output file exists
	outputFile := filepath.Join(dir, ".featherci", "workflow.yml")
	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatal("output file not created:", err)
	}

	// Verify it's valid YAML
	var wf workflow.Workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatal("output is not valid workflow YAML:", err)
	}

	if wf.Name != "CI" {
		t.Errorf("expected name 'CI', got '%s'", wf.Name)
	}

	// Check original was renamed
	if _, err := os.Stat(filepath.Join(ghDir, "ci.yml")); !os.IsNotExist(err) {
		t.Error("original file should have been renamed")
	}
	if _, err := os.Stat(filepath.Join(ghDir, "ci.yml.bak")); err != nil {
		t.Error("backup file should exist:", err)
	}
}

func TestRun_EndToEnd_CircleCI(t *testing.T) {
	dir := t.TempDir()

	// Create CircleCI config
	circleDir := filepath.Join(dir, ".circleci")
	os.MkdirAll(circleDir, 0o755)
	input := `
version: 2.1
jobs:
  test:
    docker:
      - image: golang:1.22
    steps:
      - checkout
      - run: go test ./...

workflows:
  main:
    jobs:
      - test
`
	os.WriteFile(filepath.Join(circleDir, "config.yml"), []byte(input), 0o644)

	if err := Run(dir); err != nil {
		t.Fatal(err)
	}

	outputFile := filepath.Join(dir, ".featherci", "workflow.yml")
	if _, err := os.Stat(outputFile); err != nil {
		t.Fatal("output file not created:", err)
	}

	if _, err := os.Stat(filepath.Join(circleDir, "config.yml.bak")); err != nil {
		t.Error("backup file should exist:", err)
	}
}

func TestReplaceSourceEnvVars_CircleCI(t *testing.T) {
	result := &Result{
		Source: SourceCircleCI,
		Workflow: &workflow.Workflow{
			Steps: []workflow.Step{
				{
					Name: "deploy",
					Commands: []string{
						"docker build -t myapp:$CIRCLE_SHA1 .",
						"docker push myapp:${CIRCLE_SHA1}",
						"./bin/kamal deploy --version=$CIRCLE_SHA1",
						"echo branch is $CIRCLE_BRANCH",
						"echo build $CIRCLE_BUILD_NUM for $CIRCLE_PROJECT_REPONAME",
						"echo no vars here",
					},
				},
			},
		},
	}

	replaceSourceEnvVars(result)

	expected := []string{
		"docker build -t myapp:$FEATHERCI_COMMIT_SHA .",
		"docker push myapp:${FEATHERCI_COMMIT_SHA}",
		"./bin/kamal deploy --version=$FEATHERCI_COMMIT_SHA",
		"echo branch is $FEATHERCI_BRANCH",
		"echo build $FEATHERCI_BUILD_NUMBER for $FEATHERCI_PROJECT_NAME",
		"echo no vars here",
	}

	for i, exp := range expected {
		if result.Workflow.Steps[0].Commands[i] != exp {
			t.Errorf("command[%d]:\n  got:  %s\n  want: %s", i, result.Workflow.Steps[0].Commands[i], exp)
		}
	}
}

func TestReplaceSourceEnvVars_GitHub(t *testing.T) {
	result := &Result{
		Source: SourceGitHub,
		Workflow: &workflow.Workflow{
			Steps: []workflow.Step{
				{
					Name: "build",
					Commands: []string{
						"echo $GITHUB_SHA",
						"echo ${GITHUB_REF_NAME}",
						"echo $GITHUB_RUN_NUMBER",
					},
				},
			},
		},
	}

	replaceSourceEnvVars(result)

	expected := []string{
		"echo $FEATHERCI_COMMIT_SHA",
		"echo ${FEATHERCI_BRANCH}",
		"echo $FEATHERCI_BUILD_NUMBER",
	}

	for i, exp := range expected {
		if result.Workflow.Steps[0].Commands[i] != exp {
			t.Errorf("command[%d]:\n  got:  %s\n  want: %s", i, result.Workflow.Steps[0].Commands[i], exp)
		}
	}
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"build", "build"},
		{"Build and Test", "build-and-test"},
		{"deploy_staging", "deploy_staging"},
		{"my-job-123", "my-job-123"},
		{"Job With Symbols!@#", "job-with-symbols"},
		{"", "step"},
	}

	for _, tt := range tests {
		got := sanitizeName(tt.input)
		if got != tt.expected {
			t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestReduceTransitiveDeps(t *testing.T) {
	// A → (no deps)
	// B → A
	// C → A, B  (A is redundant since B→A)
	// D → A, C  (A is redundant since C→B→A)
	wf := &workflow.Workflow{
		Steps: []workflow.Step{
			{Name: "A"},
			{Name: "B", DependsOn: []string{"A"}},
			{Name: "C", DependsOn: []string{"A", "B"}},
			{Name: "D", DependsOn: []string{"A", "C"}},
		},
	}

	reduceTransitiveDeps(wf)

	// B should keep A (only dep)
	if len(wf.Steps[1].DependsOn) != 1 || wf.Steps[1].DependsOn[0] != "A" {
		t.Errorf("B deps: got %v, want [A]", wf.Steps[1].DependsOn)
	}
	// C should only keep B (A is transitive via B)
	if len(wf.Steps[2].DependsOn) != 1 || wf.Steps[2].DependsOn[0] != "B" {
		t.Errorf("C deps: got %v, want [B]", wf.Steps[2].DependsOn)
	}
	// D should only keep C (A is transitive via C→B→A)
	if len(wf.Steps[3].DependsOn) != 1 || wf.Steps[3].DependsOn[0] != "C" {
		t.Errorf("D deps: got %v, want [C]", wf.Steps[3].DependsOn)
	}
}

func TestReduceTransitiveDeps_Diamond(t *testing.T) {
	// Diamond: A → (no deps), B → A, C → A, D → B, C
	// D's deps B and C are independent (neither is reachable from the other)
	// so both should be kept
	wf := &workflow.Workflow{
		Steps: []workflow.Step{
			{Name: "A"},
			{Name: "B", DependsOn: []string{"A"}},
			{Name: "C", DependsOn: []string{"A"}},
			{Name: "D", DependsOn: []string{"A", "B", "C"}},
		},
	}

	reduceTransitiveDeps(wf)

	// D should keep B and C but drop A (reachable via both B and C)
	if len(wf.Steps[3].DependsOn) != 2 {
		t.Errorf("D deps: got %v, want [B, C]", wf.Steps[3].DependsOn)
	}
}
