---
model: sonnet
---

# Step 25: systemd and Homebrew Packaging

## Objective
Create systemd service files for Linux and Homebrew formula for macOS installation.

## Tasks

### 25.1 Create systemd Service File
`scripts/systemd/featherci.service`:
```ini
[Unit]
Description=FeatherCI - Lightweight CI/CD System
Documentation=https://github.com/featherci/featherci
After=network.target docker.service
Requires=docker.service

[Service]
Type=simple
User=featherci
Group=featherci
ExecStart=/usr/local/bin/featherci
Restart=always
RestartSec=5

# Environment file for configuration
EnvironmentFile=-/etc/featherci/featherci.env

# Working directory
WorkingDirectory=/var/lib/featherci

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=featherci

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/var/lib/featherci /var/log/featherci

# Resource limits
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
```

### 25.2 Create systemd Worker Service
`scripts/systemd/featherci-worker.service`:
```ini
[Unit]
Description=FeatherCI Worker
Documentation=https://github.com/featherci/featherci
After=network.target docker.service
Requires=docker.service

[Service]
Type=simple
User=featherci
Group=featherci
ExecStart=/usr/local/bin/featherci --mode=worker
Restart=always
RestartSec=5

EnvironmentFile=-/etc/featherci/featherci-worker.env

WorkingDirectory=/var/lib/featherci-worker

StandardOutput=journal
StandardError=journal
SyslogIdentifier=featherci-worker

NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/var/lib/featherci-worker

[Install]
WantedBy=multi-user.target
```

### 25.3 Create Installation Script
`scripts/install.sh`:
```bash
#!/bin/bash
set -e

VERSION="${1:-latest}"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/featherci"
DATA_DIR="/var/lib/featherci"
LOG_DIR="/var/log/featherci"

echo "Installing FeatherCI ${VERSION}..."

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case $ARCH in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Download binary
DOWNLOAD_URL="https://github.com/featherci/featherci/releases/download/${VERSION}/featherci-${OS}-${ARCH}"
echo "Downloading from ${DOWNLOAD_URL}..."
curl -L -o /tmp/featherci "${DOWNLOAD_URL}"
chmod +x /tmp/featherci
sudo mv /tmp/featherci "${INSTALL_DIR}/featherci"

# Create user and group
if ! id featherci &>/dev/null; then
    sudo useradd --system --no-create-home --shell /bin/false featherci
fi

# Create directories
sudo mkdir -p "${CONFIG_DIR}" "${DATA_DIR}" "${LOG_DIR}"
sudo chown featherci:featherci "${DATA_DIR}" "${LOG_DIR}"

# Install systemd service
if [ -d /etc/systemd/system ]; then
    sudo curl -L -o /etc/systemd/system/featherci.service \
        "https://raw.githubusercontent.com/featherci/featherci/${VERSION}/scripts/systemd/featherci.service"
    sudo systemctl daemon-reload
    echo "systemd service installed. Enable with: sudo systemctl enable featherci"
fi

# Create example config
if [ ! -f "${CONFIG_DIR}/featherci.env" ]; then
    cat <<EOF | sudo tee "${CONFIG_DIR}/featherci.env"
# FeatherCI Configuration
# See https://github.com/featherci/featherci for documentation

FEATHERCI_DATABASE_PATH=${DATA_DIR}/featherci.db
FEATHERCI_CACHE_PATH=${DATA_DIR}/cache
FEATHERCI_BIND_ADDR=:8080
FEATHERCI_BASE_URL=http://localhost:8080

# Generate with: openssl rand -base64 32
FEATHERCI_SECRET_KEY=

# Comma-separated GitHub/GitLab/Gitea usernames
FEATHERCI_ADMINS=

# GitHub OAuth (https://github.com/settings/developers)
FEATHERCI_GITHUB_CLIENT_ID=
FEATHERCI_GITHUB_CLIENT_SECRET=
EOF
    echo "Example configuration created at ${CONFIG_DIR}/featherci.env"
fi

echo "Installation complete!"
echo ""
echo "Next steps:"
echo "  1. Edit ${CONFIG_DIR}/featherci.env with your configuration"
echo "  2. Start with: sudo systemctl start featherci"
echo "  3. Enable on boot: sudo systemctl enable featherci"
```

