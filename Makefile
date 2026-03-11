# FeatherCI Makefile

# Build variables
BINARY_NAME := featherci
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-X github.com/featherci/featherci/internal/version.Version=$(VERSION) \
	-X github.com/featherci/featherci/internal/version.Commit=$(COMMIT) \
	-X github.com/featherci/featherci/internal/version.BuildDate=$(BUILD_DATE)"

# Go commands
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GORUN := $(GOCMD) run
GOMOD := $(GOCMD) mod
GOFMT := gofmt

# Directories
BUILD_DIR := ./bin
CMD_DIR := ./cmd/featherci

# Tailwind CSS standalone CLI
TAILWIND_VERSION := 4.2.1
TAILWIND_CLI := $(BUILD_DIR)/tailwindcss

# Detect OS/Arch for Tailwind download
ifeq ($(shell uname -s),Darwin)
    ifeq ($(shell uname -m),arm64)
        TAILWIND_PLATFORM := macos-arm64
    else
        TAILWIND_PLATFORM := macos-x64
    endif
else
    ifeq ($(shell uname -m),aarch64)
        TAILWIND_PLATFORM := linux-arm64
    else
        TAILWIND_PLATFORM := linux-x64
    endif
endif

# HTMX version
HTMX_VERSION := 1.9.10

.PHONY: all build dev test lint fmt clean css css-watch tidy help generate-key tailwind-download htmx-download assets docker-build docker-run docker-stop docker-logs release install docs-css docs-css-watch docs-serve bump-patch bump-minor bump-major

all: build

## build: Compile the binary (includes CSS and JS assets)
build: assets
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "Built $(BUILD_DIR)/$(BINARY_NAME)"

## dev: Run in development mode (builds assets first)
dev: assets
	$(GORUN) $(LDFLAGS) $(CMD_DIR) --dev

## dev-watch: Run with CSS watch mode (run in separate terminal)
dev-watch: tailwind-download
	$(TAILWIND_CLI) -i web/tailwind/input.css -o web/static/css/main.css --watch

## assets: Build all frontend assets (CSS and JS)
assets: css htmx-download

## tailwind-download: Download Tailwind CSS standalone CLI
tailwind-download:
	@mkdir -p $(BUILD_DIR)
	@if [ ! -f $(TAILWIND_CLI) ]; then \
		echo "Downloading Tailwind CSS CLI v$(TAILWIND_VERSION)..."; \
		curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/download/v$(TAILWIND_VERSION)/tailwindcss-$(TAILWIND_PLATFORM); \
		mv tailwindcss-$(TAILWIND_PLATFORM) $(TAILWIND_CLI); \
		chmod +x $(TAILWIND_CLI); \
		echo "Tailwind CSS CLI downloaded to $(TAILWIND_CLI)"; \
	fi

## css: Compile Tailwind CSS
css: tailwind-download
	@echo "Compiling Tailwind CSS..."
	@$(TAILWIND_CLI) -i web/tailwind/input.css -o web/static/css/main.css --minify
	@echo "CSS compiled to web/static/css/main.css"

## css-watch: Watch and recompile CSS on changes
css-watch: tailwind-download
	$(TAILWIND_CLI) -i web/tailwind/input.css -o web/static/css/main.css --watch

## htmx-download: Download HTMX library
htmx-download:
	@mkdir -p web/static/js
	@if [ ! -f web/static/js/htmx.min.js ] || grep -q "Placeholder" web/static/js/htmx.min.js 2>/dev/null; then \
		echo "Downloading HTMX v$(HTMX_VERSION)..."; \
		curl -sL https://unpkg.com/htmx.org@$(HTMX_VERSION)/dist/htmx.min.js -o web/static/js/htmx.min.js; \
		echo "HTMX downloaded to web/static/js/htmx.min.js"; \
	fi

## test: Run tests
test:
	$(GOTEST) -v -race ./...

## lint: Run golangci-lint
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Install with: brew install golangci-lint"; \
		exit 1; \
	fi

## fmt: Run gofmt on all Go files
fmt:
	$(GOFMT) -s -w .

## fmt-check: Check if code is formatted (for CI)
fmt-check:
	@test -z "$$($(GOFMT) -l .)" || (echo "Code not formatted. Run 'make fmt'" && exit 1)

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f featherci.db
	@echo "Done."

## tidy: Tidy go modules
tidy:
	$(GOMOD) tidy

## generate-key: Generate a secure encryption key
generate-key: build
	@$(BUILD_DIR)/$(BINARY_NAME) --generate-key

## docker-build: Build the Docker image
docker-build:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t ghcr.io/featherci/featherci:latest \
		-t ghcr.io/featherci/featherci:$(VERSION) \
		.

## docker-run: Start FeatherCI via Docker Compose
docker-run:
	docker compose up -d

## docker-stop: Stop FeatherCI containers
docker-stop:
	docker compose down

## docker-logs: Tail FeatherCI container logs
docker-logs:
	docker compose logs -f

## release: Cross-compile release binaries for all platforms
release: assets
	@echo "Building release binaries..."
	@mkdir -p dist
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o dist/featherci-linux-amd64/featherci $(CMD_DIR)
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o dist/featherci-linux-arm64/featherci $(CMD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o dist/featherci-darwin-amd64/featherci $(CMD_DIR)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o dist/featherci-darwin-arm64/featherci $(CMD_DIR)
	@for d in dist/featherci-*; do cp scripts/config.yaml.example "$$d/config.yaml.example"; done
	@cd dist && for d in featherci-*; do tar -czf "$$d.tar.gz" -C "$$d" .; done
	@echo "Release binaries in dist/"

## install: Install binary to /usr/local/bin
install: build
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/

## docs-css: Compile Tailwind CSS for docs site
docs-css: tailwind-download
	@$(TAILWIND_CLI) -i docs/tailwind/input.css -o docs/css/docs.css --minify

## docs-css-watch: Watch and recompile docs CSS on changes
docs-css-watch: tailwind-download
	$(TAILWIND_CLI) -i docs/tailwind/input.css -o docs/css/docs.css --watch

## docs-serve: Serve docs site locally
docs-serve: docs-css
	cd docs && python3 -m http.server 3000

## bump-patch: Tag a new patch release (v0.1.0 → v0.1.1) and push
bump-patch:
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	major=$$(echo $$latest | sed 's/v//' | cut -d. -f1); \
	minor=$$(echo $$latest | sed 's/v//' | cut -d. -f2); \
	patch=$$(echo $$latest | sed 's/v//' | cut -d. -f3); \
	next="v$$major.$$minor.$$((patch + 1))"; \
	echo "Bumping $$latest → $$next"; \
	git tag -a $$next -m "Release $$next" && \
	git push origin $$next

## bump-minor: Tag a new minor release (v0.1.0 → v0.2.0) and push
bump-minor:
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	major=$$(echo $$latest | sed 's/v//' | cut -d. -f1); \
	minor=$$(echo $$latest | sed 's/v//' | cut -d. -f2); \
	next="v$$major.$$((minor + 1)).0"; \
	echo "Bumping $$latest → $$next"; \
	git tag -a $$next -m "Release $$next" && \
	git push origin $$next

## bump-major: Tag a new major release (v0.1.0 → v1.0.0) and push
bump-major:
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	major=$$(echo $$latest | sed 's/v//' | cut -d. -f1); \
	next="v$$((major + 1)).0.0"; \
	echo "Bumping $$latest → $$next"; \
	git tag -a $$next -m "Release $$next" && \
	git push origin $$next

## help: Show this help message
help:
	@echo "FeatherCI Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
