#!/usr/bin/env bash
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

echo "=== Dockify Updater ==="
echo ""

# Skip sudo if already root (e.g. when invoked via systemd-run)
if [ "$(id -u)" -eq 0 ]; then
  SUDO=""
else
  SUDO="sudo"
fi

# Detect installation mode: binary+caddy, binary, or docker
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

# Fetch latest version from GitHub
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
  echo -e "${GREEN}Dockify updated to $VERSION${NC}"
  echo "Logs: docker compose -f /opt/dockify/docker-compose.yml logs -f"
  exit 0
fi

# Use a temp dir so stale files never block a future run
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT
BINARY="$TMP_DIR/dockify"

echo "Downloading Dockify $VERSION..."
curl -fsSL -o "$BINARY" \
  "https://github.com/coderbuzz/dockify/releases/download/${VERSION}/dockify-linux-amd64"
chmod +x "$BINARY"

echo ""
echo "Validating new binary..."
if ! "$BINARY" version >/dev/null 2>&1; then
  echo -e "${RED}Error: new binary validation failed (cannot execute). Keeping current version.${NC}"
  exit 1
fi
NEW_VERSION=$("$BINARY" version 2>&1)
echo "New binary version: $NEW_VERSION"

CURRENT_VERSION=$(/usr/local/bin/dockify version 2>/dev/null || echo "unknown")
echo "Current version:    $CURRENT_VERSION"

if [ "$NEW_VERSION" = "$CURRENT_VERSION" ]; then
  echo ""
  echo "Already running $CURRENT_VERSION."
  FORCE="${DOCKIFY_FORCE:-n}"
  if [ "${FORCE,,}" != "y" ] && [ -c /dev/tty ]; then
    read -p "Force reinstall? [y/N] " FORCE < /dev/tty
  fi
  if [ "${FORCE,,}" != "y" ]; then
    exit 0
  fi
  echo "Forcing reinstall..."
fi

echo ""
echo "Backing up current binary..."
$SUDO cp /usr/local/bin/dockify /usr/local/bin/dockify.bak

echo "Stopping Dockify..."
$SUDO systemctl stop dockify

echo "Installing new binary..."
$SUDO mv "$BINARY" /usr/local/bin/dockify

echo "Starting Dockify..."
$SUDO systemctl start dockify

sleep 2

if systemctl is-active --quiet dockify; then
  echo -e "${GREEN}Dockify started successfully.${NC}"
  $SUDO rm -f /usr/local/bin/dockify.bak
else
  echo -e "${RED}Error: new version failed to start. Rolling back...${NC}"
  $SUDO systemctl stop dockify 2>/dev/null || true
  $SUDO mv /usr/local/bin/dockify.bak /usr/local/bin/dockify
  $SUDO systemctl start dockify
  sleep 1
  if systemctl is-active --quiet dockify; then
  echo -e "${GREEN}Rollback successful. Previous version restored.${NC}"
  else
  echo -e "${RED}Rollback failed. Manual intervention required.${NC}"
  fi
  exit 1
fi

if [ "$MODE" = "binary+caddy" ]; then
  echo "Restarting Caddy..."
  $SUDO systemctl restart dockify-caddy
fi

echo ""
echo -e "${GREEN}Dockify updated to $VERSION${NC}"
if [ "$MODE" = "binary+caddy" ]; then
  echo "Logs: journalctl -u dockify -f | journalctl -u dockify-caddy -f"
else
  echo "Logs: journalctl -u dockify -f"
fi