### 25.4 Create Homebrew Formula
`scripts/homebrew/featherci.rb`:
```ruby
class Featherci < Formula
  desc "Lightweight CI/CD system"
  homepage "https://github.com/featherci/featherci"
  version "0.1.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/featherci/featherci/releases/download/v#{version}/featherci-darwin-arm64.tar.gz"
      sha256 "PLACEHOLDER_ARM64_SHA256"
    else
      url "https://github.com/featherci/featherci/releases/download/v#{version}/featherci-darwin-amd64.tar.gz"
      sha256 "PLACEHOLDER_AMD64_SHA256"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/featherci/featherci/releases/download/v#{version}/featherci-linux-arm64.tar.gz"
      sha256 "PLACEHOLDER_LINUX_ARM64_SHA256"
    else
      url "https://github.com/featherci/featherci/releases/download/v#{version}/featherci-linux-amd64.tar.gz"
      sha256 "PLACEHOLDER_LINUX_AMD64_SHA256"
    end
  end

  depends_on "docker" => :optional

  def install
    bin.install "featherci"
  end

  def post_install
    (var/"featherci").mkpath
    (var/"log/featherci").mkpath
  end

  def caveats
    <<~EOS
      To start FeatherCI, first create a configuration file:
        cp #{etc}/featherci/featherci.env.example #{etc}/featherci/featherci.env
        # Edit the file with your settings

      Then start the service:
        brew services start featherci

      Or run manually:
        featherci

      Configuration documentation:
        https://github.com/featherci/featherci#configuration
    EOS
  end

  service do
    run [opt_bin/"featherci"]
    environment_variables FEATHERCI_DATABASE_PATH: var/"featherci/featherci.db",
                          FEATHERCI_CACHE_PATH: var/"featherci/cache",
                          FEATHERCI_LOG_PATH: var/"log/featherci"
    keep_alive true
    working_dir var/"featherci"
    log_path var/"log/featherci/featherci.log"
    error_log_path var/"log/featherci/featherci.error.log"
  end

  test do
    assert_match "featherci version", shell_output("#{bin}/featherci --version")
  end
end
```

### 25.5 Create Example Environment File
`scripts/featherci.env.example`:
```bash
# FeatherCI Configuration

# Database location
FEATHERCI_DATABASE_PATH=/var/lib/featherci/featherci.db

# Cache directory for build artifacts
FEATHERCI_CACHE_PATH=/var/lib/featherci/cache

# Server binding
FEATHERCI_BIND_ADDR=:8080

# Public URL (required for OAuth callbacks)
FEATHERCI_BASE_URL=https://ci.example.com

# Encryption key for secrets (required)
# Generate with: openssl rand -base64 32
FEATHERCI_SECRET_KEY=

# Administrator usernames (comma-separated)
FEATHERCI_ADMINS=yourusername

# GitHub OAuth credentials
# Create at: https://github.com/settings/developers
FEATHERCI_GITHUB_CLIENT_ID=
FEATHERCI_GITHUB_CLIENT_SECRET=

# GitLab OAuth credentials (optional)
# FEATHERCI_GITLAB_URL=https://gitlab.com
# FEATHERCI_GITLAB_CLIENT_ID=
# FEATHERCI_GITLAB_CLIENT_SECRET=

# Gitea/Forgejo OAuth credentials (optional)
# FEATHERCI_GITEA_URL=https://gitea.example.com
# FEATHERCI_GITEA_CLIENT_ID=
# FEATHERCI_GITEA_CLIENT_SECRET=

# Worker mode: master, worker, or standalone (default)
# FEATHERCI_MODE=standalone

# For worker mode only:
# FEATHERCI_MASTER_URL=https://ci.example.com
# FEATHERCI_WORKER_SECRET=shared-worker-secret
```

### 25.6 Create Uninstall Script
`scripts/uninstall.sh`:
```bash
#!/bin/bash
set -e

echo "Uninstalling FeatherCI..."

# Stop service
sudo systemctl stop featherci 2>/dev/null || true
sudo systemctl disable featherci 2>/dev/null || true

# Remove binary
sudo rm -f /usr/local/bin/featherci

# Remove systemd service
sudo rm -f /etc/systemd/system/featherci.service
sudo systemctl daemon-reload

echo "FeatherCI uninstalled."
echo "Configuration and data remain in /etc/featherci and /var/lib/featherci"
echo "Remove manually if no longer needed."
```

### 25.7 Add Makefile Targets
```makefile
.PHONY: release
release: build
	mkdir -p dist
	# Linux AMD64
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/featherci-linux-amd64 ./cmd/featherci
	# Linux ARM64
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o dist/featherci-linux-arm64 ./cmd/featherci
	# Darwin AMD64
	GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o dist/featherci-darwin-amd64 ./cmd/featherci
	# Darwin ARM64
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o dist/featherci-darwin-arm64 ./cmd/featherci
	# Create tarballs
	cd dist && for f in featherci-*; do tar -czf "$$f.tar.gz" "$$f"; done

.PHONY: install
install: build
	sudo cp bin/featherci /usr/local/bin/
```

## Deliverables
- [ ] `scripts/systemd/featherci.service` - Main systemd service
- [ ] `scripts/systemd/featherci-worker.service` - Worker systemd service
- [ ] `scripts/install.sh` - Linux installation script
- [ ] `scripts/uninstall.sh` - Uninstallation script
- [ ] `scripts/homebrew/featherci.rb` - Homebrew formula
- [ ] `scripts/featherci.env.example` - Example configuration
- [ ] Makefile release targets

## Dependencies
- All previous steps (complete application)

## Estimated Effort
Small - Standard packaging scripts
