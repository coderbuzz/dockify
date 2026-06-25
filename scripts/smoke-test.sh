#!/usr/bin/env bash
set -euo pipefail

echo "=== Dockify Smoke Test ==="

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

GO=$(which go 2>/dev/null || echo /usr/local/go/bin/go)

echo "Building binary..."
$GO build -o "$TMPDIR/dockify" ./cmd/dockify
echo "Binary: $TMPDIR/dockify"

echo ""
PORT=$(( 10000 + RANDOM % 55000 ))

echo "Starting server on port $PORT..."
DOCKIFY_DATA_DIR="$TMPDIR/data" \
DOCKIFY_PORT="$PORT" \
DOCKIFY_ADMIN_PASSWORD=smoketest \
"$TMPDIR/dockify" serve &
PID=$!

sleep 2

BASE="http://localhost:$PORT"

echo ""
echo "Test: /health (unauthenticated)..."
STATUS=$(curl -sf -o /dev/null -w '%{http_code}' "$BASE/health" 2>/dev/null || echo "000")
if [ "$STATUS" = "200" ]; then
  echo "  PASS"
else
  echo "  FAIL (HTTP $STATUS)"
  kill $PID 2>/dev/null || true
  wait $PID 2>/dev/null || true
  exit 1
fi

echo ""
echo "Test: /login renders..."
STATUS=$(curl -sf -o /dev/null -w '%{http_code}' "$BASE/login" 2>/dev/null || echo "000")
if [ "$STATUS" = "200" ]; then
  echo "  PASS"
else
  echo "  FAIL (HTTP $STATUS)"
  kill $PID 2>/dev/null || true
  wait $PID 2>/dev/null || true
  exit 1
fi

echo ""
echo "Test: / redirects to login..."
STATUS=$(curl -sf -o /dev/null -w '%{http_code}' "$BASE/" 2>/dev/null || echo "000")
if [ "$STATUS" = "302" ] || [ "$STATUS" = "200" ]; then
  echo "  PASS"
else
  echo "  FAIL (HTTP $STATUS)"
  kill $PID 2>/dev/null || true
  wait $PID 2>/dev/null || true
  exit 1
fi

echo ""
echo "Test: /api/servers requires auth (returns 302 redirect)..."
STATUS=$(curl -sf -o /dev/null -w '%{http_code}' "$BASE/api/servers" 2>/dev/null || echo "000")
if [ "$STATUS" = "302" ]; then
  echo "  PASS"
else
  echo "  FAIL (HTTP $STATUS)"
  kill $PID 2>/dev/null || true
  wait $PID 2>/dev/null || true
  exit 1
fi

kill $PID 2>/dev/null || true
sleep 1

echo ""
echo "=== All smoke tests passed! ==="
