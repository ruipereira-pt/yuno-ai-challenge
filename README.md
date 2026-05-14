# PSP Health Monitoring Service

Go backend for real-time PSP health monitoring with rolling metrics, degradation incidents, and PSP comparison/ranking.

This repository is optimized for reviewer experience: one-command verification, deterministic test data, clear API contracts, and a direct demo flow.

## Reviewer Quick Start (3-5 minutes)

From repo root:

```bash
make verify
make generate-data
make run
```

Then in a second terminal:

```bash
curl -sS -X POST "http://localhost:8080/events/batch" \
  -H "Content-Type: application/json" \
  --data-binary @testdata/transactions.json

curl -sS "http://localhost:8080/health"
curl -sS "http://localhost:8080/alerts?active_only=true"
curl -sS "http://localhost:8080/comparison"
```

Open API docs in browser:

- Swagger UI: `http://localhost:8080/docs`
- OpenAPI spec: `http://localhost:8080/openapi.yaml`

---

## What Is Implemented

- Transaction ingestion (`POST /events/batch`) with:
  - validation
  - global dedupe by `transaction_id`
  - per-row rejection details
  - atomic batch application for valid events
- Rolling metrics per PSP over:
  - `5m`
  - `15m`
  - `60m`
- Deterministic health score calculation (0-100)
- Degradation incident lifecycle with configurable thresholds
- Comparison/ranking endpoint for routing decisions
- Comprehensive test suite including race checks
- Optional Next.js dashboard (stretch goal)

---

## Run

```bash
make run
```

Default backend address: `:8080`

### Environment Variables

- `HTTP_ADDR` (default `:8080`)
- `MAX_EVENT_AGE` (default `180m`)
- `MAX_FUTURE_SKEW` (default `2m`)
- `ALERT_HEALTH_THRESHOLD` (default `60`)
- `ALERT_APPROVAL_LT` (default `0.70`)
- `ALERT_ERROR_GT` (default `0.15`)
- `ALERT_MIN_DURATION` (default `5m`)
- `WS_INGEST_ENABLED` (default `false`)
- `WS_MAX_FRAME_BYTES` (default `1048576`)
- `WS_READ_TIMEOUT` (default `30s`)
- `WS_BATCH_SIZE` (default `50`)
- `WS_FLUSH_INTERVAL` (default `1s`)
- `WS_QUEUE_SIZE` (default `1000`)

Runtime config is env-driven.  
`configs/local.yaml` is illustrative and not loaded at runtime.

---

## Verification Commands

- Full gate (format + tests + race):
  ```bash
  make verify
  ```
- Deterministic dataset generation:
  ```bash
  make generate-data
  ```
- Detailed checklist:
  - `TESTING.md`

---

## API Summary

- `GET /status` - liveness probe
- `POST /events/batch` - ingest events
- `GET /events/stream` - optional WebSocket streaming ingest (feature-flagged)
- `GET /health` - health snapshot
- `GET /psps` - alias of `/health`
- `GET /alerts` - degradation incidents
- `GET /comparison` - performance ranking
- `GET /docs` - Swagger UI
- `GET /openapi.yaml` - OpenAPI document

Detailed endpoint contracts and examples:
- `docs/api.md`
- `docs/openapi.yaml`

---

## Health Score Methodology

For each window:

- `approvalScore = approvalRate * 100`
- `errorScore = (1 - errorRate) * 100`
- `latencyScore = clamp(100 - ((avgLatencyMs - 200) / 2800) * 100, 0, 100)`
- `health = 0.60*approvalScore + 0.25*errorScore + 0.15*latencyScore`

Score is rounded to one decimal.

### Time Window Semantics

Event-time windows use half-open intervals `[start, end)`:

- include when `timestamp >= start`
- exclude when `timestamp >= end`

---

## Core Contracts (Important)

### Ingest

- `transaction_id` dedupe is global in process memory.
- Duplicates are counted and ignored.
- Too-old events are rejected (`MAX_EVENT_AGE`).
- Too-future events are rejected (`MAX_FUTURE_SKEW`).
- Ingest request bodies are capped at 5MB (`413 payload_too_large` beyond limit).
- Accepted/duplicate/rejected counts are always returned.

### Health no-data behavior

- No data in 60m:
  - `no_data=true`
  - `health_score=null`
  - `windows={}`
- If 60m has data but 5m has none:
  - `no_data=false`
  - `health_score=null`
  - empty window aggregates are omitted from `windows`

### Alerts

