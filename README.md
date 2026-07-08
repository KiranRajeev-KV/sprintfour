# Redactlane

[Hackathon writeup](./writeup.md) · [Benchmark notes](./benchmark.md)

Redactlane is a batch-first anonymization workflow for high-volume case review. It is built around triage, not one-document-at-a-time editing:

- safe documents move to bulk approval
- risky or uncertain documents move to focused review
- failed documents can be retried without blocking the rest of the batch
- approved documents can be exported as redacted `.txt` files

## Repo

- `frontend/`: React UI
- `backend/`: Go API server
- `ner-sidecar/`: optional local FastAPI + GLiNER2 sidecar
- `dataset/`: CUAD prep scripts and manual text fixtures
- `exported/`: generated redacted output

## Quick Start

Backend:

```bash
cd backend/cmd/server
go run .
```

API:

```text
http://127.0.0.1:8080
```

Frontend:

```bash
cd frontend
bun install
bun run dev
```

UI:

```text
http://127.0.0.1:3000
```

Optional local NER sidecar:

```bash
cd ner-sidecar
uv sync
uv run uvicorn main:app --host 127.0.0.1 --port 8090
```

The backend works without the sidecar. When the sidecar is enabled, it adds semantic detections for `PERSON`, `ADDRESS`, `EMAIL`, and `PHONE`.

## Fixtures

Manual upload fixtures used by tests and smoke runs live in:

```text
dataset/raw/manual_synthetic_txt/
```

## Verification

Backend tests:

```bash
cd backend
go test ./...
go vet ./...
```

Full backend API smoke test:

```bash
bash backend/scripts/api_smoke.sh
```

## Docs

- Backend API and runtime behavior: [backend/README.md](./backend/README.md)
- Sidecar setup and API: [ner-sidecar/README.md](./ner-sidecar/README.md)
- Dataset layout and prep scripts: [dataset/README.md](./dataset/README.md)
