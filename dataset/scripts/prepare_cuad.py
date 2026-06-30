#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import shutil
import sys
import urllib.request
import zipfile
from datetime import datetime, timezone
from pathlib import Path
import re


DEFAULT_SOURCE_URL = "https://zenodo.org/records/4595826/files/CUAD_v1.zip?download=1"
FALLBACK_SOURCE_URL = "https://github.com/TheAtticusProject/cuad/raw/main/data.zip"
DATASET_NAME = "CUAD_v1"
TARGET_RECORD_COUNT = 220
MINIMUM_RECORD_COUNT = 200
DEFAULT_MAX_CHARS = 20000


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Download, extract, and normalize CUAD v1 TXT contracts into JSONL."
    )
    parser.add_argument(
        "--source-url",
        default=DEFAULT_SOURCE_URL,
        help="Primary archive URL to download.",
    )
    parser.add_argument(
        "--fallback-source-url",
        default=FALLBACK_SOURCE_URL,
        help="Fallback archive URL if the primary download fails.",
    )
    parser.add_argument(
        "--max-records",
        type=int,
        default=TARGET_RECORD_COUNT,
        help="Maximum number of TXT contracts to write.",
    )
    parser.add_argument(
        "--min-records",
        type=int,
        default=MINIMUM_RECORD_COUNT,
        help="Minimum number of TXT contracts required to continue.",
    )
    parser.add_argument(
        "--max-chars",
        type=int,
        default=DEFAULT_MAX_CHARS,
        help="Maximum normalized characters per document.",
    )
    parser.add_argument(
        "--skip-download",
        action="store_true",
        help="Reuse an existing archive or extracted files if present.",
    )
    return parser.parse_args()


def repo_root() -> Path:
    return Path(__file__).resolve().parents[2]


def download_file(url: str, destination: Path) -> str:
    destination.parent.mkdir(parents=True, exist_ok=True)
    print(f"[prepare_cuad] downloading {url}")
    temporary_path = destination.with_suffix(destination.suffix + ".tmp")
    if temporary_path.exists():
        temporary_path.unlink()
    with urllib.request.urlopen(url) as response, temporary_path.open("wb") as output_file:
        shutil.copyfileobj(response, output_file)
    temporary_path.replace(destination)
    return url


def ensure_archive(archive_path: Path, primary_url: str, fallback_url: str, skip_download: bool) -> str:
    if archive_path.exists() and zipfile.is_zipfile(archive_path):
        print(f"[prepare_cuad] reusing archive {archive_path}")
        return primary_url

    if archive_path.exists():
        print(f"[prepare_cuad] removing invalid archive {archive_path}")
        archive_path.unlink()

    if skip_download:
        raise FileNotFoundError(
            f"archive not found at {archive_path} and --skip-download was provided"
        )

    errors: list[str] = []
    for candidate in (primary_url, fallback_url):
        try:
            return download_file(candidate, archive_path)
        except Exception as exc:  # noqa: BLE001
            errors.append(f"{candidate}: {exc}")
            if archive_path.exists():
                archive_path.unlink()

    raise RuntimeError("failed to download CUAD archive:\n" + "\n".join(errors))


def extract_archive(archive_path: Path, extracted_dir: Path) -> None:
    if extracted_dir.exists():
        print(f"[prepare_cuad] removing previous extraction at {extracted_dir}")
        shutil.rmtree(extracted_dir)

    print(f"[prepare_cuad] extracting {archive_path.name}")
    extracted_dir.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(archive_path) as archive:
        archive.extractall(extracted_dir)


def find_contract_files(extracted_dir: Path) -> list[Path]:
    candidates = sorted(path for path in extracted_dir.rglob("*.txt") if path.is_file())
    prioritized = [
        path
        for path in candidates
        if "full_contract_txt" in {part.lower() for part in path.parts}
    ]
    contract_files = prioritized or candidates
    return [
        path
        for path in contract_files
        if "readme" not in path.name.lower()
    ]


