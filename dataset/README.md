# Dataset Preparation

This repository uses the CUAD v1 legal contract dataset as the starting corpus for the Sprintfour batch-review product.

## What this step does

- Downloads the CUAD v1 archive from an official source.
- Extracts the plain-text contract files.
- Selects the first 220 `.txt` contracts in deterministic sorted order.
- Normalizes them into UTF-8 JSONL records for backend seeding.
- Writes a small manifest describing the prepared output.

## What this step does not do

- No synthetic PII injection.
- No mock detections.
- No frontend or backend product logic.

## Command

```bash
python3 dataset/scripts/prepare_cuad.py
python3 dataset/scripts/generate_mock_redactions.py
```

## Generated files

- `dataset/processed/cuad_contracts.jsonl`
- `dataset/processed/cuad_manifest.json`
- `dataset/processed/documents_seed.jsonl`
- `dataset/processed/mock_redactions.jsonl`
- `dataset/processed/mock_redaction_manifest.json`
- `dataset/processed/pii_exploration_summary.json`

## Mock redaction step

`dataset/scripts/generate_mock_redactions.py` reads the processed CUAD contracts, explores existing PII-like regex candidates, injects deterministic synthetic PII into a controlled subset of documents, and writes mock redaction suggestions for the hackathon batch workflow.

- Synthetic injection means the script adds deterministic fake contact/identifier values into selected documents so later backend/frontend work has stable review cases.
- Regex candidates mean the script flags existing email-like, phone-like, or similar patterns as heuristic review signals only. They are not treated as production-quality PII detection or as ground truth.
- The generated outputs are mock suggestions for hackathon workflow demos, not production PII detection.

## Ignored raw files

Large raw CUAD downloads and extracted source files are kept under `dataset/raw/cuad_v1/` and ignored by git.
