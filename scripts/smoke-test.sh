#!/usr/bin/env sh
set -eu
BASE_URL="${BASE_URL:-http://localhost:8080}"
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
PROJECT_DIR=$(dirname "$SCRIPT_DIR")
curl -fsS "$BASE_URL/api/v1/health"
printf '\n'
curl -fsS "$BASE_URL/metrics" | grep -q 'seal_http_requests_total'
curl -fsS -X POST "$BASE_URL/api/v1/seals/render" \
  -H 'Content-Type: application/json' \
  --data-binary "@$PROJECT_DIR/testdata/seal-config-v2.json" \
  > /tmp/seal-smoke.svg
printf 'created /tmp/seal-smoke.svg\n'
grep -q '<svg' /tmp/seal-smoke.svg
printf 'health, metrics and SVG render smoke tests passed\n'