- Evaluated on minute-sampled rolling 5-minute windows.
- Opens only after 5 continuous degraded minute-samples.
- No-data samples are unknown:
  - no auto-open
  - no auto-close
- Reason precedence on open:
  - `health` > `approval` > `error`

### Comparison sorting

1. health score desc
2. approval rate desc
3. avg latency asc
4. PSP name asc

### Error envelope

All API errors use:

```json
{
  "error": {
    "code": "invalid_range_format",
    "message": "from must be RFC3339",
    "details": {
      "field": "from"
    }
  }
}
```

Range validation:
- invalid datetime format -> `400`
- `from >= to` -> `422`

---

## Deterministic Test Data

Generate:

```bash
make generate-data
```

Output file:
- `testdata/transactions.json`

Properties:
- 800 events
- ~3-hour timeline
- 3 PSPs: `PSP_ALPHA`, `PSP_BETA`, `PSP_GAMMA`
- includes a degradation/recovery scenario

---

## Demo Walkthrough (Screen Recording Ready)

1. Quality gate:
   ```bash
   make verify
   ```
2. Generate data:
   ```bash
   make generate-data
   ```
3. Start API:
   ```bash
   make run
   ```
4. Ingest dataset:
   ```bash
   curl -sS -X POST "http://localhost:8080/events/batch" \
     -H "Content-Type: application/json" \
     --data-binary @testdata/transactions.json
   ```
5. Show health:
   ```bash
   curl -sS "http://localhost:8080/health"
   ```
6. Show active incidents:
   ```bash
   curl -sS "http://localhost:8080/alerts?active_only=true"
   ```
7. Show ranking:
   ```bash
   curl -sS "http://localhost:8080/comparison"
   ```
8. Show docs:
   - `http://localhost:8080/docs`

---

## Optional Stretch: Next.js Dashboard

Frontend is in `frontend/`.

Run locally:

```bash
make run
# new terminal
make frontend-dev
```

Open:
- `http://localhost:3000`

By default it proxies to backend at `http://localhost:8080`.  
Override with `API_BASE_URL` in `frontend/.env.local` (see `frontend/.env.example`).

## Optional Stretch: WebSocket Streaming Ingest

Streaming ingest is optional and disabled by default.

Enable it:

```bash
export WS_INGEST_ENABLED=true
make run
```

WebSocket endpoint:

- `ws://localhost:8080/events/stream`

Payload formats accepted:

1) single event:

```json
{
  "transaction_id": "tx-ws-1",
  "psp": "PSP_ALPHA",
  "status": "approved",
  "response_time_ms": 210,
  "timestamp": "2026-05-14T10:00:00Z"
}
```

2) batch payload:

```json
{
  "events": [
    {
      "transaction_id": "tx-ws-2",
      "psp": "PSP_BETA",
      "status": "error",
      "response_time_ms": 1400,
      "timestamp": "2026-05-14T10:00:00Z"
    }
  ]
}
```

Server sends batch-style acknowledgements with:
- `accepted_count`
- `duplicate_count`
- `rejected_count`
- `errors[]`

Quick reviewer test without `wscat`:

```bash
export WS_INGEST_ENABLED=true
make run
# new terminal
make ws-demo
make ws-demo-degraded
make demo-degraded-now
```

Optional flags:

```bash
go run ./cmd/ws-demo -psp PSP_BETA -status error -response-ms 1800
go run ./cmd/ws-demo -batch
go run ./cmd/ws-demo -scenario degraded -psp PSP_GAMMA
```

One-command end-to-end recording flow:

```bash
./scripts/demo-final.sh
```

### Troubleshooting

If `make run` fails with `bind: address already in use`:

```bash
lsof -nP -iTCP:8080 -sTCP:LISTEN
kill <PID>
make run
```

If `make ws-demo` fails with `websocket: bad handshake`:

```bash
# ensure the server is started with WS enabled
WS_INGEST_ENABLED=true make run

# in another terminal
make ws-demo
```

Quick check:

```bash
curl -i http://localhost:8080/events/stream
```

- If you see `stream ingest endpoint is disabled`, restart the server with `WS_INGEST_ENABLED=true`.

---

## Assumptions and Trade-offs

- In-memory state is the source of truth for challenge scope.
- Dedupe lifetime is process lifetime.
- Alert recomputation is ingest-driven and version-guarded for consistency.
- Design prioritizes determinism, clarity, and reviewer reproducibility over persistence/scalability.

---

## Future Improvements

- Persistent storage / snapshot recovery
- Prometheus metrics + structured logging pipeline
- Incremental alert recomputation for higher throughput
- Auth/rate limits for production-facing ingest
