#!/usr/bin/env bash
set -euo pipefail

echo "=== Dockify Updater ==="
echo ""

if systemctl cat dockify-caddy >/dev/null 2>&1; then
  MODE="binary+caddy"
elif systemctl cat dockify >/dev/null 2>&1; then
  MODE="binary"
elif [ -x /usr/local/bin/dockify ]; then
  MODE="binary"
elif docker ps --format '{{.Names}}' 2>/dev/null | grep -q dockify; then
  MODE="docker"
else
  echo "Error: cannot detect Dockify installation. Is it running?"
  exit 1
fi

echo "Detected mode: $MODE"
echo ""

echo "Fetching latest version..."
VERSION=$(curl -fsSL "https://api.github.com/repos/coderbuzz/dockify/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$VERSION" ]; then echo "Error: could not fetch latest version"; exit 1; fi
echo "Latest: $VERSION"
echo ""

if [ "$MODE" = "docker" ]; then
  echo "Updating Docker images..."
  cd /opt/dockify
  docker compose pull && docker compose up -d
  echo ""
  echo "Dockify updated to $VERSION"
  echo "Logs: docker compose -f /opt/dockify/docker-compose.yml logs -f"
  exit 0
fi

echo "Downloading Dockify $VERSION..."
curl -fsSL -o /tmp/dockify-update \
  "https://github.com/coderbuzz/dockify/releases/download/${VERSION}/dockify-linux-amd64"
sudo mv /tmp/dockify-update /usr/local/bin/dockify
sudo chmod +x /usr/local/bin/dockify

if [ "$MODE" = "binary+caddy" ]; then
  echo "Restarting Dockify..."
  sudo systemctl restart dockify
  echo ""
  echo "Dockify updated to $VERSION"
  echo "Logs: journalctl -u dockify -f | journalctl -u dockify-caddy -f"
else
  echo "Restarting service..."
  sudo systemctl restart dockify
  echo ""
  echo "Dockify updated to $VERSION"
  echo "Logs: journalctl -u dockify -f"
fi
