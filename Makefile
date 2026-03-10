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
TAILWIND_VERSION := 3.4.1
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

.PHONY: all build dev test lint fmt clean css css-watch tidy help generate-key tailwind-download htmx-download assets

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
	$(TAILWIND_CLI) -c web/tailwind/tailwind.config.js -i web/tailwind/input.css -o web/static/css/main.css --watch

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
	@$(TAILWIND_CLI) -c web/tailwind/tailwind.config.js -i web/tailwind/input.css -o web/static/css/main.css --minify
	@echo "CSS compiled to web/static/css/main.css"

## css-watch: Watch and recompile CSS on changes
css-watch: tailwind-download
	$(TAILWIND_CLI) -c web/tailwind/tailwind.config.js -i web/tailwind/input.css -o web/static/css/main.css --watch

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

## help: Show this help message
help:
	@echo "FeatherCI Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
