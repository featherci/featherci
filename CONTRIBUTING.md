# Contributing to FeatherCI

Thank you for your interest in contributing!

## Development Setup

1. Install Go 1.22+
2. Install Docker
3. Clone the repository
4. Run `make dev` to start the development server

Development mode skips OAuth and auto-logs in as an admin, so you don't need to configure OAuth providers for local development.

## Code Style

- Run `make fmt` to format all Go files
- Follow [Effective Go](https://go.dev/doc/effective_go) conventions
- Add tests for new features
- Run `make lint` to check for common issues

## Pull Request Process

1. Create a feature branch from `master`
2. Make your changes
3. Run `make test` to verify tests pass
4. Run `make lint` to check for linting issues
5. Submit a PR with a clear description of what changed and why

## Reporting Issues

Please include:
- FeatherCI version (`featherci --version`)
- Operating system and architecture
- Steps to reproduce
- Expected vs actual behavior
- Relevant log output
