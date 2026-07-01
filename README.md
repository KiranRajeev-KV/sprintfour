# Redactlane

[Hackathon writeup](./writeup.md) · [Demo video](./demo.mp4)

Redactlane is a batch-first anonymization workflow for high-volume case file review. It is built for exception-first triage: safe files move forward quickly, risky files are routed to focused review, failed files can be retried, and approved files can be exported without blocking the rest of the batch.

## Dev Docs

### Stack

- Frontend: Vite, React, TypeScript, TanStack Router, TanStack Query
- Backend: Go, Gin, `log/slog`

### Repo layout

- `frontend/`: UI application
- `backend/`: API server
- `ner-sidecar/`: local FastAPI + GLiNER2 sidecar
- `dataset/`: CUAD source corpus, manual synthetic samples, and helper scripts
- `exported/`: exported redacted text output
- `benchmark.md`: local upload benchmark results

### Environment

Backend env files live in `backend/`:

- [backend/.env](backend/.env)
- [backend/.env.example](backend/.env.example)

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

Optional local GLiNER sidecar:

```bash
cd ner-sidecar
uv sync
env UV_CACHE_DIR=/tmp/uv-cache HF_HUB_DISABLE_XET=1 uv run uvicorn main:app --host 127.0.0.1 --port 8090
```

The sidecar loads `ner-sidecar/.env` automatically. With the current repo config, `uv sync` installs the CPU-only PyTorch wheel.

Enable it in `backend/.env`:

```text
GLINER_ENABLED=true
GLINER_URL=http://127.0.0.1:8090
GLINER_TIMEOUT_MS=120000
GLINER_MAX_CONCURRENCY=1
```

Recommended sidecar `.env`:

```text
GLINER_MODEL=fastino/gliner2-base-v1
GLINER_DEVICE=cpu
GLINER_QUANTIZE=false
GLINER_COMPILE=false
```

If you later want GPU inference, you will need to replace the CPU-only torch install in `ner-sidecar/.venv` with a CUDA-enabled wheel manually.

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

Backend API and workflow details are documented in [backend/README.md](backend/README.md).

### Benchmark

Measured upload timings and dataset-size benchmarks are documented in [benchmark.md](benchmark.md). The current benchmark document is for pure regex detection only, without the optional GLiNER sidecar.

### Arch
[Arch](arch.png)
