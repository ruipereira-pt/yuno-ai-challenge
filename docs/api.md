# API Reference

Production-facing HTTP API for ingesting PSP transaction events and querying real-time health, incidents, and ranking.

Base URL (local): `http://localhost:8080`

## Endpoints

- `GET /status` - liveness probe.
- `POST /events/batch` - ingest and validate transaction events with deduplication.
- `GET /events/stream` - optional WebSocket stream ingest (disabled by default).
- `GET /health` - health snapshot for all PSPs or one PSP.
- `GET /psps` - alias of `GET /health` (same behavior and payload).
- `GET /alerts` - degradation incidents over a time range.
- `GET /comparison` - PSP ranking over a time range.
- `GET /docs` - Swagger UI.
- `GET /openapi.yaml` - raw OpenAPI 3.0 spec.

---

## Common Behavior

### Time range query contract (`from`, `to`)
Used by `GET /alerts` and `GET /comparison`.

- If both are omitted: defaults to last 60 minutes (`now-60m` to `now`).
- If one is missing: `400 invalid_range_format`.
- If either is not RFC3339: `400 invalid_range_format`.
- If `from >= to`: `422 invalid_range_bounds`.
- All computations use UTC internally.

### Window semantics
All event-time aggregations use **half-open intervals**: `[start, end)`.

An event is included iff:

- `event.timestamp >= start`
- `event.timestamp < end`

---

## Health Score

Health score is computed from a window's aggregate metrics:

- `approvalScore = clamp(approvalRate * 100, 0, 100)`
- `errorScore = clamp((1 - errorRate) * 100, 0, 100)`
- `latencyScore = 100` when `avg_response_time_ms <= 200`
- otherwise `latencyScore = clamp(100 - ((avg_response_time_ms - 200) / 2800) * 100, 0, 100)`
- `health = 0.60*approvalScore + 0.25*errorScore + 0.15*latencyScore`
- rounded to 1 decimal place.

---

## `POST /events/batch`

Ingests `events[]` with validation, dedupe, and per-row rejection reporting.

### Request body
```json
{
  "events": [
    {
      "transaction_id": "tx-1001",
      "psp": "PSP_ALPHA",
      "status": "approved",
      "response_time_ms": 180,
      "timestamp": "2026-05-14T09:30:00Z"
    }
  ]
}
```

### Validation and ingestion rules

- `transaction_id`, `psp`, and `timestamp` are required.
- `status` must be one of: `approved`, `declined`, `error`.
- `response_time_ms` must be `>= 0`.
- Request body is capped at `5MB`; larger payloads return `413 payload_too_large`.
- Event too old (older than `MAX_EVENT_AGE`, default `180m`) is rejected.
- Event too far in future (beyond `MAX_FUTURE_SKEW`, default `2m`) is rejected.
- Deduplication is global in process memory by `transaction_id`:
  - duplicates already ingested or repeated in same batch are counted as duplicates.
- Accepted events are stored; rejected events are returned in `errors[]` with index and reason code.

### Response (200)
```json
{
  "accepted_count": 1,
  "duplicate_count": 0,
  "rejected_count": 0,
  "errors": []
}
```

---

## `GET /events/stream` (Optional Stretch)

WebSocket endpoint for streaming ingestion with micro-batching into the same core ingestion path used by `POST /events/batch`.

Feature flag:
- `WS_INGEST_ENABLED=true` to enable.

Behavior:
- Accepts either a single event JSON object or `{ "events": [...] }`.
- Incoming events are enqueued in a bounded in-memory queue.
- Server periodically flushes queued events (configurable size/interval) through `IngestBatch`.
- Client receives batch-style ack payloads with:
  - `accepted_count`
  - `duplicate_count`
  - `rejected_count`
  - `errors[]`

Error behavior:
- Invalid payload -> ack with `rejected_count=1`, `code=invalid_request`.
- Queue overflow -> ack with `code=queue_full` for rejected events.
- If endpoint is disabled -> HTTP `404 not_found`.

---

## `GET /health` and `GET /psps`

Optional query:
- `psp=<name>` (filter to one PSP).

### Response shape

```json
{
  "generated_at": "2026-05-14T10:00:00Z",
  "psps": [
    {
      "psp": "PSP_ALPHA",
      "health_score": 92.4,
      "no_data": false,
      "degraded": false,
      "windows": {
        "5m": { "...": "metrics" },
        "15m": { "...": "metrics" },
        "60m": { "...": "metrics" }
      }
    }
  ]
}
```

### Semantics

- Unknown PSP filter returns `200` with `psps: []`.
- `no_data=true` only when there is no data in the last 60m for that PSP.
- If 60m exists but 5m has zero events:
  - `no_data=false`
  - `health_score=null`
  - `5m` is omitted from `windows` (same for `15m` if empty)
  - `60m` remains present.

---

## `GET /alerts`

Query params:

- `psp` (optional)
- `from`, `to` (optional; see common range contract)
- `active_only` (optional boolean)

### `active_only` parsing

- Parsed strictly as a boolean query value.
- Invalid value returns `400 invalid_request` with `details.field=active_only`.

### Alert lifecycle semantics

- Evaluated on **minute-sampled rolling 5-minute windows**.
- Degraded sample when any condition is true:
  - score `< health threshold`
  - approval rate `< approval threshold`
  - error rate `> error threshold`
- Incident opens only after **5 consecutive degraded minute samples** (default config).
- No-data samples are **unknown**:
  - do not advance open streak,
  - do not auto-open incidents,
  - do not auto-close active incidents.
- Incident closes on the first healthy sample after being active.
- Opening reason precedence:
  1. `health`
  2. `approval`
  3. `error`
- Reason is fixed when the incident opens.
- Result ordering: newest `started_at` first; tie-break by `psp` ascending.

---

## `GET /comparison`

Query params:
- `from`, `to` (optional; see common range contract)

Returns ranking rows only for PSPs with data in the selected range.

### Ranking sort order

1. `health_score` descending
2. `approval_rate` descending
3. `avg_response_time_ms` ascending
4. `psp` ascending (deterministic tie-break)

---

## Error Envelope

All error responses use:

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

Typical codes:
- `invalid_request`
- `invalid_range_format`
- `invalid_range_bounds`
- `method_not_allowed`
- `not_found`
