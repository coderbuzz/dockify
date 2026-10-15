#!/usr/bin/env bash
set -euo pipefail

# Prepare a worker VM for Dockify. Run it as the user Dockify will connect as,
# with the public key shown on the server's page in Dockify:
#   curl -fsSL https://raw.githubusercontent.com/coderbuzz/dockify/main/scripts/setup-worker.sh | bash -s -- 'ssh-ed25519 AAAA... dockify'

PUBKEY="${1:-}"
case "$PUBKEY" in
  ssh-*|ecdsa-*) ;;
  *)
    echo "Usage: setup-worker.sh '<public key from the Dockify server page>'" >&2
    echo "  e.g. setup-worker.sh 'ssh-ed25519 AAAA... dockify'" >&2
    exit 1
    ;;
esac

echo "=== Dockify Worker Setup ==="
echo ""

# Authorize the Dockify controller key
mkdir -p "$HOME/.ssh"
chmod 700 "$HOME/.ssh"
touch "$HOME/.ssh/authorized_keys"
chmod 600 "$HOME/.ssh/authorized_keys"
if grep -qxF "$PUBKEY" "$HOME/.ssh/authorized_keys"; then
  echo "[SKIP] Public key already in authorized_keys"
else
  echo "$PUBKEY" >> "$HOME/.ssh/authorized_keys"
  echo "[OK] Public key added to authorized_keys"
fi

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
# Not recursive: app bind-mount data under apps/ keeps its container-owned uid.
sudo chown "$ACTIVE_USER":"$ACTIVE_USER" /opt/dockify /opt/dockify/apps
sudo chown -R "$ACTIVE_USER":"$ACTIVE_USER" /opt/dockify/caddy
echo "[OK] /opt/dockify prepared with ownership for $ACTIVE_USER"

echo ""
echo "============================================"
echo "  Worker setup complete"
echo "============================================"
echo ""
echo "Worker IP:    $(curl -fsSL ifconfig.me 2>/dev/null || hostname -I | awk '{print $1}')"
echo "Worker user:  $(id -un)"
echo ""
echo "Check that the server in Dockify uses this host and user, then click"
echo "'Init Worker' on its page to install Caddy."
