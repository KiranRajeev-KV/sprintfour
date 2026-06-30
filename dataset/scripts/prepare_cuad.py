#!/usr/bin/env python3
from __future__ import annotations

import argparse
import shutil
import sys
import urllib.request
import zipfile
from pathlib import Path


DEFAULT_SOURCE_URL = "https://zenodo.org/records/4595826/files/CUAD_v1.zip?download=1"
FALLBACK_SOURCE_URL = "https://github.com/TheAtticusProject/cuad/raw/main/data.zip"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Download and extract CUAD v1 until full_contract_txt is available."
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


def full_contract_dir(extracted_dir: Path) -> Path:
    return extracted_dir / "CUAD_v1" / "full_contract_txt"


def main() -> int:
    args = parse_args()

    root = repo_root()
    raw_dir = root / "dataset" / "raw" / "cuad_v1"
    archive_path = raw_dir / "downloads" / "CUAD_v1.zip"
    extracted_dir = raw_dir / "extracted"

    source_url_used = ensure_archive(
        archive_path=archive_path,
        primary_url=args.source_url,
        fallback_url=args.fallback_source_url,
        skip_download=args.skip_download,
    )
    extract_archive(archive_path=archive_path, extracted_dir=extracted_dir)

    contracts_dir = full_contract_dir(extracted_dir)
    if not contracts_dir.exists() or not contracts_dir.is_dir():
        raise RuntimeError(f"expected extracted folder at {contracts_dir}")

    contract_files = sorted(path for path in contracts_dir.glob("*.txt") if path.is_file())
    print(f"[prepare_cuad] source url used: {source_url_used}")
    print(f"[prepare_cuad] extracted contracts dir: {contracts_dir}")
    print(f"[prepare_cuad] found {len(contract_files)} TXT files")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:  # noqa: BLE001
        print(f"[prepare_cuad] error: {exc}", file=sys.stderr)
        raise
