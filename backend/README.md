# Backend API

Run from the `backend/` directory:

```bash
go run .
```

Required dataset files:

- `../dataset/processed/documents_seed.jsonl`
- `../dataset/processed/mock_redactions.jsonl`

Endpoints:

- `GET /healthz`
- `GET /api/batch/summary`
- `GET /api/documents`
- `GET /api/documents/:id`
- `GET /api/documents/:id/redactions`

Startup validation fails fast if the dataset files are missing, JSONL is invalid, IDs are duplicated, redactions reference unknown documents, or redaction spans do not match the seeded document text.

Privacy note: request/startup logs use structured `slog` fields and do not log raw document text or raw redaction text.
