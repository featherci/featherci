# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

### Added
- GitHub, GitLab, and Gitea/Forgejo OAuth authentication
- Workflow-based pipeline configuration (`.featherci/workflow.yml`)
- DAG pipeline visualization with SVG graph
- Manual approval gates for deployment workflows
- Conditional steps with branch/event expressions
- AES-256 encrypted secrets management
- Build caching with template-based cache keys
- Real-time log streaming
- Notification channels: Email (SMTP, SendGrid, Mailgun), Slack, Discord, Pushover
- Distributed worker support (standalone, master, worker modes)
- Docker-based step execution
- Commit status posting to GitHub, GitLab, and Gitea
- Admin panel for user and project management
- Docker and systemd deployment options
- Cross-platform release builds (Linux/macOS, amd64/arm64)
