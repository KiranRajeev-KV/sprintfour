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

## Runtime model

- The backend starts empty.
- Runtime state is fully in memory.
- Documents are loaded through `POST /api/uploads/documents` as plain-text `.txt` uploads.
- Approval, retry, review, and export flows are designed to be idempotent where practical.

## Environment

Primary backend env files:

- [backend/.env](/home/kr/dev/sprintfour/backend/.env:1)
- [backend/.env.example](/home/kr/dev/sprintfour/backend/.env.example:1)

Current knobs:

- `WORKER_COUNT`
- `QUEUE_DEPTH`

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
- Runtime detection is deterministic and regex/rule-based.
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
