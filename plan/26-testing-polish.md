---
model: opus
---

# Step 26: Testing and Polish

## Objective
Comprehensive testing, code cleanup, and final polish before release.

## Tasks

### 26.1 Unit Tests
Ensure all packages have adequate test coverage:

```go
// Target: >70% coverage for critical packages
// - internal/crypto
// - internal/workflow
// - internal/models
// - internal/auth
```

Run coverage:
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### 26.2 Integration Tests
Create integration tests for key flows:

```go
// tests/integration/auth_test.go
func TestOAuthFlow(t *testing.T) {
    // Test OAuth login flow with mock provider
}

// tests/integration/build_test.go
func TestBuildExecution(t *testing.T) {
    // Test full build lifecycle
}

// tests/integration/webhook_test.go
func TestWebhookProcessing(t *testing.T) {
    // Test webhook receipt and build trigger
}
```

### 26.3 End-to-End Tests
Create E2E tests using test containers:

```go
// tests/e2e/e2e_test.go
func TestFullPipeline(t *testing.T) {
    // 1. Start FeatherCI in test mode
    // 2. Create a project
    // 3. Trigger a build via webhook
    // 4. Verify build completes
    // 5. Check logs are available
}
```

### 26.4 Code Quality Checks
```makefile
.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: fmt
fmt:
	gofmt -s -w .

.PHONY: vet
vet:
	go vet ./...

.PHONY: check
check: fmt vet lint test
```

### 26.5 Security Audit
- Review all user input handling
- Verify SQL injection prevention (parameterized queries)
- Check XSS prevention in templates
- Audit secret handling
- Review OAuth state token handling

### 26.6 Performance Testing
```go
// tests/benchmark/benchmark_test.go
func BenchmarkWorkflowParsing(b *testing.B)
func BenchmarkGraphLayout(b *testing.B)
func BenchmarkEncryption(b *testing.B)
```

### 26.7 Error Handling Review
- Ensure all errors are properly wrapped with context
- Verify user-facing error messages are helpful
- Check that sensitive info isn't leaked in errors

### 26.8 Logging Review
- Consistent log levels (debug, info, warn, error)
- Structured logging with slog
- No sensitive data in logs
- Request IDs for tracing

### 26.9 Database Migrations Test
```go
func TestMigrationsUpDown(t *testing.T) {
    // Test all migrations can be applied and rolled back
}
```

### 26.10 UI Polish
- Responsive design check (mobile, tablet, desktop)
- Accessibility audit (ARIA labels, keyboard navigation)
- Loading states for async operations
- Error state displays
- Empty state displays

### 26.11 Create Example Workflow Files
`examples/workflows/basic.yml`:
```yaml
name: Basic CI

on:
  push:
  pull_request:

steps:
  - name: test
    image: golang:1.22
    commands:
      - go test ./...
```

`examples/workflows/full.yml`:
```yaml
name: Full Pipeline

on:
  push:
    branches: [main]

steps:
  - name: lint
    image: golangci/golangci-lint:latest
    commands:
      - golangci-lint run
    
  - name: test
    image: golang:1.22
    commands:
      - go test -v -race ./...
    cache:
      key: go-{{ checksum "go.sum" }}
      paths:
        - /go/pkg/mod
    
  - name: build
    image: golang:1.22
    depends_on: [lint, test]
    commands:
      - go build -o app ./cmd/app
    
  - name: approve-deploy
    type: approval
    depends_on: [build]
    
  - name: deploy
    image: alpine:latest
    depends_on: [approve-deploy]
    commands:
      - echo "Deploying..."
```

### 26.12 Final Checklist
- [ ] All tests pass
- [ ] No golangci-lint errors
- [ ] gofmt applied to all files
- [ ] All TODOs resolved or documented
- [ ] Version number set correctly
- [ ] License file present
- [ ] Docker build works
- [ ] systemd service works
- [ ] Homebrew formula works

## Deliverables
- [ ] >70% test coverage on critical packages
- [ ] Integration tests for key flows
- [ ] E2E test suite
- [ ] Clean golangci-lint run
- [ ] Example workflow files
- [ ] All checklist items complete

## Dependencies
- All previous steps

## Estimated Effort
Large - Comprehensive testing and polish
