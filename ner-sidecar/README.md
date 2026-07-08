# NER Sidecar

Optional local FastAPI sidecar for GLiNER2 inference.

The backend does not require this process to run. When enabled, the sidecar supplies semantic detections that complement the backend's regex detection.

Current label schema:

- `PERSON`
- `EMAIL`
- `PHONE`
- `ADDRESS`

## Install

```bash
cd ner-sidecar
uv sync
```

The current `uv` config pins PyTorch to the CPU wheel by default.

## Run

```bash
uv run uvicorn main:app --host 127.0.0.1 --port 8090
```

The sidecar loads `ner-sidecar/.env` automatically at startup. Explicit environment variables still override file values.

Example `.env`:

```text
GLINER_MODEL=fastino/gliner2-base-v1
GLINER_DEVICE=cpu
GLINER_QUANTIZE=false
GLINER_COMPILE=false
LOG_LEVEL=INFO
```

## Backend Integration

Enable the sidecar from `backend/.env`:

```text
GLINER_ENABLED=true
GLINER_URL=http://127.0.0.1:8090
GLINER_TIMEOUT_MS=120000
GLINER_MAX_CONCURRENCY=1
```

If the sidecar request fails, the backend falls back to regex-only detection for that document.

## Runtime Behavior

- model is loaded once during FastAPI startup
- requests larger than `200000` characters are rejected by request validation
- text is chunked into `1800` character windows with `250` character overlap
- overlapping same-label detections are merged by span and confidence
- logs include startup, health, per-request timing, chunk count, and label counts
- logs do not include raw document text

## API

- `GET /healthz`
- `POST /detect`

Example request:

```json
{
  "document_id": "upload_000001",
  "text": "John Smith lives at 100 Main Street."
}
```

Example response shape:

```json
{
  "model": "fastino/gliner2-base-v1",
  "items": [
    {
      "start": 0,
      "end": 10,
      "text": "John Smith",
      "label": "PERSON",
      "score": 0.91
    }
  ]
}
```

## CPU and GPU Notes

CPU is the current default path.

If you want CUDA later, replace the installed torch wheel manually after `uv sync`:

```bash
uv pip uninstall torch torchvision torchaudio
uv pip install torch torchvision torchaudio --index-url https://download.pytorch.org/whl/cu126
```

Then set:

```text
GLINER_DEVICE=cuda
```

Start with `GLINER_QUANTIZE=false` and `GLINER_COMPILE=false` until the plain CUDA path is stable on the target machine.
