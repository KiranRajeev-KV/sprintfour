# Backend API

Run from the `backend/` directory:

```bash
go run .
```

Startup behavior:

- The backend now starts empty.
- CUAD dataset files are no longer required at runtime.
- Documents are loaded through `POST /api/uploads/documents` as `.txt` uploads.

Endpoints:

- `GET /healthz`
- `GET /api/batch/summary`
- `POST /api/uploads/documents`
- `GET /api/documents`
- `GET /api/documents/:id`
- `GET /api/documents/:id/redactions`
- `GET /api/documents/:id/review-summary`
- `POST /api/documents/:id/redactions`
- `POST /api/redactions/:id/accept`
- `POST /api/redactions/:id/reject`
- `POST /api/documents/:id/approve`
- `POST /api/documents/bulk-approve`
- `POST /api/documents/:id/retry`
- `POST /api/documents/bulk-retry`
- `POST /api/export`
- `GET /api/export/latest`

Upload behavior:

- Accepts one or many `.txt` files with multipart field name `files`.
- `mode=replace` resets the active in-memory batch before loading the new files.
- `mode=append` keeps the existing batch and adds more uploaded files.
- Runtime detection uses deterministic regex/rule-based matching for email, phone, PAN-like IDs, case IDs, client IDs, account-like IDs, and conservative address-like phrases.
- Clean uploads can remain `CLEAN`; high-confidence detections become `READY`; uncertain detections become `NEEDS_REVIEW`; ingestion failures become `FAILED`.

Runtime workflow state is kept fully in memory. Approval, retry, redaction review, and export operations are idempotent where practical and only affect the running process.

Redaction review state is also kept in memory:

- Seed suggestions initialize as `ACCEPTED` or `PENDING` based on `suggested_status` and source.
- Runtime review states are `PENDING`, `ACCEPTED`, `REJECTED`, and `ADDED`.
- User-added manual redactions use source `user_added`, suggested status `USER_ADDED`, and review state `ADDED`.

Approval safety rule:

- `FAILED` documents must be retried before review or approval.
- `NEEDS_REVIEW` documents cannot be approved while they still have unresolved blocking redaction items.
- Bulk approve approves `READY` and `CLEAN` documents by default.

Export behavior:

- Export only includes documents currently in `APPROVED` with no blocking review items.
- Export applies only redactions in runtime review state `ACCEPTED` or `ADDED`.
- Export skips `PENDING` and `REJECTED` redactions.
- Overlapping accepted spans are handled defensively and counted in the export summary rather than corrupting output.
- Export writes redacted `.txt` files to `../exported/` from the backend directory, which is `/home/kr/dev/sprintfour/exported` at the repo root.
- Output filenames use the source filename plus the `_redacted` suffix before the extension, for example `case-note_redacted.txt`.
- Each export run recreates the `exported/` folder so stale files from older exports are removed.

Privacy note: request/startup logs use structured `slog` fields and do not log raw document text or raw redaction text.