def normalize_text(text: str) -> str:
    text = text.replace("\r\n", "\n").replace("\r", "\n")
    text = text.replace("\t", " ")
    text = "\n".join(line.rstrip() for line in text.split("\n"))
    text = re.sub(r"[ ]{2,}", " ", text)
    text = re.sub(r"\n{3,}", "\n\n", text)
    return text.strip()


def truncate_text(text: str, max_chars: int) -> tuple[str, bool]:
    if max_chars <= 0 or len(text) <= max_chars:
        return text, False
    return text[:max_chars].rstrip(), True


def build_record(index: int, file_path: Path, max_chars: int) -> tuple[dict[str, object], bool]:
    original_text = file_path.read_text(encoding="utf-8", errors="strict")
    normalized_text = normalize_text(original_text)
    truncated_text, was_truncated = truncate_text(normalized_text, max_chars)
    record = {
        "id": f"cuad_{index:04d}",
        "title": file_path.stem,
        "source": DATASET_NAME,
        "source_file": file_path.name,
        "text": truncated_text,
        "char_count": len(truncated_text),
    }
    return record, was_truncated


def write_jsonl(output_path: Path, records: list[dict[str, object]]) -> None:
    output_path.parent.mkdir(parents=True, exist_ok=True)
    with output_path.open("w", encoding="utf-8", newline="\n") as output_file:
        for record in records:
            output_file.write(json.dumps(record, ensure_ascii=False) + "\n")


def write_manifest(
    manifest_path: Path,
    source_url_used: str,
    source_txt_count: int,
    records_written: int,
    any_truncated: bool,
    max_chars: int,
) -> None:
    manifest = {
        "dataset_name": DATASET_NAME,
        "source_url_used": source_url_used,
        "source_txt_files_found": source_txt_count,
        "records_written": records_written,
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "text_truncated": any_truncated,
        "max_characters_per_document": max_chars if any_truncated else None,
    }
    manifest_path.parent.mkdir(parents=True, exist_ok=True)
    manifest_path.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")


def main() -> int:
    args = parse_args()
    if args.max_records < args.min_records:
        raise ValueError("--max-records must be greater than or equal to --min-records")

    root = repo_root()
    raw_dir = root / "dataset" / "raw" / "cuad_v1"
    archive_path = raw_dir / "downloads" / "CUAD_v1.zip"
    extracted_dir = raw_dir / "extracted"
    processed_dir = root / "dataset" / "processed"
    jsonl_path = processed_dir / "cuad_contracts.jsonl"
    manifest_path = processed_dir / "cuad_manifest.json"

    source_url_used = ensure_archive(
        archive_path=archive_path,
        primary_url=args.source_url,
        fallback_url=args.fallback_source_url,
        skip_download=args.skip_download,
    )
    extract_archive(archive_path=archive_path, extracted_dir=extracted_dir)

    contract_files = find_contract_files(extracted_dir)
    print(f"[prepare_cuad] found {len(contract_files)} TXT files")
    if len(contract_files) < args.min_records:
        raise RuntimeError(
            f"expected at least {args.min_records} TXT contracts, found {len(contract_files)}"
        )

    selected_files = contract_files[: args.max_records]
    records: list[dict[str, object]] = []
    any_truncated = False
    for index, file_path in enumerate(selected_files, start=1):
        record, was_truncated = build_record(index=index, file_path=file_path, max_chars=args.max_chars)
        records.append(record)
        any_truncated = any_truncated or was_truncated

    write_jsonl(jsonl_path, records)
    write_manifest(
        manifest_path=manifest_path,
        source_url_used=source_url_used,
        source_txt_count=len(contract_files),
        records_written=len(records),
        any_truncated=any_truncated,
        max_chars=args.max_chars,
    )

    print(f"[prepare_cuad] wrote {len(records)} records to {jsonl_path}")
    print(f"[prepare_cuad] wrote manifest to {manifest_path}")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:  # noqa: BLE001
        print(f"[prepare_cuad] error: {exc}", file=sys.stderr)
        raise
