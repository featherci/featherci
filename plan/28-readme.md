---
model: sonnet
---

# Step 27: README and Repository Documentation

## Objective
Create a comprehensive README.md and supporting documentation for the repository.

## Tasks

### 27.1 Create Main README.md
`README.md`:
```markdown
# FeatherCI

A lightweight, self-hosted CI/CD system written in Go. Fast, simple, and easy to deploy.

![Build Status](https://img.shields.io/github/actions/workflow/status/featherci/featherci/ci.yml?branch=main)
![License](https://img.shields.io/github/license/featherci/featherci)
![Go Version](https://img.shields.io/github/go-mod/go-version/featherci/featherci)

<!-- SCREENSHOT: Dashboard showing recent builds across projects -->
<!-- Filename: docs/images/dashboard.png -->
<!-- Description: Screenshot of the main dashboard with 3-4 projects showing recent build statuses (mix of success, failure, running) -->

## Features

- **Single Binary** - Everything bundled into one executable, no dependencies
- **Multi-Platform Auth** - Sign in with GitHub, GitLab, or Gitea/Forgejo
- **Visual Pipelines** - DAG visualization of your build steps
- **Manual Approvals** - Gate deployments with approval steps
- **Encrypted Secrets** - AES-256 encrypted secrets at rest
- **Build Caching** - Speed up builds with dependency caching
- **Real-time Logs** - Stream build logs as they happen
- **Distributed Workers** - Scale with multiple worker nodes

<!-- SCREENSHOT: Pipeline graph visualization -->
<!-- Filename: docs/images/pipeline-graph.png -->
<!-- Description: Screenshot of a build page showing the SVG pipeline graph with multiple steps, some succeeded (green), some failed (red), showing dependencies between steps -->

## Quick Start

### Using Docker

```bash
docker run -d \
  -p 8080:8080 \
  -v featherci-data:/data \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e FEATHERCI_SECRET_KEY=$(openssl rand -base64 32) \
  -e FEATHERCI_BASE_URL=http://localhost:8080 \
  -e FEATHERCI_ADMINS=yourgithubusername \
  -e FEATHERCI_GITHUB_CLIENT_ID=your_client_id \
  -e FEATHERCI_GITHUB_CLIENT_SECRET=your_client_secret \
  featherci/featherci:latest
```

### Using Homebrew (macOS)

```bash
brew install featherci/tap/featherci
brew services start featherci
```

### Manual Installation

```bash
curl -fsSL https://raw.githubusercontent.com/featherci/featherci/main/scripts/install.sh | bash
```

## Configuration

FeatherCI is configured via environment variables:

| Variable | Required | Description |
|----------|----------|-------------|
| `FEATHERCI_SECRET_KEY` | Yes | 32-byte base64-encoded encryption key |
| `FEATHERCI_BASE_URL` | Yes | Public URL for OAuth callbacks |
| `FEATHERCI_ADMINS` | Yes | Comma-separated admin usernames |
| `FEATHERCI_DATABASE_PATH` | No | SQLite database path (default: `./featherci.db`) |
| `FEATHERCI_BIND_ADDR` | No | Server bind address (default: `:8080`) |

### OAuth Provider Configuration

**GitHub:**
```bash
FEATHERCI_GITHUB_CLIENT_ID=your_client_id
FEATHERCI_GITHUB_CLIENT_SECRET=your_client_secret
```

**GitLab:**
```bash
FEATHERCI_GITLAB_URL=https://gitlab.com  # or self-hosted URL
FEATHERCI_GITLAB_CLIENT_ID=your_client_id
FEATHERCI_GITLAB_CLIENT_SECRET=your_client_secret
```

**Gitea/Forgejo:**
```bash
FEATHERCI_GITEA_URL=https://your-gitea-instance.com
FEATHERCI_GITEA_CLIENT_ID=your_client_id
FEATHERCI_GITEA_CLIENT_SECRET=your_client_secret
```

See the [full configuration guide](https://featherci.github.io/configuration) for all options.

## Workflow Configuration

Create `.featherci/workflow.yml` in your repository:

```yaml
name: CI Pipeline

on:
  push:
    branches: [main, develop]
  pull_request:

