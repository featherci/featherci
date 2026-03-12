# FeatherCI

A lightweight, self-hosted CI/CD system written in Go. Fast, simple, and easy to deploy.

<!-- SCREENSHOT: docs/images/dashboard.png - Main dashboard showing projects with various build statuses (success, failure, running) -->

## Features

- **Single Binary** - Everything bundled into one executable, no external dependencies
- **Multi-Platform Auth** - Sign in with GitHub, GitLab, or Gitea/Forgejo
- **Visual Pipelines** - DAG visualization of your build steps
- **Manual Approvals** - Gate deployments with approval steps
- **Conditional Steps** - Run steps based on branch, event, or custom expressions
- **Encrypted Secrets** - AES-256 encrypted secrets at rest
- **Build Caching** - Speed up builds with dependency caching
- **Real-time Logs** - Stream build logs as they happen
- **Notifications** - Email (SMTP/SendGrid/Mailgun), Slack, Discord, and Pushover
- **Distributed Workers** - Scale with multiple worker nodes
- **Docker Execution** - Steps run in isolated Docker containers

<!-- SCREENSHOT: docs/images/pipeline-graph.png - Build page showing the SVG pipeline graph with multiple steps and dependencies, mix of succeeded/failed/running -->

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
  ghcr.io/featherci/featherci:latest
```

The Docker socket mount (`/var/run/docker.sock`) is required so FeatherCI can run build steps in containers. See the [Docker deployment](#docker-deployment) section for more details.

### Homebrew (macOS / Linux)

```bash
brew install featherci/tap/featherci
```

### Install Script (Linux)

```bash
curl -sSL https://featherci.dev/install.sh | bash
```

### From Source

```bash
git clone https://github.com/featherci/featherci.git
cd featherci
make build
./bin/featherci
```

### Development Mode

For local development without OAuth configuration:

```bash
make dev
```

This starts FeatherCI with `--dev` flag, which skips OAuth and auto-logs in as an admin user.

## Configuration

FeatherCI can be configured with a **YAML config file**, environment variables, or a `.env` file. Precedence: env vars > `.env` > YAML config > defaults.

```bash
# Copy the example config
cp scripts/config.yaml.example config.yaml
# Or for system-wide installs
sudo cp scripts/config.yaml.example /etc/featherci/config.yaml
```

FeatherCI automatically checks `/etc/featherci/config.yaml` and `./config.yaml`. Use `--config` to specify a custom path.

### Required Settings

| Variable | Description |
|----------|-------------|
| `FEATHERCI_SECRET_KEY` | 32-byte base64-encoded encryption key (generate with `featherci --generate-key`) |
| `FEATHERCI_BASE_URL` | Public URL for OAuth callbacks (e.g. `https://ci.example.com`) |
| `FEATHERCI_ADMINS` | Comma-separated admin usernames |

### Optional Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `FEATHERCI_BIND_ADDR` | `:8080` | Server bind address |
| `FEATHERCI_DATABASE_PATH` | `./featherci.db` | SQLite database path |
| `FEATHERCI_CACHE_PATH` | `./cache` | Build cache directory |
| `FEATHERCI_MODE` | `standalone` | Operating mode: `standalone`, `master`, or `worker` |

### OAuth Providers

Configure at least one OAuth provider. Create an OAuth application in your provider's settings with the callback URL set to `{FEATHERCI_BASE_URL}/auth/{provider}/callback`.

**GitHub:**
```bash
FEATHERCI_GITHUB_CLIENT_ID=your_client_id
FEATHERCI_GITHUB_CLIENT_SECRET=your_client_secret
```

**GitLab:**
```bash
FEATHERCI_GITLAB_URL=https://gitlab.com       # or self-hosted URL
FEATHERCI_GITLAB_CLIENT_ID=your_client_id
FEATHERCI_GITLAB_CLIENT_SECRET=your_client_secret
```

**Gitea/Forgejo:**
```bash
FEATHERCI_GITEA_URL=https://your-gitea-instance.com
FEATHERCI_GITEA_CLIENT_ID=your_client_id
FEATHERCI_GITEA_CLIENT_SECRET=your_client_secret
```

## Migrating from GitHub Actions or CircleCI

Already using another CI system? FeatherCI can automatically convert your existing configuration:

```bash
cd your-project
featherci convert
```

This will:
- Auto-detect `.github/workflows/*.yml` or `.circleci/config.yml`
- Generate `.featherci/workflow.yml` with equivalent steps
- Rename the original file with `.bak` so you can review and delete it
- Print clear warnings for any features that need manual adjustment

**Supported conversions:**

| Feature | GitHub Actions | CircleCI |
|---------|---------------|----------|
| Jobs/steps | Jobs → steps | Jobs → steps |
| Docker images | `container` | `docker` |
| Dependencies | `needs` | `requires` |
| Caching | `actions/cache` | `save_cache`/`restore_cache` |
| Secrets | `${{ secrets.X }}` | — |
| Conditions | `github.ref` expressions | Workflow filters → `if` |
| Approvals | — | `type: approval` |
| Service containers | — | `docker[1:]` → `services` |
| Custom commands | — | Inlined with shell equivalents |
| Common orbs | — | Expanded to shell commands |

