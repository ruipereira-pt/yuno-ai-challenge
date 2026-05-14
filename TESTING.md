# Testing Checklist

Use this checklist to validate the project locally in one pass before submission.

## Prerequisites

- Go 1.22+ installed and available in PATH (`go version`)
- Project root:
  - `cd /Users/messontour/Projects/yuno-ai-challenge`

## 1) Format and Static Hygiene

```bash
make fmt
```

Pass criteria:
- Command exits with code 0.
- No unexpected file changes after formatting.

Or run the full gate in one command:

```bash
make verify
```

## 2) Unit + Contract + Integration Tests

```bash
make test
```

Pass criteria:
- Command exits with code 0.
- Includes passing tests for:
  - ingest validation and dedupe precedence
  - range parsing (`400` / `422`)
  - scoring logic
  - alert lifecycle transitions
  - API contract/integration flow

## 3) Race Detector (Concurrency Safety)

```bash
make test-race
```

Pass criteria:
- Command exits with code 0.
- No race warnings (`WARNING: DATA RACE` must not appear).

## 4) Generate Deterministic Dataset

```bash
make generate-data
```

Pass criteria:
- Command exits with code 0.
- `testdata/transactions.json` exists.
- File contains a JSON object with `events` array.

## 5) Run Service

```bash
make run
```

Pass criteria:
- Service starts successfully.
- Log contains startup line similar to:
  - `starting psp health monitoring service on :8080`

## 6) Manual API Smoke Test (new terminal)

### 6.1 Ingest dataset

```bash
curl -sS -X POST "http://localhost:8080/events/batch" \
  -H "Content-Type: application/json" \
  --data-binary @testdata/transactions.json
```

Pass criteria:
- HTTP `200`.
- Response includes:
  - `accepted_count`
  - `duplicate_count`
  - `rejected_count`
  - `errors`

### 6.2 Health snapshot

```bash
curl -sS "http://localhost:8080/health"
```

Pass criteria:
- HTTP `200`.
- Response includes `generated_at` and non-empty `psps` array.

### 6.3 Alerts query

```bash
curl -sS "http://localhost:8080/alerts"
```

Pass criteria:
- HTTP `200`.
- Response includes `events` array (may be empty depending on data state).

### 6.4 Comparison query

```bash
curl -sS "http://localhost:8080/comparison"
```

Pass criteria:
- HTTP `200`.
- Response includes `range` and `ranking`.

### 6.5 Error contract checks

Invalid format (`400` expected):

```bash
curl -i "http://localhost:8080/comparison?from=bad&to=2026-05-14T10:00:00Z"
```

Invalid bounds (`422` expected):

```bash
curl -i "http://localhost:8080/comparison?from=2026-05-14T10:00:00Z&to=2026-05-14T10:00:00Z"
```

Pass criteria:
- Status codes are `400` and `422` respectively.
- Body follows:
  - `{ "error": { "code": "...", "message": "...", "details": {...} } }`

## 7) Final Submission Gate

Mark as ready only if all are true:
- [ ] `make test` passes
- [ ] `make test-race` passes
- [ ] dataset generated successfully
- [ ] manual API smoke tests pass
- [ ] OpenAPI file exists at `docs/openapi.yaml`
- [ ] README is up to date with endpoints and run instructions
