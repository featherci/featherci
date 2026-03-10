# Build stage
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src

# Download dependencies first (cacheable layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy source (includes pre-built CSS and JS assets)
COPY . .

# Build binary with version info
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w \
        -X github.com/featherci/featherci/internal/version.Version=${VERSION} \
        -X github.com/featherci/featherci/internal/version.Commit=${COMMIT} \
        -X github.com/featherci/featherci/internal/version.BuildDate=${BUILD_DATE}" \
    -o /featherci ./cmd/featherci

# Runtime stage
FROM alpine:3.21

RUN apk add --no-cache \
    ca-certificates \
    git \
    docker-cli \
    tzdata

# Create non-root user
RUN addgroup -g 1000 featherci && \
    adduser -D -u 1000 -G featherci featherci

# Create directories
RUN mkdir -p /data /cache /workspaces && \
    chown -R featherci:featherci /data /cache /workspaces

# Copy binary
COPY --from=builder /featherci /usr/local/bin/featherci

# Default environment
ENV FEATHERCI_DATABASE_PATH=/data/featherci.db
ENV FEATHERCI_CACHE_PATH=/cache
ENV FEATHERCI_WORKSPACE_PATH=/workspaces
ENV FEATHERCI_BIND_ADDR=:8080

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:8080/health || exit 1

USER featherci

VOLUME ["/data", "/cache", "/workspaces"]

ENTRYPOINT ["featherci"]
