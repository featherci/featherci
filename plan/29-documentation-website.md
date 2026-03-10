---
model: opus
---

# Step 28: GitHub Pages Documentation Website

## Objective
Create a modern, clean documentation website hosted on GitHub Pages.

## Tasks

### 28.1 Choose Static Site Generator
Use **Hugo** with a modern documentation theme for simplicity and speed.

### 28.2 Create Documentation Site Structure
```
docs/
├── .hugo_build.lock
├── config.toml
├── content/
│   ├── _index.md
│   ├── getting-started/
│   │   ├── _index.md
│   │   ├── installation.md
│   │   ├── configuration.md
│   │   └── first-project.md
│   ├── guides/
│   │   ├── _index.md
│   │   ├── workflow-syntax.md
│   │   ├── secrets.md
│   │   ├── caching.md
│   │   ├── notifications.md
│   │   ├── manual-approvals.md
│   │   └── distributed-workers.md
│   ├── reference/
│   │   ├── _index.md
│   │   ├── environment-variables.md
│   │   ├── workflow-reference.md
│   │   └── api.md
│   └── deployment/
│       ├── _index.md
│       ├── docker.md
│       ├── systemd.md
│       └── kubernetes.md
├── static/
│   └── images/
│       └── (screenshots go here)
├── layouts/
│   └── (custom layouts if needed)
└── themes/
    └── (theme submodule)
```

### 28.3 Create Hugo Configuration
`docs/config.toml`:
```toml
baseURL = "https://featherci.github.io/"
languageCode = "en-us"
title = "FeatherCI Documentation"
theme = "hugo-book"  # or "docsy", "doks"

[params]
  description = "Lightweight CI/CD System"
  github_repo = "https://github.com/featherci/featherci"
  github_branch = "main"
  
  # Logo
  logo = "/images/logo.svg"
  
  # Edit this page link
  editPage = true
  
[menu]
  [[menu.main]]
    name = "GitHub"
    url = "https://github.com/featherci/featherci"
    weight = 100

[markup]
  [markup.highlight]
    style = "github"
    lineNos = false
```

### 28.4 Create Landing Page
`docs/content/_index.md`:
```markdown
---
title: "FeatherCI"
---

# FeatherCI

A lightweight, self-hosted CI/CD system. Fast, simple, and easy to deploy.

<!-- SCREENSHOT: Hero image showing dashboard -->
<!-- Filename: docs/static/images/hero-dashboard.png -->
<!-- Description: Clean screenshot of the dashboard with a few projects, showing the modern UI -->

## Why FeatherCI?

- **Single Binary** - No dependencies, just download and run
- **Multi-Platform** - Integrate with GitHub, GitLab, or Gitea
- **Visual Pipelines** - See your build flow at a glance
- **Secure** - Encrypted secrets, OAuth authentication
- **Scalable** - Add workers as you grow

## Quick Start

Get up and running in 5 minutes:

\`\`\`bash
docker run -d -p 8080:8080 featherci/featherci:latest
\`\`\`

[Get Started →](/getting-started/)

## Features

{{< columns >}}
### Visual Pipelines
See your build steps and dependencies as an interactive graph.

<!-- SCREENSHOT: Pipeline graph close-up -->
<!-- Filename: docs/static/images/feature-pipeline.png -->
<!-- Description: Zoomed view of pipeline graph showing step dependencies -->

<--->

### Real-time Logs  
Stream build logs as they happen, with ANSI color support.

<!-- SCREENSHOT: Log streaming view -->
<!-- Filename: docs/static/images/feature-logs.png -->
<!-- Description: Build logs panel showing colored terminal output -->

{{< /columns >}}
```

