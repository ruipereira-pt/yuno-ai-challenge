#!/usr/bin/env bash
set -euo pipefail

PORT="${PORT:-18080}"
API_BASE_URL="http://localhost:${PORT}"
WS_DEMO_URL="ws://localhost:${PORT}/events/stream"

echo "== PSP Health Monitoring - Final Demo Script =="
echo "Using API base URL: ${API_BASE_URL}"
echo

echo "1) Verify code quality gates"
make verify
echo

echo "2) Generate deterministic dataset"
make generate-data
echo

echo "3) Start server with WS ingest enabled"
WS_INGEST_ENABLED=true HTTP_ADDR=":${PORT}" go run ./cmd/server > /tmp/psp-demo-server.log 2>&1 &
SERVER_PID=$!
trap 'kill "${SERVER_PID}" >/dev/null 2>&1 || true' EXIT

for _ in $(seq 1 20); do
  if curl -fsS "${API_BASE_URL}/status" >/dev/null; then
    break
  fi
  sleep 0.5
done

echo "Server started (pid=${SERVER_PID})."
echo

echo "4) Ingest historical dataset"
curl -sS -X POST "${API_BASE_URL}/events/batch" \
  -H "Content-Type: application/json" \
  --data-binary @testdata/transactions.json
echo
echo

echo "5) Core API walkthrough"
echo "GET /health"
curl -sS "${API_BASE_URL}/health"
echo
echo

echo "GET /alerts?active_only=true"
curl -sS "${API_BASE_URL}/alerts?active_only=true"
echo
echo

echo "GET /comparison"
curl -sS "${API_BASE_URL}/comparison"
echo
echo

echo "6) WS single-event demo"
make ws-demo WS_DEMO_URL="${WS_DEMO_URL}"
echo

echo "7) WS degraded-now demo (creates active incident fast)"
make demo-degraded-now WS_DEMO_URL="${WS_DEMO_URL}" API_BASE_URL="${API_BASE_URL}"
echo

echo "8) Docs endpoints"
echo "Swagger UI: ${API_BASE_URL}/docs"
echo "OpenAPI:    ${API_BASE_URL}/openapi.yaml"
echo

echo "Demo complete."
echo "Server logs: /tmp/psp-demo-server.log"
