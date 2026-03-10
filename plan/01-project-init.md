---
model: sonnet
---

# Step 01: Project Initialization

## Objective
Set up the Go project structure with module initialization, basic directory layout, and essential tooling configuration.

## Tasks

### 1.1 Initialize Go Module
```bash
go mod init github.com/featherci/featherci
```

### 1.2 Create Directory Structure
```
cmd/featherci/main.go       # Entry point
internal/config/config.go   # Configuration placeholder
internal/version/version.go # Version info
web/templates/.gitkeep
web/static/.gitkeep
web/tailwind/.gitkeep
scripts/systemd/.gitkeep
scripts/homebrew/.gitkeep
scripts/docker/.gitkeep
docs/.gitkeep
```

### 1.3 Create Main Entry Point
Basic `main.go` that:
- Parses command-line flags (--version, --help)
- Prints version information
- Sets up signal handling for graceful shutdown

### 1.4 Create Makefile
Targets for:
- `build`: Compile the binary
- `dev`: Run in development mode
- `test`: Run tests
- `lint`: Run golangci-lint
- `fmt`: Run gofmt
- `clean`: Clean build artifacts
- `css`: Compile Tailwind CSS

### 1.5 Create .gitignore
Ignore:
- Binary outputs
- IDE files
- `.env` files (but NOT `.env.example`)
- `*.db` files
- `node_modules/` (for Tailwind)
- Build cache

### 1.6 Create .env.example
Create a template `.env.example` file with all configuration options (commented with descriptions). This file IS committed to git as a reference.

```bash
# FeatherCI Configuration
# Copy to .env and fill in your values: cp .env.example .env

# Required: 32-byte base64 encryption key
# Generate with: openssl rand -base64 32
FEATHERCI_SECRET_KEY=

# Required: Public URL (for OAuth callbacks)
FEATHERCI_BASE_URL=http://localhost:8080

# Required: Admin usernames (comma-separated)
FEATHERCI_ADMINS=yourusername

# Server bind address
FEATHERCI_BIND_ADDR=:8080

# Database path
FEATHERCI_DATABASE_PATH=./featherci.db

# Cache path
FEATHERCI_CACHE_PATH=./cache

# GitHub OAuth (create at https://github.com/settings/developers)
# Set callback URL to: {FEATHERCI_BASE_URL}/auth/github/callback
FEATHERCI_GITHUB_CLIENT_ID=
FEATHERCI_GITHUB_CLIENT_SECRET=

# GitLab OAuth (optional)
# FEATHERCI_GITLAB_URL=https://gitlab.com
# FEATHERCI_GITLAB_CLIENT_ID=
# FEATHERCI_GITLAB_CLIENT_SECRET=

# Gitea/Forgejo OAuth (optional)
# FEATHERCI_GITEA_URL=https://your-gitea.com
# FEATHERCI_GITEA_CLIENT_ID=
# FEATHERCI_GITEA_CLIENT_SECRET=

# Worker configuration (optional)
# FEATHERCI_MODE=standalone
# FEATHERCI_WORKER_SECRET=
# FEATHERCI_MASTER_URL=
```

### 1.7 Add golangci-lint Configuration
Create `.golangci.yml` with sensible defaults for:
- gofmt
- govet
- errcheck
- staticcheck
- unused

## Deliverables
- [ ] `go.mod` initialized
- [ ] Directory structure created
- [ ] `cmd/featherci/main.go` with basic CLI
- [ ] `Makefile` with essential targets
- [ ] `.gitignore` configured
- [ ] `.env.example` with all config options documented
- [ ] `.golangci.yml` configured
- [ ] Code compiles and runs with `go run ./cmd/featherci`

## Dependencies
None (first step)

## Estimated Effort
Small - Foundation setup