### 28.5 Create Getting Started Section
`docs/content/getting-started/installation.md`:
```markdown
---
title: "Installation"
weight: 1
---

# Installation

Choose your preferred installation method.

## Docker (Recommended)

The easiest way to run FeatherCI:

\`\`\`bash
docker run -d \\
  --name featherci \\
  -p 8080:8080 \\
  -v featherci-data:/data \\
  -v /var/run/docker.sock:/var/run/docker.sock \\
  -e FEATHERCI_SECRET_KEY=$(openssl rand -base64 32) \\
  -e FEATHERCI_BASE_URL=http://localhost:8080 \\
  -e FEATHERCI_ADMINS=yourusername \\
  featherci/featherci:latest
\`\`\`

## Homebrew (macOS)

\`\`\`bash
brew tap featherci/tap
brew install featherci
brew services start featherci
\`\`\`

## Binary Download

Download the latest release for your platform:

| Platform | Architecture | Download |
|----------|--------------|----------|
| Linux | x86_64 | [featherci-linux-amd64](https://github.com/featherci/featherci/releases/latest) |
| Linux | ARM64 | [featherci-linux-arm64](https://github.com/featherci/featherci/releases/latest) |
| macOS | Intel | [featherci-darwin-amd64](https://github.com/featherci/featherci/releases/latest) |
| macOS | Apple Silicon | [featherci-darwin-arm64](https://github.com/featherci/featherci/releases/latest) |

## From Source

\`\`\`bash
git clone https://github.com/featherci/featherci.git
cd featherci
make build
./bin/featherci
\`\`\`

---

Next: [Configuration →](/getting-started/configuration/)
```

`docs/content/getting-started/configuration.md`:
```markdown
---
title: "Configuration"
weight: 2
---

# Configuration

FeatherCI is configured entirely through environment variables.

## Required Variables

| Variable | Description |
|----------|-------------|
| `FEATHERCI_SECRET_KEY` | 32-byte base64 encryption key. Generate with: `openssl rand -base64 32` |
| `FEATHERCI_BASE_URL` | Public URL where FeatherCI is accessible |
| `FEATHERCI_ADMINS` | Comma-separated list of admin usernames |

## OAuth Providers

Configure at least one OAuth provider:

### GitHub

1. Go to [GitHub Developer Settings](https://github.com/settings/developers)
2. Create a new OAuth App
3. Set callback URL to `{BASE_URL}/auth/github/callback`
4. Copy Client ID and Secret

\`\`\`bash
FEATHERCI_GITHUB_CLIENT_ID=your_client_id
FEATHERCI_GITHUB_CLIENT_SECRET=your_client_secret
\`\`\`

<!-- SCREENSHOT: GitHub OAuth app settings -->
<!-- Filename: docs/static/images/github-oauth-setup.png -->
<!-- Description: GitHub OAuth application creation page with callback URL highlighted -->

### GitLab

\`\`\`bash
FEATHERCI_GITLAB_URL=https://gitlab.com
FEATHERCI_GITLAB_CLIENT_ID=your_client_id  
FEATHERCI_GITLAB_CLIENT_SECRET=your_client_secret
\`\`\`

### Gitea / Forgejo

\`\`\`bash
FEATHERCI_GITEA_URL=https://your-instance.com
FEATHERCI_GITEA_CLIENT_ID=your_client_id
FEATHERCI_GITEA_CLIENT_SECRET=your_client_secret
\`\`\`

## Full Reference

See [Environment Variables Reference](/reference/environment-variables/) for all options.
```

### 28.6 Create Workflow Syntax Guide
`docs/content/guides/workflow-syntax.md`:
```markdown
---
title: "Workflow Syntax"
weight: 1
---

# Workflow Syntax

Define your CI pipeline in `.featherci/workflow.yml`.

## Basic Example

\`\`\`yaml
name: CI

on:
  push:
  pull_request:

steps:
  - name: test
    image: node:20
    commands:
      - npm ci
      - npm test
\`\`\`

## Triggers

### Push Events

\`\`\`yaml
on:
  push:
    branches: [main, develop]
    tags: [v*]
\`\`\`

### Pull Request Events

\`\`\`yaml
on:
  pull_request:
    branches: [main]  # Target branch
\`\`\`

## Steps

Each step runs in a Docker container:

\`\`\`yaml
steps:
  - name: build
    image: golang:1.22
    commands:
      - go build ./...
    env:
      CGO_ENABLED: "0"
    working_dir: /workspace/src
    timeout_minutes: 30
\`\`\`

## Dependencies

Create complex pipelines with step dependencies:

\`\`\`yaml
steps:
  - name: lint
    image: golangci/golangci-lint
    commands:
      - golangci-lint run

  - name: test
    image: golang:1.22
    commands:
      - go test ./...

  - name: build
    image: golang:1.22
    depends_on: [lint, test]  # Waits for both
    commands:
      - go build -o app
\`\`\`

<!-- SCREENSHOT: Pipeline with dependencies -->
<!-- Filename: docs/static/images/workflow-dependencies.png -->
<!-- Description: Pipeline graph showing lint and test running in parallel, then build after both complete -->

See [Workflow Reference](/reference/workflow-reference/) for complete syntax.
```

