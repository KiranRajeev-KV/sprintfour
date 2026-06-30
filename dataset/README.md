# Dataset Preparation

This repository uses the CUAD v1 legal contract dataset as the starting corpus for the Sprintfour batch-review product.

## What this step does

- Downloads the CUAD v1 archive from an official source.
- Extracts the raw CUAD archive.
- Verifies the extracted plain-text contract folder exists at `dataset/raw/cuad_v1/extracted/CUAD_v1/full_contract_txt`.
- Reports the number of extracted `.txt` files.

## What this step does not do

- No JSONL preparation.
- No synthetic PII injection.
- No mock detections.
- No frontend or backend product logic.

## Manual synthetic samples

Manually created synthetic text samples for upload and workflow testing now live under:

- `dataset/raw/manual_synthetic_txt/`

These are separate from the CUAD extraction flow and are intended for local batch upload and exception-path testing.

## Command

```bash
python3 dataset/scripts/prepare_cuad.py
python3 dataset/scripts/benchmark_upload.py 100
python3 dataset/scripts/generate_mock_redactions.py
```

## Output of `prepare_cuad.py`

- `dataset/raw/cuad_v1/downloads/CUAD_v1.zip`
- `dataset/raw/cuad_v1/extracted/CUAD_v1/full_contract_txt/`

## Processed mock-data artifacts

Other dataset scripts may generate:

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
