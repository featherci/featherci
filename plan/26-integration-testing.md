# Step 26: Integration Testing

---
model: opus
depends_on: [25]
estimated_time: 2 hours
---

## Overview

Create a test application within the FeatherCI repository to perform end-to-end integration testing. This test app will have a comprehensive workflow that exercises all FeatherCI features including parallel steps, dependencies, manual approval gates, secrets, and caching.

## IMPORTANT: User Action Required

**Before starting this step, you (the user) must:**

1. Create a new GitHub repository for the test application (e.g., `featherci-test-app`)
2. Push the test application code to that repository
3. Add the repository to FeatherCI via the UI
4. Configure the webhook in GitHub repository settings
5. Optionally: Add test secrets via the FeatherCI UI

The assistant will create the test application code locally, but you must handle the GitHub setup.

## Test Application Structure

```
testapp/                          # Added to .gitignore
├── README.md                     # Instructions for setup
├── .featherci/
│   └── workflow.yml              # Comprehensive test workflow
├── src/
│   └── main.go                   # Simple Go application
├── tests/
│   └── main_test.go              # Unit tests
└── Dockerfile                    # For deployment stage testing
```

## Test Workflow Features

The workflow should exercise ALL FeatherCI features:

### 1. Parallel Jobs
```yaml
jobs:
  lint:
    # Runs in parallel with test
  test:
    # Runs in parallel with lint
```

### 2. Dependencies
```yaml
jobs:
  build:
    needs: [lint, test]           # Waits for both to complete
```

### 3. Matrix Builds
```yaml
jobs:
  test:
    strategy:
      matrix:
        go-version: ['1.21', '1.22']
```

### 4. Manual Approval
```yaml
jobs:
  deploy-staging:
    needs: [build]
    environment:
      name: staging
      approval: required          # Requires manual approval
```

### 5. Secrets Usage
```yaml
jobs:
  deploy:
    steps:
      - run: echo "Deploying with ${{ secrets.DEPLOY_TOKEN }}"
```

### 6. Caching
```yaml
jobs:
  build:
    steps:
      - uses: cache
        with:
          path: ~/go/pkg/mod
          key: go-mod-${{ hashFiles('go.sum') }}
```

### 7. Artifacts
```yaml
jobs:
  build:
    steps:
      - uses: upload-artifact
        with:
          name: binary
          path: ./bin/app
  
  deploy:
    steps:
      - uses: download-artifact
        with:
          name: binary
```

### 8. Conditional Execution
```yaml
jobs:
  deploy-prod:
    if: github.ref == 'refs/heads/main'
```

### 9. Services (Database for tests)
```yaml
jobs:
  integration-test:
    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_PASSWORD: test
```

### 10. Timeouts and Failure Handling
```yaml
jobs:
  long-test:
    timeout-minutes: 30
    continue-on-error: true
```

## Complete Test Workflow

```yaml
name: CI/CD Pipeline

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main]

env:
  GO_VERSION: '1.22'
  APP_NAME: testapp

jobs:
  # Stage 1: Quality checks (parallel)
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: checkout
      - name: Setup Go
        uses: setup-go
        with:
          go-version: ${{ env.GO_VERSION }}
      - name: Run linter
        run: |
          go install golang.org/x/lint/golint@latest
          golint ./...

  test:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go-version: ['1.21', '1.22']
    services:
      redis:
        image: redis:7
        ports:
          - 6379:6379
    steps:
      - uses: checkout
      - name: Setup Go
        uses: setup-go
        with:
          go-version: ${{ matrix.go-version }}
      - name: Cache Go modules
        uses: cache
        with:
          path: ~/go/pkg/mod
          key: go-mod-${{ hashFiles('go.sum') }}
      - name: Run tests
        run: go test -v -race -coverprofile=coverage.out ./...
      - name: Upload coverage
        uses: upload-artifact
        with:
          name: coverage-${{ matrix.go-version }}
          path: coverage.out

  # Stage 2: Build (depends on quality checks)
  build:
    needs: [lint, test]
    runs-on: ubuntu-latest
    steps:
      - uses: checkout
      - name: Setup Go
        uses: setup-go
        with:
          go-version: ${{ env.GO_VERSION }}
      - name: Cache Go modules
        uses: cache
        with:
          path: ~/go/pkg/mod
          key: go-mod-${{ hashFiles('go.sum') }}
      - name: Build binary
        run: |
          CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/app ./src
      - name: Upload binary
        uses: upload-artifact
        with:
          name: app-binary
          path: bin/app

  # Stage 3: Deploy to staging (manual approval)
  deploy-staging:
    needs: [build]
    runs-on: ubuntu-latest
    environment:
      name: staging
      approval: required
    steps:
      - name: Download binary
        uses: download-artifact
        with:
          name: app-binary
      - name: Deploy to staging
        run: |
          echo "Deploying to staging..."
          echo "Using secret: ${{ secrets.STAGING_TOKEN }}"
          sleep 5
          echo "Staging deployment complete!"

  # Stage 4: Integration tests on staging
  integration-test:
    needs: [deploy-staging]
    runs-on: ubuntu-latest
    timeout-minutes: 10
    steps:
      - uses: checkout
      - name: Run integration tests
        run: |
          echo "Running integration tests against staging..."
          sleep 3
          echo "Integration tests passed!"

  # Stage 5: Deploy to production (main branch only, manual approval)
  deploy-prod:
    needs: [integration-test]
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    environment:
      name: production
      approval: required
    steps:
      - name: Download binary
        uses: download-artifact
        with:
          name: app-binary
      - name: Deploy to production
        run: |
          echo "Deploying to production..."
          echo "Using secret: ${{ secrets.PROD_TOKEN }}"
          sleep 5
          echo "Production deployment complete!"

  # Cleanup job (always runs)
  cleanup:
    needs: [deploy-staging]
    if: always()
    runs-on: ubuntu-latest
    continue-on-error: true
    steps:
      - name: Cleanup resources
        run: |
          echo "Cleaning up temporary resources..."
          echo "Cleanup complete!"
```

