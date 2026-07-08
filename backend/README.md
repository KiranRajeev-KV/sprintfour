# Backend

Go API server for Redactlane.

The backend is intentionally small:

- runtime state is in memory
- uploads are plain-text `.txt` files only
- documents are processed asynchronously by a bounded worker pool
- export writes redacted `.txt` files into the repo-level `exported/` directory
- the server starts empty; dataset seeding is used in tests, not at runtime

## Run

From the repo root:

```bash
cd backend/cmd/server
go run .
```

Default address:

```text
http://127.0.0.1:8080
```

The backend loads `backend/.env` automatically when present. Explicit process environment variables still win.

## Environment

Primary knobs:

- `HTTP_ADDR` default `:8080`
- `WORKER_COUNT` default `8`
- `QUEUE_DEPTH` default `200`
- `GLINER_ENABLED` default `false`
- `GLINER_URL` default `http://127.0.0.1:8090`
- `GLINER_TIMEOUT_MS` default `2500`
- `GLINER_MAX_CONCURRENCY` default `1`

`GLINER_MAX_CONCURRENCY` should stay low on one machine. The backend uses it to limit concurrent sidecar inference requests.

## Runtime Model

- `POST /api/uploads/documents` creates a new batch in `replace` mode or appends to the current one in `append` mode.
- Uploaded documents start in `QUEUED`.
- Workers move them through `PROCESSING` and then to `READY`, `NEEDS_REVIEW`, or `CLEAN`.
- Detection failures move a document to `FAILED`.
- Approved documents move to `APPROVED`.
- Exported documents move to `EXPORTED`.

Document statuses:

- `QUEUED`
- `PROCESSING`
- `READY`
- `NEEDS_REVIEW`
- `CLEAN`
- `FAILED`
- `APPROVED`
- `EXPORTED`

Redaction review states:

- `PENDING`
- `ACCEPTED`
- `REJECTED`
- `ADDED`

## Detection Behavior

Default processing is regex/rule based.

Regex-owned labels include structured identifiers such as:

- `CASE_ID`
- `CLIENT_ID`
- `BANK_ACCOUNT`
- `ROUTING_NUMBER`
- `SSN`
- `EIN`
- `ITIN`
- `CREDIT_CARD`
- `IBAN`
- `SWIFT_BIC`
- `AADHAAR`
- `PASSPORT`
- `US_DRIVER_LICENSE`
- `NPI`
- `DEA`
- `MEDICAL_LICENSE`
- `API_KEY`
- `DOB`

If `GLINER_ENABLED=true`, the backend calls the local sidecar for semantic labels:

- `PERSON`
- `ADDRESS`
- `EMAIL`
- `PHONE`

If the sidecar fails, the backend logs the failure and falls back to regex-only detection for that document.

## API

Health and batch summary:

- `GET /healthz`
- `GET /api/batch/summary`

Document reads:

- `GET /api/documents`
- `GET /api/documents/:id`
- `GET /api/documents/:id/redactions`
- `GET /api/documents/:id/review-summary`

Uploads:

- `POST /api/uploads/documents`

Redaction review:

- `POST /api/documents/:id/redactions`
- `POST /api/redactions/:id/accept`
- `POST /api/redactions/:id/reject`
- `POST /api/redactions/bulk-accept`
- `POST /api/redactions/bulk-reject`

Document actions:

- `POST /api/documents/:id/approve`
- `POST /api/documents/bulk-approve`
- `POST /api/documents/:id/retry`
- `POST /api/documents/bulk-retry`

Export:

- `POST /api/export`
- `GET /api/export/latest`

## Query and Mutation Notes

`GET /api/documents` supports:

- `status`
- `risk`
- `q`
- `limit`
- `offset`

Upload endpoint behavior:

- multipart field name for files is `files`
- `mode=replace` resets in-memory state before adding documents
- `mode=append` keeps the current batch
- non-`.txt` files are rejected per item
- empty files are rejected per item
- upload parsing is streamed part-by-part

Manual redaction rules:

- request body requires `start`, `end`, and `type`
- offsets are rune offsets, not byte offsets
- `selected_text`, when provided, must match the document span exactly
- failed documents cannot be edited until retried
- exported documents are locked in this MVP

Approval rules:

- `FAILED` documents must be retried before approval
- documents with blocking `PENDING` redactions cannot be approved
- bulk approve only changes `READY` and `CLEAN` documents
- approving an already approved or exported document is a no-op success

Retry rules:

- retry only changes `FAILED` documents
- bulk retry skips non-failed and missing documents

## Export Semantics

- only `APPROVED` documents with no blocking review items are exported
- only redactions in `ACCEPTED` or `ADDED` are applied
- `PENDING` and `REJECTED` redactions are skipped
- overlapping accepted spans are handled defensively
- output files are written to the repo root `exported/` directory
- filenames use the source filename plus `_redacted`
- repeated export calls are idempotent once approved documents have already been consumed

## Logging

The server uses `log/slog` with JSON output.

It logs:

- request method, path, status, duration, request id
- upload counts and byte totals
- worker job success and retry events
- sidecar fallback warnings
- export summary counts

It does not log:

- raw document text
- raw PII values

## Validation

Backend package checks:

```bash
cd backend
go test ./...
go vet ./...
```

Full API smoke test:

```bash
bash backend/scripts/api_smoke.sh
```

The smoke script starts the backend, uploads files from `dataset/raw/manual_synthetic_txt/`, exercises every HTTP endpoint, and validates the JSON responses with `jq`.
