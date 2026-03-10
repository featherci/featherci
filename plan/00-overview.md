# FeatherCI - Project Overview

## Summary

FeatherCI is a lightweight, self-hosted CI/CD system written in Go with a modern web UI. It integrates with GitHub, GitLab, and Forgejo/Gitea for authentication and status reporting.

## Key Design Decisions

### Architecture
- **Single binary**: All assets (HTML, CSS, JS) embedded using Go's `embed` package
- **Database**: SQLite with configurable location via `FEATHERCI_DATABASE_PATH`
- **Workers**: Master/worker architecture with HTTP polling; master can also act as worker
- **UI**: Server-rendered HTML with Tailwind CSS, HTMX for interactivity, SVG for pipeline graphs

### Technology Stack
- **Go 1.22+**: Modern Go with standard library HTTP router
- **SQLite**: Via `modernc.org/sqlite` (pure Go, no CGO required)
- **Tailwind CSS**: Compiled at build time via standalone CLI
- **HTMX**: For dynamic updates without heavy JavaScript
- **Docker SDK**: For container orchestration on workers

### Authentication
- OAuth via GitHub, GitLab, and Forgejo/Gitea
- Administrators set via `FEATHERCI_ADMINS` environment variable (comma-separated usernames)
- Users can be added by admins with their platform username

### Security
- Secrets encrypted at rest using AES-256-GCM
- Master encryption key via `FEATHERCI_SECRET_KEY` environment variable
- Worker authentication via shared secret tokens

## File Structure

```
featherci/
├── cmd/
│   └── featherci/          # Main binary entry point
├── internal/
│   ├── config/             # Configuration loading
│   ├── database/           # SQLite setup and migrations
│   ├── models/             # Database models
│   ├── auth/               # OAuth providers
│   ├── handlers/           # HTTP handlers
│   ├── worker/             # Worker logic
│   ├── executor/           # Docker execution
│   ├── workflow/           # Workflow parsing
│   ├── git/                # Git operations
│   ├── cache/              # Build caching
│   └── crypto/             # Encryption utilities
├── web/
│   ├── templates/          # Go HTML templates
│   ├── static/             # Static assets (compiled CSS, JS)
│   └── tailwind/           # Tailwind source
├── scripts/
│   ├── systemd/            # systemd service files
│   ├── homebrew/           # Homebrew formula
│   └── docker/             # Dockerfile
├── plan/                   # This planning directory
└── docs/                   # Documentation
```

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `FEATHERCI_DATABASE_PATH` | No | SQLite database path (default: `./featherci.db`) |
| `FEATHERCI_SECRET_KEY` | Yes | 32-byte key for encryption (base64 encoded) |
| `FEATHERCI_ADMINS` | Yes | Comma-separated admin usernames |
| `FEATHERCI_BIND_ADDR` | No | Server bind address (default: `:8080`) |
| `FEATHERCI_BASE_URL` | Yes | Public URL for OAuth callbacks |
| `FEATHERCI_GITHUB_CLIENT_ID` | No | GitHub OAuth client ID |
| `FEATHERCI_GITHUB_CLIENT_SECRET` | No | GitHub OAuth client secret |
| `FEATHERCI_GITLAB_CLIENT_ID` | No | GitLab OAuth client ID |
| `FEATHERCI_GITLAB_CLIENT_SECRET` | No | GitLab OAuth client secret |
| `FEATHERCI_GITEA_URL` | No | Gitea/Forgejo instance URL |
| `FEATHERCI_GITEA_CLIENT_ID` | No | Gitea OAuth client ID |
| `FEATHERCI_GITEA_CLIENT_SECRET` | No | Gitea OAuth client secret |
| `FEATHERCI_WORKER_SECRET` | No | Shared secret for worker auth |
| `FEATHERCI_MODE` | No | `master`, `worker`, or `standalone` (default) |
| `FEATHERCI_MASTER_URL` | No | Master URL (for worker mode) |
| `FEATHERCI_CACHE_PATH` | No | Build cache directory (default: `./cache`) |

## Implementation Phases

1. **Foundation** (Steps 01-03): Project setup, database, configuration
2. **Authentication** (Steps 04-05): OAuth providers, user management
3. **Core UI** (Steps 06-08): Templates, styling, layouts
4. **Projects** (Steps 09-10): Project management, webhooks
5. **Workflow Engine** (Steps 11-14): Parsing, execution, Docker integration
6. **Workers** (Steps 15-16): Master/worker architecture, job distribution
7. **Build UI** (Steps 17-19): Build status, logs, pipeline graph
8. **Secrets & Caching** (Steps 20-21): Encrypted secrets, build caching
9. **Manual Approval** (Step 22): Approval gates for deployments
10. **Status Integration** (Step 23): Commit status posting to platforms
11. **Deployment** (Steps 24-25): Docker, systemd, Homebrew packaging
12. **Integration Testing** (Step 26): Test application with full workflow
13. **Notifications** (Step 27): Email (SMTP/Sendgrid/Mailgun), Slack, Discord, Pushover
14. **Polish** (Step 28): Final testing and polish
15. **Documentation** (Steps 29-30): README and GitHub Pages documentation site

