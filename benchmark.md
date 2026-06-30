# Redactlane Upload Benchmark

Date: 2026-06-30

## Setup

- Backend: local server on `http://localhost:8080`
- Worker pool default: `8`
- Queue depth: `200`
- Upload mode: `replace`
- Corpus: `/home/kr/dev/sprintfour/dataset/raw/cuad_v1/extracted/CUAD_v1/full_contract_txt`
- Measurement script: `dataset/scripts/benchmark_upload.py`
- `.env` handling: the backend now loads `backend/.env` at startup when present, while still allowing explicit process environment variables to override file values.

## Timing results

| Files | Time (s) | Total Bytes | Text Characters | Est. Tokens (`chars / 4`) | Word-like Tokens |
| --- | ---: | ---: | ---: | ---: | ---: |
| 10 | 0.209 | 349,937 | 349,820 | 87,455 | 53,312 |
| 50 | 0.213 | 3,256,306 | 3,255,590 | 813,898 | 490,751 |
| 100 | 0.419 | 5,959,391 | 5,958,008 | 1,489,502 | 894,832 |
| 200 | 0.838 | 11,126,826 | 11,123,513 | 2,780,879 | 1,669,836 |
| 300 | 1.121 | 16,703,668 | 16,698,527 | 4,174,632 | 2,491,160 |
| 400 | 1.423 | 21,702,275 | 21,694,696 | 5,423,674 | 3,246,967 |
| 510 | 1.770 | 26,820,468 | 26,807,133 | 6,701,784 | 4,009,206 |

Notes:

- `Est. Tokens (chars / 4)` is only a rough estimate, not model tokenizer output.
- `Word-like Tokens` is whitespace-style token counting, not model tokenizer output.

## Final batch summaries

### 200 files

- `ready`: `16`
- `needs_review`: `66`
- `clean`: `118`
- `failed`: `0`
- `total_redactions`: `635`

### 300 files

- `ready`: `25`
- `needs_review`: `98`
- `clean`: `177`
- `failed`: `0`
- `total_redactions`: `868`

### 400 files

- `ready`: `35`
- `needs_review`: `126`
- `clean`: `239`
- `failed`: `0`
- `total_redactions`: `1261`

### 510 files

- `ready`: `43`
- `needs_review`: `167`
- `clean`: `300`
- `failed`: `0`
- `total_redactions`: `1493`
