#!/usr/bin/env bash
set -euo pipefail

echo "=== Dockify Worker Setup ==="
echo "Run this script on the worker VM to prepare it for Dockify."
echo ""

# Install Docker
if command -v docker &>/dev/null; then
  echo "[OK] Docker already installed: $(docker --version)"
else
  echo "[INFO] Installing Docker..."
  curl -fsSL https://get.docker.com | sh
  echo "[OK] Docker installed"
fi

# Install docker compose plugin
if docker compose version &>/dev/null 2>&1; then
  echo "[OK] Docker Compose plugin already installed: $(docker compose version 2>/dev/null | head -1)"
else
  echo "[INFO] Installing Docker Compose plugin..."
  sudo mkdir -p /usr/local/lib/docker/cli-plugins
  sudo curl -fsSL "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" \
    -o /usr/local/lib/docker/cli-plugins/docker-compose
  sudo chmod +x /usr/local/lib/docker/cli-plugins/docker-compose
  echo "[OK] Docker Compose plugin installed"
fi

# Start Docker service
if command -v systemctl &>/dev/null; then
  sudo systemctl enable docker 2>/dev/null || true
  sudo systemctl start docker 2>/dev/null || true
fi

# Generate SSH key for Dockify
KEY_PATH="$HOME/.ssh/dockify"
if [ -f "$KEY_PATH" ]; then
  echo "[SKIP] SSH key already exists at $KEY_PATH"
else
  mkdir -p "$HOME/.ssh"
  chmod 700 "$HOME/.ssh"
  ssh-keygen -t ed25519 -f "$KEY_PATH" -N "" -C "dockify@$(hostname)" -q
  echo "[OK] SSH key generated at $KEY_PATH"
fi

# Authorize itself (public key -> authorized_keys)
PUBKEY=$(cat "$KEY_PATH.pub")
if [ -f "$HOME/.ssh/authorized_keys" ] && grep -qF "$PUBKEY" "$HOME/.ssh/authorized_keys" 2>/dev/null; then
  echo "[SKIP] Public key already in authorized_keys"
else
  echo "$PUBKEY" >> "$HOME/.ssh/authorized_keys"
  chmod 600 "$HOME/.ssh/authorized_keys"
  echo "[OK] Public key added to authorized_keys"
fi

echo ""
echo "============================================"
echo "  Worker setup complete"
echo "============================================"
echo ""
echo "Worker IP:    $(curl -fsSL ifconfig.me 2>/dev/null || hostname -I | awk '{print $1}')"
echo ""
echo "Copy the private key below (including the -----BEGIN/END lines)"
echo "and paste it into the Dockify Add Server form:"
echo ""
cat "$KEY_PATH"
echo ""
echo "Then in Dockify UI -> Servers -> Add Server, fill:"
echo "  Name:       <any label, e.g. worker-01>"
echo "  Host:       $(curl -fsSL ifconfig.me 2>/dev/null || hostname -I | awk '{print $1}')"
echo "  User:       ${USER:-root}"
echo "  SSH Key:    <paste the private key above>"
echo ""
echo "After adding, click 'Initialize Worker' to install Caddy."

# Add active non-root user to docker group and setup /opt/dockify permissions
ACTIVE_USER="${SUDO_USER:-$USER}"
if [ "$ACTIVE_USER" != "root" ]; then
  echo ""
  echo "[INFO] Adding $ACTIVE_USER to docker group..."
  sudo usermod -aG docker "$ACTIVE_USER"
  echo "[OK] User $ACTIVE_USER added to docker group"
fi

echo ""
echo "[INFO] Preparing /opt/dockify directory..."
sudo mkdir -p /opt/dockify/apps /opt/dockify/caddy
sudo chown -R "$ACTIVE_USER":"$ACTIVE_USER" /opt/dockify
echo "[OK] /opt/dockify prepared with ownership for $ACTIVE_USER"


