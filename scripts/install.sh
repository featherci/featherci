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

# Add featherci user to docker group for container execution
if getent group docker &>/dev/null; then
    sudo usermod -aG docker featherci
fi

# Create directories
sudo mkdir -p "${CONFIG_DIR}" "${DATA_DIR}" "${DATA_DIR}/cache" "${DATA_DIR}/workspaces" "${LOG_DIR}"
sudo chown featherci:featherci "${DATA_DIR}" "${DATA_DIR}/cache" "${DATA_DIR}/workspaces" "${LOG_DIR}"

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
FEATHERCI_WORKSPACE_PATH=${DATA_DIR}/workspaces
FEATHERCI_BIND_ADDR=:8080
FEATHERCI_BASE_URL=http://localhost:8080

# Generate with: featherci --generate-key
FEATHERCI_SECRET_KEY=

# Comma-separated GitHub/GitLab/Gitea usernames
FEATHERCI_ADMINS=

# GitHub OAuth (https://github.com/settings/developers)
FEATHERCI_GITHUB_CLIENT_ID=
FEATHERCI_GITHUB_CLIENT_SECRET=
EOF
    echo "Example configuration created at ${CONFIG_DIR}/featherci.env"
fi

echo ""
echo "Installation complete!"
echo ""
echo "Next steps:"
echo "  1. Edit ${CONFIG_DIR}/featherci.env with your configuration"
echo "  2. Start with: sudo systemctl start featherci"
echo "  3. Enable on boot: sudo systemctl enable featherci"
