# Dataset

This directory contains two things:

- source material and helper scripts for CUAD-based experimentation
- manual text fixtures used for local uploads, tests, and smoke runs

## Layout

- `raw/cuad_v1/`: downloaded and extracted CUAD artifacts
- `raw/manual_synthetic_txt/`: hand-written `.txt` fixtures for backend upload and export flows
- `processed/`: generated JSONL seed artifacts used by tests and offline prep scripts
- `scripts/`: dataset preparation and benchmarking helpers

## Main Commands

Prepare CUAD:

```bash
python3 dataset/scripts/prepare_cuad.py
```

Generate processed mock artifacts:

```bash
python3 dataset/scripts/generate_mock_redactions.py
```

Run upload benchmark:

```bash
python3 dataset/scripts/benchmark_upload.py 100
```

## Important Notes

- the backend does not load dataset seeds at runtime
- the backend starts empty and expects uploads through the API
- `dataset/raw/manual_synthetic_txt/` is the fixture set used by `backend/scripts/api_smoke.sh`
- generated JSONL files under `processed/` are primarily used by tests and offline mock-data prep

## Generated Outputs

The prep scripts may produce:

- `processed/documents_seed.jsonl`
- `processed/mock_redactions.jsonl`
- `processed/mock_redaction_manifest.json`
- `processed/pii_exploration_summary.json`
