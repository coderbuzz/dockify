#!/usr/bin/env bash
set -e

REPO="https://github.com/coderbuzz/dockify"
RAW="https://raw.githubusercontent.com/coderbuzz/dockify/main"
INSTALL_DIR="${INSTALL_DIR:-/opt/dockify}"

echo "=== Dockify Installer ==="
echo ""

OS=$(uname -s)
ARCH=$(uname -m)
if [ "$OS" != "Linux" ]; then
    echo "Error: Dockify only supports Linux. Detected: $OS"
    exit 1
fi
if [ "$ARCH" != "x86_64" ]; then
    echo "Error: Dockify only supports x86_64. Detected: $ARCH"
    exit 1
fi

# --- Collect config ---
read -p "Domain for Dockify (e.g., dockify.amg.id): " DOMAIN
if [ -z "$DOMAIN" ]; then
    echo "Error: domain is required"
    exit 1
fi

echo ""
echo "Cloudflare API credentials are optional. They enable automatic DNS A record"
echo "creation when you deploy apps to worker VMs."
echo ""
read -p "Cloudflare API Token (optional, press Enter to skip): " CF_TOKEN
read -p "Cloudflare Zone ID (optional): " CF_ZONE

echo ""
echo "Admin credentials for web UI login. If no password is set, the web UI will"
echo "have no authentication (open to anyone)."
echo ""
read -p "Admin username [admin]: " ADMIN_USER
ADMIN_USER="${ADMIN_USER:-admin}"
read -s -p "Admin password (optional, press Enter to skip): " ADMIN_PASS
echo ""

echo ""
echo "DOCKIFY_BASE_PATH is only needed when accessing Dockify through a URL prefix"
echo "(e.g., behind code-server proxy: /proxy/9898). Leave empty for normal access."
echo ""
read -p "Base path (optional, press Enter to skip): " BASE_PATH

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
