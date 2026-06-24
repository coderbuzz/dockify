#!/usr/bin/env bash
set -e
[ -z "$1" ] && echo "Usage: $0 <worker-ip>" && exit 1
echo "=== Enabling HTTPS on worker Caddy ==="
ssh -o StrictHostKeyChecking=no "root@$1" 'docker exec caddy curl -s -X PUT http://localhost:2019/config/apps/http/servers/srv0/listen \
  -H "Content-Type: application/json" \
  -d "[\":80\",\":443\"]"'
echo ""
echo "Done. HTTPS enabled on port 443."