Features like GitHub Actions (`uses:`), build matrices, and advanced orbs will generate warnings with guidance on alternatives.

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

  - name: lint
    image: golang:1.22
    commands:
      - golangci-lint run
    cache:
      key: go-{{ checksum "go.sum" }}
      paths:
        - /go/pkg/mod

  - name: build
    image: golang:1.22
    depends_on: [test, lint]
    commands:
      - go build -o app ./cmd/app

  - name: deploy-approval
    type: approval
    depends_on: [build]

  - name: integration
    image: ruby:3.4
    depends_on: [test]
    services:
      - image: mysql:8.0
        env:
          MYSQL_ROOT_PASSWORD: test
      - image: redis:7
    commands:
      - bundle exec rspec spec/integration

  - name: deploy-approval
    type: approval
    depends_on: [build, integration]

  - name: build-docker
    image: docker:27
    depends_on: [test]
    docker: true
    commands:
      - docker build -t myapp .
      - docker push myapp

  - name: deploy
    image: alpine:latest
    depends_on: [deploy-approval]
    condition: branch == "main"
    commands:
      - ./deploy.sh
    env:
      DEPLOY_TOKEN: $DEPLOY_TOKEN
```

Service containers run on a shared Docker network and are accessible from the step container by hostname (derived from the image name, e.g., `mysql:8.0` → hostname `mysql`).

<!-- SCREENSHOT: docs/images/workflow-example.png - Split view showing the workflow YAML alongside the resulting build with steps -->

### Workflow Reference

**Triggers** (`on`):
- `push.branches` - Run on pushes to specific branches
- `pull_request` - Run on pull request events

**Step fields:**
| Field | Description |
|-------|-------------|
| `name` | Step name (required) |
| `image` | Docker image to run the step in |
| `commands` | Shell commands to execute |
| `depends_on` | List of step names this step depends on |
| `type` | Set to `approval` for manual approval gates |
| `condition` | Expression to conditionally run the step (e.g. `branch == "main"`) |
| `timeout` | Step timeout (e.g. `10m`, `1h`) |
| `env` | Environment variables (can reference secrets with `$NAME`) |
| `cache.key` | Cache key template (supports `{{ checksum "file" }}`) |
| `cache.paths` | Directories to cache between builds |
| `services` | Sidecar containers (e.g., databases) accessible by hostname |
| `docker` | Mount Docker socket into the container (`true`/`false`) |

## Secrets Management

<!-- SCREENSHOT: docs/images/secrets.png - Secrets management page showing a list of secrets (names only, not values) with add/delete buttons -->

Secrets are encrypted at rest with AES-256-GCM and injected as environment variables during builds.

1. Navigate to your project's **Settings** page
2. Add secrets as key-value pairs
3. Reference them in your workflow with `$SECRET_NAME` in the `env` block

Secrets are never exposed in logs or the UI after creation.

## Notifications

<!-- SCREENSHOT: docs/images/notifications.png - Notification channels list page showing configured channels with test/delete buttons -->

Configure notification channels per-project via the **Notifications** page in project settings. Supported channels:

- **Email** - SMTP, SendGrid, or Mailgun
- **Slack** - Via incoming webhooks
- **Discord** - Via webhooks
- **Pushover** - Push notifications

Each channel can be configured to notify on success, failure, and/or cancellation.

## Distributed Workers

FeatherCI supports scaling build capacity with additional worker nodes. All instances share the same SQLite database (or connect to a master).

**Master instance:**
```bash
FEATHERCI_MODE=master
FEATHERCI_WORKER_SECRET=your-shared-secret
```

**Worker instance:**
```bash
FEATHERCI_MODE=worker
FEATHERCI_MASTER_URL=https://ci.example.com
FEATHERCI_WORKER_SECRET=your-shared-secret
```

Or with Docker:

```bash
docker run -d \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e FEATHERCI_MODE=worker \
  -e FEATHERCI_MASTER_URL=https://ci.example.com \
  -e FEATHERCI_WORKER_SECRET=your-shared-secret \
  ghcr.io/featherci/featherci:latest
```

## Docker Deployment

FeatherCI runs build steps inside Docker containers using the host's Docker daemon. When running FeatherCI itself in Docker, mount the Docker socket:

```bash
docker run -v /var/run/docker.sock:/var/run/docker.sock featherci/featherci
```

This uses the "sibling container" pattern — build containers run alongside FeatherCI rather than inside it. Ensure the FeatherCI process has permission to access the Docker socket (typically via the `docker` group or running as root).

You can also set `DOCKER_HOST` to point to a remote Docker daemon if needed.

<!-- SCREENSHOT: docs/images/build-logs.png - Build detail page showing streaming log output in the terminal-style viewer -->

## Development

### Prerequisites

- Go 1.22+
- Docker (for running builds)
- Make

### Commands

```bash
make dev          # Run in development mode (with hot CSS reloading)
make build        # Compile the binary
make test         # Run tests
make lint         # Run golangci-lint
make fmt          # Format Go code
make css          # Compile Tailwind CSS
make css-watch    # Watch and recompile CSS on changes
make release      # Cross-compile release binaries
make docker-build # Build Docker image
make help         # Show all available targets
```

### Project Structure

```
cmd/featherci/       # Application entrypoint
internal/
  auth/              # OAuth provider implementations
  config/            # Configuration loading
  convert/           # CI migration tool (GitHub Actions, CircleCI)
  crypto/            # Encryption utilities
  database/          # SQLite migrations
  executor/          # Docker step execution
  git/               # Git clone/checkout operations
  handlers/          # HTTP request handlers
  models/            # Data models and repositories
  notify/            # Notification channels (email, Slack, etc.)
  server/            # HTTP server and routing
  services/          # Business logic layer
  status/            # Git provider commit status posting
  templates/         # HTML template engine
  worker/            # Build step polling and execution
  workflow/          # Workflow YAML parsing
web/
  static/            # CSS, JS assets
  tailwind/          # Tailwind CSS source
  templates/         # HTML templates (layouts, pages, components)
```

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT License - see [LICENSE](LICENSE) for details.
