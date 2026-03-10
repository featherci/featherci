---
model: sonnet
---

# Step 24: Docker Container Build

## Objective
Create Dockerfile and container build configuration for distributing FeatherCI as a Docker image.

## Tasks

### 24.1 Create Multi-stage Dockerfile
`Dockerfile`:
```dockerfile
# Build stage
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates curl

WORKDIR /src

# Download Tailwind CLI
ARG TAILWIND_VERSION=3.4.1
RUN curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/download/v${TAILWIND_VERSION}/tailwindcss-linux-x64 \
    && chmod +x tailwindcss-linux-x64 \
    && mv tailwindcss-linux-x64 /usr/local/bin/tailwindcss

# Download dependencies first (cacheable layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build CSS
RUN tailwindcss -c web/tailwind/tailwind.config.js -i web/tailwind/input.css -o web/static/css/main.css --minify

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /featherci ./cmd/featherci

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache \
    ca-certificates \
    git \
    docker-cli \
    tzdata

# Create non-root user
RUN addgroup -g 1000 featherci && \
    adduser -D -u 1000 -G featherci featherci

# Create directories
RUN mkdir -p /data /cache /logs && \
    chown -R featherci:featherci /data /cache /logs

# Copy binary
COPY --from=builder /featherci /usr/local/bin/featherci

# Default environment
ENV FEATHERCI_DATABASE_PATH=/data/featherci.db
ENV FEATHERCI_CACHE_PATH=/cache
ENV FEATHERCI_LOG_PATH=/logs
ENV FEATHERCI_BIND_ADDR=:8080

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:8080/health || exit 1

# Run as non-root user
USER featherci

# Volumes for persistent data
VOLUME ["/data", "/cache", "/logs"]

ENTRYPOINT ["featherci"]
```

### 24.2 Create Docker Compose File
`docker-compose.yml`:
```yaml
version: '3.8'

services:
  featherci:
    image: featherci/featherci:latest
    build: .
    ports:
      - "8080:8080"
    environment:
      - FEATHERCI_BASE_URL=http://localhost:8080
      - FEATHERCI_SECRET_KEY=${FEATHERCI_SECRET_KEY}
      - FEATHERCI_ADMINS=${FEATHERCI_ADMINS}
      - FEATHERCI_GITHUB_CLIENT_ID=${FEATHERCI_GITHUB_CLIENT_ID}
      - FEATHERCI_GITHUB_CLIENT_SECRET=${FEATHERCI_GITHUB_CLIENT_SECRET}
    volumes:
      - featherci-data:/data
      - featherci-cache:/cache
      - featherci-logs:/logs
      - /var/run/docker.sock:/var/run/docker.sock:ro
    restart: unless-stopped

volumes:
  featherci-data:
  featherci-cache:
  featherci-logs:
```

### 24.3 Create Docker Compose for Development
`docker-compose.dev.yml`:
```yaml
version: '3.8'

services:
  featherci:
    build:
      context: .
      dockerfile: Dockerfile.dev
    ports:
      - "8080:8080"
    environment:
      - FEATHERCI_BASE_URL=http://localhost:8080
      - FEATHERCI_SECRET_KEY=development-key-32-bytes-long!!
      - FEATHERCI_ADMINS=testuser
    volumes:
      - .:/src
      - /var/run/docker.sock:/var/run/docker.sock:ro
    command: ["go", "run", "./cmd/featherci"]
```

### 24.4 Create .dockerignore
`.dockerignore`:
```
# Git
.git
.gitignore

# IDE
.idea
.vscode
*.swp
*.swo

# Build artifacts
bin/
dist/

# Test files
*_test.go
testdata/

# Documentation
docs/
*.md
!README.md

# CI files
.github/
.gitlab-ci.yml

# Local config
.env
.env.*
*.db

# Plan files
plan/
```

### 24.5 Create Worker Dockerfile
For running standalone workers:
`Dockerfile.worker`:
```dockerfile
FROM alpine:3.19

RUN apk add --no-cache \
    ca-certificates \
    git \
    docker-cli

COPY --from=builder /featherci /usr/local/bin/featherci

ENV FEATHERCI_MODE=worker

ENTRYPOINT ["featherci"]
CMD ["--mode=worker"]
```

### 24.6 Add Makefile Targets
```makefile
.PHONY: docker-build
docker-build:
	docker build -t featherci/featherci:latest .

.PHONY: docker-push
docker-push: docker-build
	docker push featherci/featherci:latest

.PHONY: docker-run
docker-run:
	docker-compose up -d

.PHONY: docker-logs
docker-logs:
	docker-compose logs -f

.PHONY: docker-stop
docker-stop:
	docker-compose down
```

### 24.7 Add Tests
- Test Docker build completes
- Test container starts and responds to health check
- Test container can connect to Docker socket

## Deliverables
- [ ] `Dockerfile` - Production multi-stage build
- [ ] `Dockerfile.dev` - Development build
- [ ] `docker-compose.yml` - Production compose file
- [ ] `docker-compose.dev.yml` - Development compose file
- [ ] `.dockerignore` - Build exclusions
- [ ] Makefile targets for Docker operations
- [ ] Container builds and runs successfully

## Dependencies
- All previous steps (complete application)

## Estimated Effort
Small - Standard Docker packaging
