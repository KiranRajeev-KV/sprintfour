# Backend

## Run

From `backend/`:

```bash
go run .
```

The backend loads `backend/.env` automatically at startup when present. Explicit process environment variables still override file values.

Default local API:

```text
http://localhost:8080
```

Optional sidecar:

```bash
cd ../ner-sidecar
uv sync
env UV_CACHE_DIR=/tmp/uv-cache HF_HUB_DISABLE_XET=1 uv run uvicorn main:app --host 127.0.0.1 --port 8090
```

The sidecar loads `ner-sidecar/.env` automatically. With the current repo config, `uv sync` installs CPU-only PyTorch. Sidecar device selection is configured there:

```text
GLINER_DEVICE=cpu
GLINER_QUANTIZE=false
GLINER_COMPILE=false
```

If you later want GPU inference, replace the sidecar virtualenv torch install manually with a CUDA-enabled wheel before setting `GLINER_DEVICE=cuda`.

## Runtime model

- The backend starts empty.
- Runtime state is fully in memory.
- Documents are loaded through `POST /api/uploads/documents` as plain-text `.txt` uploads.
- Approval, retry, review, and export flows are designed to be idempotent where practical.

## Environment

Primary backend env files:

- [.env](.env)
- [.env.example](.env.example)

Current knobs:

- `WORKER_COUNT`
- `QUEUE_DEPTH`
- `GLINER_ENABLED`
- `GLINER_URL`
- `GLINER_TIMEOUT_MS`
- `GLINER_MAX_CONCURRENCY`

Set `GLINER_ENABLED=true` to enable sidecar-backed `PERSON`, `ADDRESS`, `EMAIL`, and `PHONE` detections. If the sidecar is unavailable, the backend falls back to regex-only detection.
`GLINER_MAX_CONCURRENCY` limits how many documents can be in GLiNER inference at once. Keep this low on a single machine to avoid memory spikes.

## API

Health and summary:

- `GET /healthz`
- `GET /api/batch/summary`

Upload and document reads:

- `POST /api/uploads/documents`
- `GET /api/documents`
- `GET /api/documents/:id`
- `GET /api/documents/:id/redactions`
- `GET /api/documents/:id/review-summary`

Review and mutation actions:

- `POST /api/documents/:id/redactions`
- `POST /api/redactions/:id/accept`
- `POST /api/redactions/:id/reject`
- `POST /api/documents/:id/approve`
- `POST /api/documents/bulk-approve`
- `POST /api/documents/:id/retry`
- `POST /api/documents/bulk-retry`

Export:

- `POST /api/export`
- `GET /api/export/latest`

## Upload behavior

- Accepts one or many `.txt` files using multipart field name `files`.
- `mode=replace` resets the active batch before loading new files.
- `mode=append` keeps the existing batch and adds more files.
- Upload reading is streamed part-by-part rather than whole-form parsed up front.
- Runtime detection is regex/rule-based by default.
- Optional local GLiNER sidecar detection can be enabled over localhost HTTP.
- When GLiNER is enabled, semantic labels (`PERSON`, `ADDRESS`, `EMAIL`, `PHONE`) come from the sidecar and structured identifiers stay regex-owned.
- Clean uploads can become `CLEAN`.
- Higher-confidence uploads can become `READY`.
- Uncertain uploads become `NEEDS_REVIEW`.
- Processing failures become `FAILED`.

## Review and approval rules

Redaction review state:

- Runtime review states are `PENDING`, `ACCEPTED`, `REJECTED`, and `ADDED`.
- User-added manual redactions use source `user_added`, suggested status `USER_ADDED`, and review state `ADDED`.

Approval safety:

- `FAILED` documents must be retried before review or approval.
- `NEEDS_REVIEW` documents cannot be approved while blocking review items remain.
- Bulk approve only approves `READY` and `CLEAN` documents by default.

## Export behavior

- Export only includes documents currently in `APPROVED` with no blocking review items.
- Export applies only redactions in runtime review state `ACCEPTED` or `ADDED`.
- Export skips `PENDING` and `REJECTED` redactions.
- Overlapping accepted spans are handled defensively and counted in the export summary.
- Export writes redacted `.txt` files to the repo-root `exported/` folder.
- Output filenames use the source filename plus the `_redacted` suffix.
- Each export run recreates `exported/` so stale files are removed.

## Logging

- Uses structured `log/slog`
- Does not log raw document text
- Does not log raw redaction text
- Upload instrumentation includes request timing, file counts, and byte totals
