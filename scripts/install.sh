#!/usr/bin/env bash
set -e

DOCKIFY_VERSION="${DOCKIFY_VERSION:-0.1.0}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
DATA_DIR="${DATA_DIR:-/var/lib/dockify}"
SERVICE_USER="${SERVICE_USER:-dockify}"

echo "=== Dockify v${DOCKIFY_VERSION} Installer ==="

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

echo "[1/5] Creating service user: $SERVICE_USER"
if ! id -u "$SERVICE_USER" >/dev/null 2>&1; then
    sudo useradd -r -s /bin/false -d "$DATA_DIR" -m "$SERVICE_USER"
fi

echo "[2/5] Creating data directories"
sudo mkdir -p "$DATA_DIR"
sudo mkdir -p "$DATA_DIR/keys"
sudo chown -R "$SERVICE_USER:$SERVICE_USER" "$DATA_DIR"

echo "[3/5] Downloading Dockify v${DOCKIFY_VERSION}..."
TMP_DIR=$(mktemp -d)
curl -fsSL "https://github.com/coderbuzz/dockify/releases/download/v${DOCKIFY_VERSION}/dockify-linux-amd64" -o "$TMP_DIR/dockify"
sudo mv "$TMP_DIR/dockify" "$INSTALL_DIR/dockify"
sudo chmod +x "$INSTALL_DIR/dockify"
rm -rf "$TMP_DIR"

echo "[4/5] Testing binary..."
"$INSTALL_DIR/dockify" version

echo "[5/5] Creating systemd service..."
sudo tee /etc/systemd/system/dockify.service > /dev/null << 'SYSTEMD'
[Unit]
Description=Dockify - Self-hosted Docker app deployment platform
After=network.target

[Service]
Type=simple
User=SERVICE_USER_PLACEHOLDER
ExecStart=/usr/local/bin/dockify serve
Restart=on-failure
RestartSec=10
EnvironmentFile=/etc/dockify/dockify.env

[Install]
WantedBy=multi-user.target
SYSTEMD

sudo sed -i "s/SERVICE_USER_PLACEHOLDER/$SERVICE_USER/" /etc/systemd/system/dockify.service

if [ ! -f /etc/dockify/dockify.env ]; then
    echo ""
    echo "Creating /etc/dockify/dockify.env with defaults..."
    sudo mkdir -p /etc/dockify
    sudo tee /etc/dockify/dockify.env > /dev/null << 'ENVFILE'
DOCKIFY_HOST=0.0.0.0
DOCKIFY_PORT=8080
DOCKIFY_DATA_DIR=/var/lib/dockify
DOCKIFY_SSH_KEY_DIR=/var/lib/dockify/keys
# CLOUDFLARE_API_TOKEN=
# CLOUDFLARE_ZONE_ID=
ENVFILE
fi

sudo systemctl daemon-reload
sudo systemctl enable dockify

echo ""
echo "=== Dockify installed successfully! ==="
echo ""
echo "Next steps:"
echo "  1. Edit /etc/dockify/dockify.env to configure your settings"
echo "  2. Set Cloudflare API token if you want DNS automation"
echo "  3. Start: sudo systemctl start dockify"
echo "  4. Open http://<your-ip>:8080"
echo ""
echo "Manually build from source:"
echo "  git clone https://github.com/coderbuzz/dockify.git"
echo "  cd dockify"
echo "  go build -o dockify ./cmd/dockify"
echo "  ./dockify serve"
