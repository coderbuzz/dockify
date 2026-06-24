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

TTY=""
[ -c /dev/tty ] && TTY="/dev/tty"

prompt() {
  local var="$1" msg="$2" default="$3" required="$4"
  local val="${!var}"
  if [ -z "$val" ] && [ -n "$TTY" ]; then
    read -p "$msg" val < "$TTY" || true
  fi
  [ -z "$val" ] && val="$default"
  if [ -z "$val" ] && [ -n "$required" ]; then echo "Error: $msg is required"; exit 1; fi
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

prompt MODE "Install mode (1=Docker Compose with Caddy, 2=Binary only, 3=Binary + Caddy) [1]: " "1"

if [ "$MODE" = "2" ] || [ "$MODE" = "3" ]; then
  INSTALL_BIN="${INSTALL_BIN:-/usr/local/bin}"
  echo ""
  echo "Admin credentials (optional - empty password = no auth)"
  prompt ADMIN_USER "Admin username [admin]: " admin
  prompt_secret ADMIN_PASS "Admin password (optional, Enter to skip): "
  prompt CF_TOKEN "Cloudflare API Token (optional): "
  prompt CF_ZONE  "Cloudflare Zone ID (optional): "
  prompt BASE_PATH "Base path (optional): "

  if [ "$MODE" = "3" ]; then
    prompt DOMAIN "Domain for Dockify (e.g., dockify.amg.id): " "" required
  fi

  echo ""
  echo "Fetching latest Dockify version..."
  VERSION=$(curl -fsSL "https://api.github.com/repos/coderbuzz/dockify/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/' | sed 's/^v//')
  [ -z "$VERSION" ] && VERSION="0.1.0"

  echo "Downloading Dockify v${VERSION}..."
  sudo mkdir -p "$INSTALL_DIR" "$INSTALL_DIR/keys"
  TMP_DIR=$(mktemp -d)
  curl -fsSL "https://github.com/coderbuzz/dockify/releases/download/v${VERSION}/dockify-linux-amd64" -o "$TMP_DIR/dockify"
  sudo mv "$TMP_DIR/dockify" "$INSTALL_BIN/dockify"
  sudo chmod +x "$INSTALL_BIN/dockify"
  rm -rf "$TMP_DIR"

  cat > "$INSTALL_DIR/.env" << ENVEOF
DOCKIFY_ADMIN_USER=$ADMIN_USER
DOCKIFY_ADMIN_PASSWORD=$ADMIN_PASS
CLOUDFLARE_API_TOKEN=$CF_TOKEN
CLOUDFLARE_ZONE_ID=$CF_ZONE
DOCKIFY_BASE_PATH=$BASE_PATH
ENVEOF

  sudo tee /etc/systemd/system/dockify.service > /dev/null << 'SYSTEMD'
[Unit]
Description=Dockify
After=network.target
[Service]
Type=simple
ExecStart=/usr/local/bin/dockify serve
Restart=on-failure
RestartSec=10
EnvironmentFile=/opt/dockify/.env
[Install]
WantedBy=multi-user.target
SYSTEMD

  sudo systemctl daemon-reload
  sudo systemctl enable dockify

  # Mode 3: install native Caddy
  if [ "$MODE" = "3" ]; then
    echo ""
    echo "Downloading Caddy..."
    curl -fsSL "https://caddyserver.com/api/download?os=linux&arch=amd64" -o "$TMP_DIR/caddy"
    sudo mv "$TMP_DIR/caddy" "$INSTALL_BIN/caddy"
    sudo chmod +x "$INSTALL_BIN/caddy"

    sudo tee /etc/systemd/system/dockify-caddy.service > /dev/null << 'SYSCTY'
[Unit]
Description=Caddy reverse proxy for Dockify
After=dockify.service
Requires=dockify.service
[Service]
Type=simple
ExecStart=/usr/local/bin/caddy run --config /opt/dockify/Caddyfile
ExecReload=/usr/local/bin/caddy reload --config /opt/dockify/Caddyfile
Restart=on-failure
RestartSec=10
[Install]
WantedBy=multi-user.target
SYSCTY

    cat > "$INSTALL_DIR/Caddyfile" << CADDYEOF
{
	debug false
	log { level error }
}

$DOMAIN {
	reverse_proxy 127.0.0.1:8080
}
CADDYEOF

    sudo systemctl daemon-reload
    sudo systemctl enable dockify-caddy
  fi

  echo ""
  if [ "$MODE" = "3" ]; then
    echo "=== Dockify v${VERSION} + Caddy installed! ==="
    echo "Start:  sudo systemctl start dockify-caddy"
    echo "        (starts dockify automatically)"
    echo "Logs:   journalctl -u dockify-caddy -f"
    echo "Config: $INSTALL_DIR/.env"
    echo ""
    echo "Open https://$DOMAIN"
  else
    echo "=== Dockify v${VERSION} installed! ==="
    echo "Start:  sudo systemctl start dockify"
    echo "Logs:   journalctl -u dockify -f"
    echo "Config: $INSTALL_DIR/.env"
    echo ""
    echo "Open http://<your-ip>:8080"
  fi

else
  prompt DOMAIN "Domain for Dockify (e.g., dockify.amg.id): " "" required
  echo ""
  echo "Cloudflare credentials (optional)"
  prompt CF_TOKEN "Cloudflare API Token: "
  prompt CF_ZONE  "Cloudflare Zone ID: "
  echo ""
  echo "Admin credentials (optional - empty password = no auth)"
  prompt ADMIN_USER "Admin username [admin]: " admin
  prompt_secret ADMIN_PASS "Admin password: "
  prompt BASE_PATH "Base path: "

  echo ""
  echo "Downloading files..."
  sudo mkdir -p "$INSTALL_DIR"
  curl -fsSL "$RAW/docker-compose.yml" -o "$INSTALL_DIR/docker-compose.yml"
  curl -fsSL "$RAW/Caddyfile" -o "$INSTALL_DIR/Caddyfile"

  cat > "$INSTALL_DIR/.env" << ENVEOF
DOMAIN=$DOMAIN
DOCKIFY_ADMIN_USER=$ADMIN_USER
DOCKIFY_ADMIN_PASSWORD=$ADMIN_PASS
CLOUDFLARE_API_TOKEN=$CF_TOKEN
CLOUDFLARE_ZONE_ID=$CF_ZONE
DOCKIFY_BASE_PATH=$BASE_PATH
ENVEOF

  echo ""
  echo "=== Dockify installed! ==="
  echo "Directory: $INSTALL_DIR"
  echo "Domain:    $DOMAIN"
  echo ""
  echo "Start:  cd $INSTALL_DIR && docker compose up -d"
  echo "Logs:   docker compose -f $INSTALL_DIR/docker-compose.yml logs -f"
  echo "Update: cd $INSTALL_DIR && docker compose pull && docker compose up -d"
  echo ""
  echo "Open https://$DOMAIN"
fi
