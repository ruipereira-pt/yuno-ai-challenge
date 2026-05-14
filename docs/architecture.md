# Architecture

Production-oriented, challenge-scoped backend for event ingestion, rolling health computation, incident detection, and PSP ranking.

## 1) Components

- **HTTP API layer (`internal/api`)**
  - Routes: `/status`, `/events/batch`, `/events/stream` (optional WS), `/health`, `/psps`, `/alerts`, `/comparison`, `/docs`, `/openapi.yaml`.
  - Validates method, query contracts, and emits uniform error envelope.

- **Domain/service layer (`internal/service`)**
  - `HealthService`: ingestion orchestration, health projection, comparison ranking.
  - `Scorer`: deterministic health score calculation.
  - `AlertEvaluator`: minute-sampled rolling-window incident lifecycle.

- **State/store layer (`internal/store`)**
  - `WindowStore`: in-memory source of truth.
  - Holds events by PSP, dedupe set, computed incidents, active incident flags.
  - Provides aggregations and alert retrieval.

- **Configuration (`internal/config`)**
  - Environment-driven thresholds and temporal bounds (`MAX_EVENT_AGE`, `MAX_FUTURE_SKEW`, alert thresholds/duration).

- **Entrypoint (`cmd/server`)**
  - Wires dependencies and serves HTTP.

---

## 2) Data Flow

1. **Ingest (`POST /events/batch`)**
   - API decodes JSON -> service normalizes -> store classifies duplicates/rejections/accepted.
   - Accepted events append to in-memory PSP streams.
   - Old events are pruned.
   - Alerts are recomputed against current state.

1b. **Streaming ingest (`GET /events/stream`, optional stretch)**
   - Feature-flagged WebSocket endpoint (`WS_INGEST_ENABLED`).
   - Reads single-event or batch payloads into a bounded queue.
   - Flushes micro-batches (size/interval) into the same `HealthService.IngestBatch` path as REST ingest.
   - Sends `BatchIngestResponse`-style ack payloads over the socket.

2. **Health query (`GET /health` / `/psps`)**
   - Service aggregates `[now-5m, now)`, `[now-15m, now)`, `[now-60m, now)`.
   - Computes score from 5m only when 5m has data.
   - Emits `no_data`, `health_score`, `degraded`, and available windows.

3. **Alerts query (`GET /alerts`)**
   - API parses range + `active_only`.
   - Store filters incidents by overlap with `[from,to)` and active flag.
   - Returns deterministic ordering.

4. **Comparison query (`GET /comparison`)**
   - Service aggregates each PSP over `[from,to)`.
   - Computes score and sorts ranking deterministically.

---

## 3) Concurrency Model

- Shared mutable state is centralized in `WindowStore`.
- `sync.RWMutex` guards all store maps/slices:
  - write lock for ingest/prune/incident updates,
  - read lock for queries/aggregations.
- Alert recomputation uses snapshot-style reads + versioned writes:
  - recompute from a bucket snapshot,
  - `SetIncidents` ignores stale versions to avoid out-of-order overwrite.
- This design favors correctness/determinism over parallel write throughput.

---

## 4) Core Contracts

- **Event-time semantics:** all aggregations use `[start,end)` boundaries.
- **Range parsing contract:**
  - invalid format -> `400`,
  - `from >= to` -> `422`.
- **Health no-data contract:**
  - no 60m data -> `no_data=true`, `health_score=null`, `windows={}`.
  - 60m present but 5m absent -> `no_data=false`, `health_score=null`, only non-empty windows included.
- **Alert contract:**
  - rolling 5m sampled every minute,
  - sustained degradation required for open (default 5 consecutive samples),
  - no-data treated as unknown (no auto-open/close),
  - reason precedence `health > approval > error`,
  - at most one active incident per PSP.
- **Comparison contract:** ranking sorted by score desc, approval desc, latency asc, psp asc.

---

## 5) Trade-offs

- **In-memory state (chosen)**
  - Pros: simple, fast local reads, low challenge complexity.
  - Cons: process-restart data loss; dedupe/alerts not durable.

- **Full alert recomputation on ingest (chosen)**
  - Pros: deterministic and easy to reason about.
  - Cons: recomputation cost grows with history/PSP count.

- **Single-process lock-based consistency (chosen)**
  - Pros: strong local consistency and straightforward correctness.
  - Cons: write contention at higher ingest rates.

---

## 6) Known Limitations

- No persistence/snapshot recovery.
- No horizontal scaling or partitioned ownership model.
- No exactly-once guarantees across restarts.
- No background compaction beyond age pruning.
- No authn/authz, rate limiting, or multi-tenant isolation.
- Limited operational telemetry (no built-in Prometheus/OTel in current implementation).

---

## 7) Extension Path

### Near-term hardening
- Add persistent event/incident storage (e.g., Postgres/ClickHouse) and startup rebuild.
- Add idempotency with durable dedupe index (TTL/partitioned).
- Expose metrics/traces and SLO-oriented dashboards.

### Scale path
- Move from full recompute to incremental minute-bucket alert evaluation.
- Partition by PSP across workers/shards.
- Introduce queue-based ingestion with backpressure and retries.

### Reliability path
- Add snapshot + WAL/replay strategy.
- Define explicit recovery semantics for active incidents after restart.
- Add chaos/load tests for boundary minutes and sustained degradation transitions.

### Product path
- Add per-PSP configurable thresholds.
- Add incident annotations and audit trail.
- Add historical trend endpoints backed by persistent rollups.
