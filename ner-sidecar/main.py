from __future__ import annotations

from contextlib import asynccontextmanager
from dataclasses import dataclass
import json
import logging
import os
import time
from typing import Any

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field
from dotenv import load_dotenv

from gliner2 import GLiNER2

load_dotenv()
os.environ.setdefault("HF_HUB_DISABLE_XET", "1")

MODEL_NAME = os.getenv("GLINER_MODEL", "fastino/gliner2-base-v1")
MODEL_DEVICE = os.getenv("GLINER_DEVICE", "cpu").strip().lower() or "cpu"
MODEL_QUANTIZE = os.getenv("GLINER_QUANTIZE", "false").strip().lower() in {
    "1",
    "true",
    "yes",
    "on",
}
MODEL_COMPILE = os.getenv("GLINER_COMPILE", "false").strip().lower() in {
    "1",
    "true",
    "yes",
    "on",
}
CHUNK_SIZE = 1800
CHUNK_OVERLAP = 250
MAX_TEXT_LENGTH = 200_000

logging.basicConfig(
    level=os.getenv("LOG_LEVEL", "INFO").upper(),
    format="%(asctime)s %(levelname)s %(message)s",
)
logger = logging.getLogger("ner_sidecar")

LABEL_SCHEMA = {
    "person": "Full names of people, clients, attorneys, employees, witnesses, or signatories.",
    "email": "Email addresses or inbox identifiers.",
    "phone": "Phone, mobile, landline, or fax numbers.",
    "address": "Street addresses, mailing addresses, and location lines with street numbers.",
}


class DetectRequest(BaseModel):
    document_id: str = Field(min_length=1)
    text: str = Field(min_length=1, max_length=MAX_TEXT_LENGTH)


class DetectItem(BaseModel):
    start: int
    end: int
    text: str
    label: str
    score: float


class DetectResponse(BaseModel):
    model: str
    items: list[DetectItem]


@dataclass(frozen=True)
class TextChunk:
    start: int
    end: int
    text: str


@asynccontextmanager
async def lifespan(_: FastAPI):
    logger.info(
        "sidecar_starting %s",
        json.dumps(
            {
                "model": MODEL_NAME,
                "device": MODEL_DEVICE,
                "quantize": MODEL_QUANTIZE,
                "compile": MODEL_COMPILE,
                "chunk_size": CHUNK_SIZE,
                "chunk_overlap": CHUNK_OVERLAP,
            }
        ),
    )
    app.state.extractor = GLiNER2.from_pretrained(
        MODEL_NAME,
        map_location=MODEL_DEVICE,
        quantize=MODEL_QUANTIZE,
        compile=MODEL_COMPILE,
    )
    logger.info(
        "sidecar_ready %s",
        json.dumps(
            {
                "model": MODEL_NAME,
                "device": MODEL_DEVICE,
                "quantize": MODEL_QUANTIZE,
                "compile": MODEL_COMPILE,
            }
        ),
    )
    yield
    logger.info(
        "sidecar_stopping %s",
        json.dumps({"model": MODEL_NAME, "device": MODEL_DEVICE}),
    )


app = FastAPI(title="Redactlane GLiNER2 Sidecar", lifespan=lifespan)


@app.get("/healthz")
def healthz() -> dict[str, Any]:
    loaded = getattr(app.state, "extractor", None) is not None
    logger.info(
        "healthz %s",
        json.dumps({"loaded": loaded, "model": MODEL_NAME, "device": MODEL_DEVICE}),
    )
    return {"status": "ok", "model": MODEL_NAME, "device": MODEL_DEVICE, "loaded": loaded}


@app.post("/detect", response_model=DetectResponse)
def detect_entities(request: DetectRequest) -> DetectResponse:
    extractor: GLiNER2 | None = getattr(app.state, "extractor", None)
    if extractor is None:
        logger.error(
            "detect_rejected %s",
            json.dumps({"document_id": request.document_id, "reason": "model_not_loaded"}),
        )
        raise HTTPException(status_code=503, detail="model not loaded")

    started_at = time.perf_counter()
    logger.info(
        "detect_started %s",
        json.dumps({"document_id": request.document_id, "char_count": len(request.text)}),
    )

    try:
        items, chunk_count = detect_with_chunking(extractor, request.text)
    except Exception as exc:  # pragma: no cover - runtime model failures are integration concerns
        logger.exception(
            "detect_failed %s",
            json.dumps(
                {
                    "document_id": request.document_id,
                    "char_count": len(request.text),
                    "duration_ms": round((time.perf_counter() - started_at) * 1000),
                    "error_type": type(exc).__name__,
                }
            ),
        )
        raise HTTPException(status_code=500, detail="local detection failed") from exc

    label_counts: dict[str, int] = {}
    for item in items:
        label_counts[item.label] = label_counts.get(item.label, 0) + 1

    logger.info(
        "detect_completed %s",
        json.dumps(
            {
                "document_id": request.document_id,
                "char_count": len(request.text),
                "chunk_count": chunk_count,
                "item_count": len(items),
                "label_counts": label_counts,
                "duration_ms": round((time.perf_counter() - started_at) * 1000),
            }
        ),
    )
    return DetectResponse(model=MODEL_NAME, items=items)


def detect_with_chunking(extractor: GLiNER2, text: str) -> tuple[list[DetectItem], int]:
    merged: dict[tuple[str, int, int], DetectItem] = {}
    chunks = chunk_text(text)

    for chunk in chunks:
        chunk_result = extractor.extract_entities(
            chunk.text,
            LABEL_SCHEMA,
            include_confidence=True,
            include_spans=True,
        )
        for label, raw_items in chunk_result.get("entities", {}).items():
            normalized_label = label.strip().upper()
            for raw_item in raw_items:
                start = int(raw_item["start"]) + chunk.start
                end = int(raw_item["end"]) + chunk.start
                if start < chunk.start or end > chunk.end or end <= start:
                    continue

                score = float(raw_item.get("confidence", 0.0))
                item = DetectItem(
                    start=start,
                    end=end,
                    text=text[start:end],
                    label=normalized_label,
                    score=score,
                )
                merge_item(merged, item)

    return sorted(merged.values(), key=lambda item: (item.start, item.end, item.label)), len(chunks)


def merge_item(merged: dict[tuple[str, int, int], DetectItem], candidate: DetectItem) -> None:
    exact_key = (candidate.label, candidate.start, candidate.end)
    existing = merged.get(exact_key)
    if existing is None or candidate.score > existing.score:
        merged[exact_key] = candidate

    for key, item in list(merged.items()):
        if key == exact_key or item.label != candidate.label:
            continue
        if not overlaps_heavily(item, candidate):
            continue
        if candidate.score > item.score:
            del merged[key]
            merged[exact_key] = candidate
        return


def overlaps_heavily(left: DetectItem, right: DetectItem) -> bool:
    overlap_start = max(left.start, right.start)
    overlap_end = min(left.end, right.end)
    if overlap_start >= overlap_end:
        return False

    intersection = overlap_end - overlap_start
    shorter = min(left.end - left.start, right.end - right.start)
    return intersection / shorter >= 0.5


def chunk_text(text: str) -> list[TextChunk]:
    if len(text) <= CHUNK_SIZE:
        return [TextChunk(start=0, end=len(text), text=text)]

    chunks: list[TextChunk] = []
    start = 0
    while start < len(text):
        end = min(len(text), start + CHUNK_SIZE)
        chunks.append(TextChunk(start=start, end=end, text=text[start:end]))
        if end == len(text):
            break
        start = max(end - CHUNK_OVERLAP, start + 1)
    return chunks
