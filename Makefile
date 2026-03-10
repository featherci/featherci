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

.PHONY: all build dev test lint fmt clean css tidy help generate-key

all: build

## build: Compile the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "Built $(BUILD_DIR)/$(BINARY_NAME)"

## dev: Run in development mode
dev:
	$(GORUN) $(LDFLAGS) $(CMD_DIR)

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

## css: Compile Tailwind CSS (requires npm dependencies)
css:
	@if [ -f web/tailwind/package.json ]; then \
		cd web/tailwind && npm run build; \
	else \
		echo "Tailwind not set up yet. See plan/07-tailwind-setup.md"; \
	fi

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