## Progress Tracker

Use this checklist to track completion. Each step can be done in a fresh session - just reference the relevant `plan/XX-*.md` file.

- [x] **01 - Project Init** (Sonnet) - Go module, directories, Makefile
- [x] **02 - Configuration** (Sonnet) - Environment variable loading
- [x] **03 - Database** (Sonnet) - SQLite schema, migrations
- [x] **04 - OAuth Providers** (Sonnet) - GitHub, GitLab, Gitea OAuth
- [x] **05 - User Management** (Sonnet) - Sessions, auth middleware
- [x] **06 - Web Server** (Sonnet) - HTTP routing, middleware
- [x] **07 - Tailwind Setup** (Sonnet) - CSS build, HTMX
- [x] **08 - Templates** (Opus) - Base layout, components
- [x] **09 - Project Management** (Opus) - Project CRUD, repo listing
- [x] **10 - Webhooks** (Sonnet) - Webhook handlers
- [x] **11 - Workflow Parsing** (Opus) - YAML parsing, validation
- [ ] **12 - Build Model** (Opus) - Build/step data models
- [ ] **13 - Git Operations** (Sonnet) - Clone, checkout
- [ ] **14 - Docker Executor** (Opus) - Container execution
- [ ] **15 - Worker Architecture** (Opus) - Master/worker system
- [ ] **16 - Build Orchestration** (Opus) - Step scheduling
- [ ] **17 - Build Status UI** (Opus) - Build list/detail pages
- [ ] **18 - Log Streaming** (Sonnet) - SSE log streaming
- [ ] **19 - Pipeline Graph** (Opus) - SVG DAG visualization
- [ ] **20 - Secrets** (Sonnet) - Encrypted secrets
- [ ] **21 - Caching** (Sonnet) - Build cache
- [ ] **22 - Manual Approval** (Sonnet) - Approval gates
- [ ] **23 - Commit Status** (Sonnet) - Status posting to platforms
- [ ] **24 - Docker Build** (Sonnet) - Dockerfile, compose
- [ ] **25 - Packaging** (Sonnet) - systemd, Homebrew
- [ ] **26 - Integration Testing** (Opus) - Test app setup, end-to-end testing
- [ ] **27 - Notifications** (Opus) - Email, Slack, Discord, Pushover
- [ ] **28 - Testing & Polish** (Opus) - Tests, final checks
- [ ] **29 - README** (Sonnet) - Repository docs
- [ ] **30 - Documentation Site** (Opus) - GitHub Pages site

## Model Assignments

| Step | Name | Model | Rationale |
|------|------|-------|-----------|
| 01 | Project Init | Sonnet | Standard boilerplate |
| 02 | Configuration | Sonnet | Straightforward env loading |
| 03 | Database | Sonnet | Standard schema work |
| 04 | OAuth Providers | Sonnet | Well-documented APIs |
| 05 | User Management | Sonnet | Standard auth patterns |
| 06 | Web Server | Sonnet | Standard Go HTTP |
| 07 | Tailwind Setup | Sonnet | Build tooling config |
| 08 | Templates | **Opus** | Core UI design, component library |
| 09 | Project Management | **Opus** | Complex user/project relationships |
| 10 | Webhooks | Sonnet | Standard webhook handling |
| 11 | Workflow Parsing | **Opus** | Complex parsing, validation, DAG logic |
| 12 | Build Model | **Opus** | Core domain model, state machines |
| 13 | Git Operations | Sonnet | Standard git CLI wrapper |
| 14 | Docker Executor | **Opus** | Docker SDK complexity, error handling |
| 15 | Worker Architecture | **Opus** | Distributed systems, concurrency |
| 16 | Build Orchestration | **Opus** | Complex state management, scheduling |
| 17 | Build Status UI | **Opus** | Key user-facing UI, build visualization |
| 18 | Log Streaming | Sonnet | Standard SSE implementation |
| 19 | Pipeline Graph | **Opus** | Complex graph layout algorithm |
| 20 | Secrets | Sonnet | Standard crypto patterns |
| 21 | Caching | Sonnet | File-based caching |
| 22 | Manual Approval | Sonnet | Simple state transition |
| 23 | Commit Status | Sonnet | Standard API integration |
| 24 | Docker Build | Sonnet | Standard Dockerfile |
| 25 | Packaging | Sonnet | Standard scripts |
| 26 | Integration Testing | **Opus** | Test app with complex workflow, user coordination |
| 27 | Notifications | **Opus** | Multiple integrations, email templates, async dispatch |
| 28 | Testing & Polish | **Opus** | Comprehensive testing strategy |
| 29 | README | Sonnet | Documentation writing |
| 30 | Documentation Site | **Opus** | Public website design, polished UX |

**Summary:** 14 steps use Opus (complex logic + UI design), 16 steps use Sonnet (standard patterns)
