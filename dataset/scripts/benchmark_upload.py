#!/usr/bin/env python3
from __future__ import annotations

import json
import math
import mimetypes
import re
import sys
import time
import uuid
from pathlib import Path
from urllib import request


REPO_ROOT = Path(__file__).resolve().parents[2]
DATASET_DIR = REPO_ROOT / "dataset" / "raw" / "cuad_v1" / "extracted" / "CUAD_v1" / "full_contract_txt"
UPLOAD_URL = "http://localhost:8080/api/uploads/documents"
SUMMARY_URL = "http://localhost:8080/api/batch/summary"


def log(message: str) -> None:
    timestamp = time.strftime("%H:%M:%S")
    print(f"[{timestamp}] {message}", file=sys.stderr, flush=True)


def main() -> None:
    if len(sys.argv) != 2:
        raise SystemExit("usage: python3 dataset/scripts/benchmark_upload.py <count>")

    expected_total = int(sys.argv[1])
    files = sorted(DATASET_DIR.glob("*.txt"))[:expected_total]
    if len(files) < expected_total:
        raise SystemExit(f"expected at least {expected_total} files, found {len(files)}")

    log(f"selected {len(files)} files from {DATASET_DIR}")

    boundary = f"----RedactlaneBoundary{uuid.uuid4().hex}"
    body = bytearray()
    newline = b"\r\n"
    total_file_bytes = 0
    total_text_chars = 0
    total_word_like_tokens = 0

    def add_field(name: str, value: str) -> None:
        body.extend(f"--{boundary}".encode())
        body.extend(newline)
        body.extend(f'Content-Disposition: form-data; name="{name}"'.encode())
        body.extend(newline)
        body.extend(newline)
        body.extend(value.encode())
        body.extend(newline)

    def add_file(name: str, path: Path) -> None:
        nonlocal total_file_bytes, total_text_chars, total_word_like_tokens
        mime_type = mimetypes.guess_type(path.name)[0] or "text/plain"
        file_bytes = path.read_bytes()
        file_size = len(file_bytes)
        text = file_bytes.decode("utf-8", errors="replace")
        total_file_bytes += file_size
        total_text_chars += len(text)
        total_word_like_tokens += len(re.findall(r"\S+", text))
        log(f"add file {path.name} size={file_size}")
        body.extend(f"--{boundary}".encode())
        body.extend(newline)
        disposition = f'Content-Disposition: form-data; name="{name}"; filename="{path.name}"'
        body.extend(disposition.encode())
        body.extend(newline)
        body.extend(f"Content-Type: {mime_type}".encode())
        body.extend(newline)
        body.extend(newline)
        body.extend(file_bytes)
        body.extend(newline)

    add_field("mode", "replace")
    for path in files:
        add_file("files", path)
    body.extend(f"--{boundary}--".encode())
    body.extend(newline)
    log(f"multipart body prepared bytes={len(body)}")

    headers = {
        "Content-Type": f"multipart/form-data; boundary={boundary}",
        "Content-Length": str(len(body)),
    }

    start = time.perf_counter()
    upload_req = request.Request(UPLOAD_URL, data=bytes(body), headers=headers, method="POST")
    log("starting upload request")
    with request.urlopen(upload_req, timeout=1800) as response:
        upload_payload = json.loads(response.read().decode())
    log(f"upload request complete batch_id={upload_payload.get('batch_id')}")

    last_poll_log = 0.0
    while True:
        with request.urlopen(SUMMARY_URL, timeout=30) as response:
            summary = json.loads(response.read().decode())
        now = time.perf_counter()
        if now - last_poll_log >= 1.0:
            log(
                "poll "
                f"total={summary.get('total_documents')} "
                f"queued={summary.get('queued')} "
                f"processing={summary.get('processing')} "
                f"ready={summary.get('ready')} "
                f"needs_review={summary.get('needs_review')} "
                f"clean={summary.get('clean')} "
                f"failed={summary.get('failed')}"
            )
            last_poll_log = now
        if (
            summary.get("total_documents") == expected_total
            and summary.get("queued") == 0
            and summary.get("processing") == 0
        ):
            break
        time.sleep(0.2)

    elapsed_seconds = time.perf_counter() - start
    log(f"completed elapsed_seconds={elapsed_seconds:.3f}")
    print(
        json.dumps(
            {
                "uploaded_files": len(files),
                "total_file_bytes": total_file_bytes,
                "total_text_characters": total_text_chars,
                "estimated_tokens_chars_div_4": math.ceil(total_text_chars / 4),
                "word_like_token_count": total_word_like_tokens,
                "elapsed_seconds": round(elapsed_seconds, 3),
                "upload_result": {
                    "batch_id": upload_payload.get("batch_id"),
                    "documents_created": upload_payload.get("documents_created"),
                    "accepted": upload_payload.get("accepted"),
                    "redactions_created": upload_payload.get("redactions_created"),
                },
                "final_summary": summary,
            },
            indent=2,
        )
    )


if __name__ == "__main__":
    main()
