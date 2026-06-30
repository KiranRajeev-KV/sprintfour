# Redactlane

Redactlane is a batch-first anonymization workflow for high-volume case file review. It is built for exception-first triage: safe files move forward quickly, risky files are routed to focused review, failed files can be retried, and approved files can be exported without blocking the rest of the batch.

## Dev Docs

### Stack

- Frontend: Vite, React, TypeScript, TanStack Router, TanStack Query
- Backend: Go, Gin, `log/slog`

### Repo layout

- `frontend/`: UI application
- `backend/`: API server
- `dataset/`: CUAD source corpus, manual synthetic samples, and helper scripts
- `exported/`: exported redacted text output
- `benchmark.md`: local upload benchmark results

### Environment

Backend env files live in `backend/`:

- [backend/.env](/home/kr/dev/sprintfour/backend/.env:1)
- [backend/.env.example](/home/kr/dev/sprintfour/backend/.env.example:1)

The backend loads `backend/.env` automatically at startup when present. Explicit process environment variables still override file values.

### Dataset

CUAD raw extraction target:

- `dataset/raw/cuad_v1/extracted/CUAD_v1/full_contract_txt/`

Manual synthetic upload samples:

- `dataset/raw/manual_synthetic_txt/`

Useful dataset scripts:

```bash
python3 dataset/scripts/prepare_cuad.py
python3 dataset/scripts/benchmark_upload.py 100
python3 dataset/scripts/generate_mock_redactions.py
```

### Run backend

From `backend/`:

```bash
go run .
```

Default local API address:

```text
http://localhost:8080
```

### Run frontend

From `frontend/`:

```bash
bun install
bun run dev
```

Default local UI address:

```text
http://localhost:3000
```

### Test

Backend:

```bash
cd backend
go test ./...
```

Frontend:

```bash
cd frontend
bun run build
bun run test
```

### API and backend behavior

Backend API and workflow details are documented in [backend/README.md](/home/kr/dev/sprintfour/backend/README.md:1).

### Benchmark

Measured upload timings and dataset-size benchmarks are documented in [benchmark.md](/home/kr/dev/sprintfour/benchmark.md:1).
