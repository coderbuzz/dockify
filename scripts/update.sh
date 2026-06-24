#!/usr/bin/env bash
set -euo pipefail

echo "=== Dockify Updater ==="
echo ""

# Detect install mode
if systemctl is-active --quiet dockify-caddy 2>/dev/null; then
  MODE="binary+caddy"
elif systemctl is-active --quiet dockify 2>/dev/null; then
  MODE="binary"
elif docker ps --format '{{.Names}}' 2>/dev/null | grep -q dockify; then
  MODE="docker"
else
  echo "Error: cannot detect Dockify installation. Is it running?"
  exit 1
fi

echo "Detected mode: $MODE"
echo ""

# Fetch latest version
echo "Fetching latest version..."
VERSION=$(curl -fsSL "https://api.github.com/repos/coderbuzz/dockify/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$VERSION" ]; then
  echo "Error: could not fetch latest version"
  exit 1
fi
echo "Latest: $VERSION"
echo ""

if [ "$MODE" = "docker" ]; then
  echo "Updating Docker images..."
  cd /opt/dockify
  docker compose pull
  docker compose up -d
  echo ""
  echo "Dockify updated to $VERSION"
  echo "Logs: docker compose -f /opt/dockify/docker-compose.yml logs -f"

elif [ "$MODE" = "binary+caddy" ]; then
  echo "Downloading Dockify $VERSION..."
  sudo curl -fsSL -o /usr/local/bin/dockify \
    "https://github.com/coderbuzz/dockify/releases/download/${VERSION}/dockify-linux-amd64"
  sudo chmod +x /usr/local/bin/dockify

  echo "Restarting services..."
  sudo systemctl restart dockify-caddy

  echo ""
  echo "Dockify updated to $VERSION"
  echo "Logs: journalctl -u dockify-caddy -f"

else
  echo "Downloading Dockify $VERSION..."
  sudo curl -fsSL -o /usr/local/bin/dockify \
    "https://github.com/coderbuzz/dockify/releases/download/${VERSION}/dockify-linux-amd64"
  sudo chmod +x /usr/local/bin/dockify

  echo "Restarting service..."
  sudo systemctl restart dockify

  echo ""
  echo "Dockify updated to $VERSION"
  echo "Logs: journalctl -u dockify -f"
fi