### 28.7 Create GitHub Actions Workflow for Docs
`.github/workflows/docs.yml`:
```yaml
name: Deploy Documentation

on:
  push:
    branches: [main]
    paths:
      - 'docs/**'
      - '.github/workflows/docs.yml'

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          submodules: true
          fetch-depth: 0

      - name: Setup Hugo
        uses: peaceiris/actions-hugo@v2
        with:
          hugo-version: 'latest'
          extended: true

      - name: Build
        run: hugo --minify
        working-directory: docs

      - name: Deploy
        uses: peaceiris/actions-gh-pages@v3
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
          publish_dir: ./docs/public
```

### 28.8 GitHub Pages Configuration

Create `docs/static/CNAME` if using custom domain:
```
docs.featherci.io
```

Repository Settings:
1. Go to Settings → Pages
2. Source: Deploy from a branch
3. Branch: `gh-pages` / `/ (root)`
4. Save

### 28.9 Screenshot Placeholders for Docs Site

| Filename | Location | Description |
|----------|----------|-------------|
| `hero-dashboard.png` | `docs/static/images/` | Main dashboard view, clean and professional, 1200x800px |
| `feature-pipeline.png` | `docs/static/images/` | Pipeline graph visualization, 600x400px |
| `feature-logs.png` | `docs/static/images/` | Build logs with ANSI colors, 600x400px |
| `github-oauth-setup.png` | `docs/static/images/` | GitHub OAuth app creation page |
| `workflow-dependencies.png` | `docs/static/images/` | Example pipeline showing parallel steps |
| `secrets-page.png` | `docs/static/images/` | Secrets management UI |
| `build-detail.png` | `docs/static/images/` | Single build page with all info |
| `project-settings.png` | `docs/static/images/` | Project settings page |
| `approval-step.png` | `docs/static/images/` | Step waiting for approval with button |
| `worker-status.png` | `docs/static/images/` | Admin view of connected workers |

### 28.10 Create Simple Logo
`docs/static/images/logo.svg`:
```svg
<svg viewBox="0 0 100 100" xmlns="http://www.w3.org/2000/svg">
  <circle cx="50" cy="50" r="45" fill="#0ea5e9"/>
  <path d="M30 50 L45 65 L70 35" stroke="white" stroke-width="8" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
</svg>
```

## Deliverables
- [ ] `docs/` directory with Hugo site structure
- [ ] `docs/config.toml` - Hugo configuration
- [ ] `docs/content/_index.md` - Landing page
- [ ] `docs/content/getting-started/*.md` - Getting started guides
- [ ] `docs/content/guides/*.md` - Feature guides
- [ ] `docs/content/reference/*.md` - Reference documentation
- [ ] `.github/workflows/docs.yml` - Auto-deploy workflow
- [ ] `docs/static/images/` - Directory for screenshots
- [ ] Screenshot placeholder documentation with filenames and descriptions

## GitHub Pages Setup Instructions

1. Push the `docs/` directory to the repository
2. Go to repository Settings → Pages
3. Under "Build and deployment":
   - Source: "GitHub Actions"
4. The docs workflow will automatically build and deploy on push to main
5. Site will be available at `https://featherci.github.io/`

For custom domain:
1. Add `CNAME` file to `docs/static/` with your domain
2. Configure DNS with your domain provider
3. Enable "Enforce HTTPS" in GitHub Pages settings

## Dependencies
- Step 27: README (for consistency)
- All previous steps (for accurate documentation)

## Estimated Effort
Medium - Documentation site creation
