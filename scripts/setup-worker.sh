#!/usr/bin/env bash
set -euo pipefail

echo "=== Dockify Worker Setup ==="
echo "Run this script on the worker VM to prepare it for Dockify."
echo ""

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

# Ensure Docker is installed (optional, Dockify init will do this too)
if command -v docker &>/dev/null; then
  echo "[OK] Docker already installed"
else
  echo "[INFO] Docker not installed. Dockify will install it automatically on init."
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
echo "After adding, click 'Initialize Worker' to install Docker + Caddy."
