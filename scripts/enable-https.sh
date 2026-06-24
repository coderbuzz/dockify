#!/usr/bin/env bash
set -e
echo "=== Enabling HTTPS on Caddy ==="
docker exec caddy curl -s -X PUT http://localhost:2019/config/apps/http/servers/srv0/listen \
  -H 'Content-Type: application/json' \
  -d '[":80",":443"]'
echo ""
echo "Done. HTTPS enabled on port 443."
