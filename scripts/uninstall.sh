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
sudo rm -f /etc/systemd/system/featherci-worker.service
sudo systemctl daemon-reload

echo "FeatherCI uninstalled."
echo "Configuration and data remain in /etc/featherci and /var/lib/featherci"
echo "Remove manually if no longer needed."
