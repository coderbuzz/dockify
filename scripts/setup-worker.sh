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
  mkdir -p /usr/local/lib/docker/cli-plugins
  curl -fsSL "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" \
    -o /usr/local/lib/docker/cli-plugins/docker-compose
  chmod +x /usr/local/lib/docker/cli-plugins/docker-compose
  echo "[OK] Docker Compose plugin installed"
fi

# Start Docker service
if command -v systemctl &>/dev/null; then
  systemctl enable docker 2>/dev/null || true
  systemctl start docker 2>/dev/null || true
fi

# Generate SSH key for Dockify
KEY_PATH="/root/.ssh/dockify"
if [ -f "$KEY_PATH" ]; then
  echo "[SKIP] SSH key already exists at $KEY_PATH"
else
  mkdir -p /root/.ssh
  chmod 700 /root/.ssh
  ssh-keygen -t ed25519 -f "$KEY_PATH" -N "" -C "dockify@$(hostname)" -q
  echo "[OK] SSH key generated at $KEY_PATH"
fi

# Authorize itself (public key -> authorized_keys)
PUBKEY=$(cat "$KEY_PATH.pub")
if [ -f /root/.ssh/authorized_keys ] && grep -qF "$PUBKEY" /root/.ssh/authorized_keys 2>/dev/null; then
  echo "[SKIP] Public key already in authorized_keys"
else
  echo "$PUBKEY" >> /root/.ssh/authorized_keys
  chmod 600 /root/.ssh/authorized_keys
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
echo "  User:       root"
echo "  SSH Key:    <paste the private key above>"
echo ""
echo "After adding, click 'Initialize Worker' to install Caddy."