## Test Scenarios

### Scenario 1: Happy Path (main branch push)
1. Push to main branch
2. Lint and test run in parallel
3. Build runs after both pass
4. Manual approval required for staging
5. User approves staging deployment
6. Integration tests run
7. Manual approval required for production
8. User approves production deployment
9. Pipeline graph shows all green

### Scenario 2: PR Build (no deployment)
1. Open PR to main
2. Lint and test run in parallel
3. Build runs after both pass
4. Deployment stages skipped (conditional)

### Scenario 3: Test Failure
1. Introduce a failing test
2. Push to branch
3. Test job fails
4. Build is skipped (dependency failed)
5. Pipeline graph shows failure state

### Scenario 4: Manual Approval Timeout
1. Push to main
2. Reach staging approval
3. Do NOT approve
4. Verify timeout behavior

### Scenario 5: Cancelled Build
1. Start a build
2. Cancel mid-execution
3. Verify cleanup and state

## Implementation Tasks

1. **Create testapp directory structure**
   - Add to .gitignore
   - Create README with setup instructions

2. **Create simple Go application**
   - main.go with HTTP server
   - Unit tests
   - Dockerfile

3. **Create comprehensive workflow.yml**
   - All features demonstrated
   - Realistic CI/CD pipeline

4. **Create setup instructions**
   - GitHub repo creation
   - Webhook configuration
   - Secrets setup
   - Running test scenarios

## Files to Create

| File | Purpose |
|------|---------|
| `testapp/README.md` | Setup and testing instructions |
| `testapp/.featherci/workflow.yml` | Comprehensive test workflow |
| `testapp/src/main.go` | Simple Go HTTP server |
| `testapp/tests/main_test.go` | Unit tests |
| `testapp/go.mod` | Go module file |
| `testapp/Dockerfile` | Container build |
| `.gitignore` | Add testapp/ entry |

## Verification Checklist

After setup, verify each feature works:

- [ ] Parallel job execution (lint + test run simultaneously)
- [ ] Job dependencies (build waits for lint + test)
- [ ] Matrix builds (test runs with Go 1.21 and 1.22)
- [ ] Manual approval gates (staging and prod deployments pause)
- [ ] Secrets injection (STAGING_TOKEN, PROD_TOKEN available)
- [ ] Caching (Go modules cached between runs)
- [ ] Artifacts (binary uploaded and downloaded)
- [ ] Conditional execution (prod deploy only on main)
- [ ] Services (Redis available during tests)
- [ ] Timeout handling (integration test has 10min limit)
- [ ] Continue on error (cleanup runs even if previous fails)
- [ ] Pipeline graph (DAG renders correctly)
- [ ] Log streaming (real-time output visible)
- [ ] Build cancellation (can cancel mid-run)
- [ ] Status posting (GitHub shows pending/success/failure)

## Notes

- The testapp is intentionally in the FeatherCI repo for convenience during development
- It's added to .gitignore so it doesn't pollute the main repo
- User must create a separate GitHub repo and push the testapp there
- This allows testing webhooks, status posting, and the full integration
