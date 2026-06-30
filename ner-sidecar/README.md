# NER Sidecar

Local FastAPI sidecar for GLiNER2 inference.

This project uses `uv` and the repo currently pins PyTorch to the CPU-only wheel.
The sidecar loads `ner-sidecar/.env` automatically at startup. Explicit process env vars still override file values.

## Install

```bash
cd ner-sidecar
uv sync
```

If you want CUDA later, you must replace the CPU-only PyTorch wheel manually after `uv sync`:

```bash
uv pip uninstall torch torchvision torchaudio
uv pip install torch torchvision torchaudio --index-url https://download.pytorch.org/whl/cu126
```

## Run

```bash
uv run uvicorn main:app --host 127.0.0.1 --port 8090
```

Example `.env`:

```text
HF_HUB_DISABLE_XET=1
GLINER_MODEL=fastino/gliner2-base-v1
GLINER_DEVICE=cpu
GLINER_QUANTIZE=false
GLINER_COMPILE=false
LOG_LEVEL=INFO
```

The sidecar logs startup, health checks, per-document detection timings, chunk counts, and label counts. It does not log raw document text.

If you want GPU inference, install the CUDA-enabled PyTorch wheel manually in the sidecar virtualenv and then set `GLINER_DEVICE=cuda`. Start with `GLINER_QUANTIZE=false` and `GLINER_COMPILE=false` until the plain CUDA path is stable on your machine, then turn tuning flags on deliberately.

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
