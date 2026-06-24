#!/usr/bin/env bash
set -e

REPO="https://github.com/coderbuzz/dockify"
RAW="https://raw.githubusercontent.com/coderbuzz/dockify/main"
INSTALL_DIR="${INSTALL_DIR:-/opt/dockify}"

echo "=== Dockify Installer ==="
echo ""

OS=$(uname -s)
ARCH=$(uname -m)
if [ "$OS" != "Linux" ]; then echo "Error: Linux only. Detected: $OS"; exit 1; fi
if [ "$ARCH" != "x86_64" ]; then echo "Error: x86_64 only. Detected: $ARCH"; exit 1; fi

# Try to detect interactive terminal
TTY=""
[ -c /dev/tty ] && TTY="/dev/tty"

prompt() {
  local var="$1" msg="$2" default="$3" required="$4"
  local val="${!var}"
  if [ -z "$val" ] && [ -n "$TTY" ]; then
    read -p "$msg" val < "$TTY" || true
  fi
  [ -z "$val" ] && val="$default"
  if [ -z "$val" ] && [ -n "$required" ]; then
    echo "Error: $msg is required"
    exit 1
  fi
  printf -v "$var" "%s" "$val"
}

prompt_secret() {
  local var="$1" msg="$2"
  local val="${!var}"
  if [ -z "$val" ] && [ -n "$TTY" ]; then
    read -s -p "$msg" val < "$TTY" || true
    echo ""
  fi
  printf -v "$var" "%s" "$val"
}

# You can pre-set env vars for non-interactive install:
#   DOMAIN=dockify.amg.id ADMIN_PASS=secret curl ... | bash
prompt DOMAIN "Domain for Dockify (e.g., dockify.amg.id): " "" required

echo ""
echo "Cloudflare credentials (optional — enable auto DNS A records on deploy)"
prompt CF_TOKEN "Cloudflare API Token (optional, Enter to skip): "
prompt CF_ZONE  "Cloudflare Zone ID (optional): "

echo ""
echo "Admin credentials (optional — empty password = no authentication)"
prompt ADMIN_USER "Admin username [admin]: " admin
prompt_secret ADMIN_PASS "Admin password (optional, Enter to skip): "

prompt BASE_PATH "Base path for reverse proxy (optional, Enter to skip): "

echo ""
echo "Downloading files..."
sudo mkdir -p "$INSTALL_DIR"

curl -fsSL "$RAW/docker-compose.yml" -o "$INSTALL_DIR/docker-compose.yml"
curl -fsSL "$RAW/Caddyfile" -o "$INSTALL_DIR/Caddyfile"
curl -fsSL "$RAW/.env.example" -o "$INSTALL_DIR/.env.example"

echo "Creating .env..."
cat > "$INSTALL_DIR/.env" << ENVEOF
# Domain for Caddy reverse proxy (auto HTTPS)
DOMAIN=$DOMAIN

# Admin credentials (optional, enables web UI login)
DOCKIFY_ADMIN_USER=$ADMIN_USER
DOCKIFY_ADMIN_PASSWORD=$ADMIN_PASS

# Cloudflare API credentials (optional, auto DNS A record on deploy)
CLOUDFLARE_API_TOKEN=$CF_TOKEN
CLOUDFLARE_ZONE_ID=$CF_ZONE

# Base path when behind a reverse proxy (optional)
DOCKIFY_BASE_PATH=$BASE_PATH
ENVEOF

echo ""
echo "=== Dockify installed successfully! ==="
echo ""
echo "Directory: $INSTALL_DIR"
echo "Domain:    $DOMAIN"
echo ""
echo "Next steps:"
echo ""
echo "  1. Point $DOMAIN DNS A record to this server's IP"
echo "  2. Start Dockify:"
echo ""
echo "     cd $INSTALL_DIR"
echo "     docker compose up -d"
echo ""
echo "  3. Open https://$DOMAIN"
echo ""
echo "To stop:          docker compose -f $INSTALL_DIR/docker-compose.yml down"
echo "To view logs:     docker compose -f $INSTALL_DIR/docker-compose.yml logs -f"
echo "To update:        cd $INSTALL_DIR && docker compose pull && docker compose up -d"