steps:
  - name: test
    image: golang:1.22
    commands:
      - go test -v ./...
    cache:
      key: go-{{ checksum "go.sum" }}
      paths:
        - /go/pkg/mod

  - name: build
    image: golang:1.22
    depends_on: [test]
    commands:
      - go build -o app ./cmd/app

  - name: deploy-approval
    type: approval
    depends_on: [build]

  - name: deploy
    image: alpine:latest
    depends_on: [deploy-approval]
    commands:
      - ./deploy.sh
```

<!-- SCREENSHOT: Workflow file in editor alongside build running -->
<!-- Filename: docs/images/workflow-example.png -->
<!-- Description: Split view or side-by-side showing the workflow YAML and the resulting build with steps -->

## Secrets Management

Secrets are encrypted at rest and injected as environment variables:

<!-- SCREENSHOT: Secrets management page -->
<!-- Filename: docs/images/secrets.png -->
<!-- Description: Screenshot of the secrets page showing a list of secrets (names only, not values) with add/delete buttons -->

1. Navigate to your project settings
2. Click "Secrets"
3. Add secrets with `NAME=value` format
4. Reference in your workflow via `$NAME`

## Distributed Workers

Run additional workers to scale build capacity:

```bash
featherci --mode=worker \
  --master-url=https://ci.example.com \
  --worker-secret=shared-secret
```

Or with Docker:

```bash
docker run -d \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e FEATHERCI_MODE=worker \
  -e FEATHERCI_MASTER_URL=https://ci.example.com \
  -e FEATHERCI_WORKER_SECRET=shared-secret \
  featherci/featherci:latest
```

## Development

```bash
# Clone repository
git clone https://github.com/featherci/featherci.git
cd featherci

# Install dependencies
make deps

# Run in development mode
make dev

# Run tests
make test

# Build binary
make build
```

## Contributing

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) first.

## License

MIT License - see [LICENSE](LICENSE) for details.
```

### 27.2 Create CONTRIBUTING.md
```markdown
# Contributing to FeatherCI

Thank you for your interest in contributing!

## Development Setup

1. Install Go 1.22+
2. Install Docker
3. Clone the repository
4. Run `make deps` to install dependencies
5. Run `make dev` to start development server

## Code Style

- Run `gofmt` on all Go files
- Follow [Effective Go](https://go.dev/doc/effective_go)
- Add tests for new features

## Pull Request Process

1. Create a feature branch from `main`
2. Make your changes
3. Run `make check` to verify tests and linting
4. Submit a PR with a clear description

## Reporting Issues

Please include:
- FeatherCI version
- Operating system
- Steps to reproduce
- Expected vs actual behavior
```

### 27.3 Create LICENSE
MIT License file.

### 27.4 Create CHANGELOG.md
```markdown
# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added
- Initial release
- GitHub, GitLab, and Gitea/Forgejo OAuth support
- Workflow-based pipeline configuration
- Manual approval gates
- Encrypted secrets management
- Build caching
- Real-time log streaming
- Pipeline DAG visualization
- Distributed worker support
- Docker and systemd deployment options
```

### 27.5 Screenshot Placeholders Summary

| Filename | Location | Description |
|----------|----------|-------------|
| `dashboard.png` | `docs/images/` | Main dashboard showing projects with various build statuses |
| `pipeline-graph.png` | `docs/images/` | Build page with SVG pipeline visualization |
| `workflow-example.png` | `docs/images/` | Workflow YAML alongside running build |
| `secrets.png` | `docs/images/` | Secrets management page |
| `login.png` | `docs/images/` | Login page with OAuth provider buttons |
| `build-logs.png` | `docs/images/` | Build detail page showing streaming logs |
| `project-list.png` | `docs/images/` | Project listing page |
| `new-project.png` | `docs/images/` | New project creation showing repo selection |

## Deliverables
- [ ] `README.md` - Comprehensive project README
- [ ] `CONTRIBUTING.md` - Contribution guidelines
- [ ] `LICENSE` - MIT license file
- [ ] `CHANGELOG.md` - Version changelog
- [ ] `docs/images/` directory for screenshots
- [ ] Screenshot placeholder documentation

## Dependencies
- All previous steps (for accurate documentation)

## Estimated Effort
Small - Documentation writing
