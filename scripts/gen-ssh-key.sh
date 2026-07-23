#!/usr/bin/env bash
set -euo pipefail

echo "=== Remote Access SSH Key Generator ==="
echo "Generates a dedicated key for external/agentic-AI SSH access to this VM."
echo ""

KEY_PATH="/root/.ssh/remote-access"
if [ -f "$KEY_PATH" ]; then
  echo "[SKIP] Remote key already exists at $KEY_PATH"
else
  mkdir -p /root/.ssh
  chmod 700 /root/.ssh
  ssh-keygen -t ed25519 -f "$KEY_PATH" -N "" -C "remote-access@$(hostname)" -q
  echo "[OK] Remote key generated at $KEY_PATH"
fi

PUBKEY=$(cat "$KEY_PATH.pub")
if [ -f /root/.ssh/authorized_keys ] && grep -qF "$PUBKEY" /root/.ssh/authorized_keys 2>/dev/null; then
  echo "[SKIP] Public key already in authorized_keys"
else
  echo "$PUBKEY" >> /root/.ssh/authorized_keys
  chmod 600 /root/.ssh/authorized_keys
  echo "[OK] Public key added to authorized_keys"
fi

KEX_ALGOS="sntrup761x25519-sha512@openssh.com,curve25519-sha256,curve25519-sha256@libssh.org,diffie-hellman-group16-sha512,diffie-hellman-group14-sha256"
SSHD_CONFIG="/etc/ssh/sshd_config"
if [ -f "$SSHD_CONFIG" ]; then
  if grep -qE '^\s*KexAlgorithms\s+' "$SSHD_CONFIG"; then
    echo "[SKIP] KexAlgorithms already configured in $SSHD_CONFIG"
  else
    cp "$SSHD_CONFIG" "${SSHD_CONFIG}.bak"
    echo "KexAlgorithms $KEX_ALGOS" >> "$SSHD_CONFIG"
    echo "[OK] Added post-quantum KexAlgorithms to $SSHD_CONFIG"
    if command -v systemctl >/dev/null 2>&1; then
      systemctl restart sshd 2>/dev/null || systemctl restart ssh 2>/dev/null || true
    else
      service sshd restart 2>/dev/null || service ssh restart 2>/dev/null || true
    fi
    echo "[OK] sshd restarted"
  fi
fi

HOST="$(curl -fsSL ifconfig.me 2>/dev/null || hostname -I | awk '{print $1}')"
echo ""
echo "============================================"
echo "  Remote access ready"
echo "============================================"
echo "Host:  $HOST"
echo "User:  root"
echo ""
echo "Private key (paste into your agentic-AI SSH config):"
echo ""
cat "$KEY_PATH"
echo ""
echo "Example connection:"
echo "  ssh -i remote-access root@$HOST"
