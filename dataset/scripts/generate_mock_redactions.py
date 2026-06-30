#!/usr/bin/env python3
from __future__ import annotations

import hashlib
import json
import re
import statistics
import sys
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path


DETERMINISTIC_SEED = "sprintfour-cuad-mock-redactions-v1"
INPUT_JSONL = Path("dataset/processed/cuad_contracts.jsonl")
OUTPUT_DOCUMENTS = Path("dataset/processed/documents_seed.jsonl")
OUTPUT_REDACTIONS = Path("dataset/processed/mock_redactions.jsonl")
OUTPUT_MANIFEST = Path("dataset/processed/mock_redaction_manifest.json")
OUTPUT_EXPLORATION = Path("dataset/processed/pii_exploration_summary.json")

FAILED_COUNT = 12
REVIEW_COUNT = 45
CLEAN_COUNT = 11
REVIEW_REGEX_DOC_COUNT = 18
INJECTED_READY_COUNT = 30
INJECTED_REVIEW_COUNT = 18
MISSED_CASE_COUNT = 6
OVERLAP_CASE_COUNT = 6
HIGH_PII_CASE_COUNT = 6
FALSE_POSITIVE_DOC_COUNT = 12

EMAIL_RE = re.compile(r"\b[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[A-Za-z]{2,}\b")
PHONE_RE = re.compile(r"(?:\+\d{1,3}[\s-]?)?(?:\(?\d{2,4}\)?[\s-]?){2,4}\d{2,4}")
ADDRESS_RE = re.compile(
    r"\b\d{1,5}\s+[A-Z][A-Za-z]+(?:\s+[A-Z][A-Za-z]+){0,5},\s*[A-Z][A-Za-z .]+,\s*[A-Z][A-Za-z .]+\s+\d{5,6}\b"
)
CASE_ID_RE = re.compile(r"\bCASE[-_/]?[A-Z0-9]{6,}\b", re.IGNORECASE)
CLIENT_ID_RE = re.compile(r"\bCLIENT[-_/]?[A-Z0-9]{4,}\b", re.IGNORECASE)
BANK_ACCOUNT_RE = re.compile(r"\b(?:ACCT|ACCOUNT)[-_/]?[A-Z0-9]{4,}\b", re.IGNORECASE)
PAN_RE = re.compile(r"\b[A-Z]{5}\d{4}[A-Z]\b")
DOB_RE = re.compile(
    r"\b(?:DOB|Date of Birth)\s*[:#-]?\s*(?:\d{2}[/-]\d{2}[/-]\d{4}|[A-Za-z]+\s+\d{1,2},\s*\d{4})\b",
    re.IGNORECASE,
)

FALSE_POSITIVE_PATTERNS = [
    ("May", re.compile(r"\bMay\b"), "PERSON"),
    ("Rose", re.compile(r"\bRose\b"), "PERSON"),
    ("Ray", re.compile(r"\bRay\b"), "PERSON"),
    ("Notice", re.compile(r"\bNotice\b", re.IGNORECASE), "ORGANIZATION_CONTACT"),
    ("Party", re.compile(r"\bParty\b", re.IGNORECASE), "ORGANIZATION_CONTACT"),
    ("Section", re.compile(r"\bSection\b", re.IGNORECASE), "ORGANIZATION_CONTACT"),
]

PII_PROFILES = [
    {
        "name": "Ananya Raman",
        "email": "ananya.raman@example.test",
        "phone": "+91 98765 43210",
        "address": "14 MG Road, Coimbatore, Tamil Nadu 641001",
        "case_id": "CASE-2026-CIV-0198",
        "client_id": "CLIENT-2026-0142",
        "bank_account": "ACCT-TEST-4831",
        "pan": "ABCDE1234F",
        "dob": "Date of Birth: 14/05/1991",
        "organization_contact": "Meridian Review Desk",
    },
    {
        "name": "Karthik Iyer",
        "email": "karthik.iyer@example.test",
        "phone": "+91 91234 56780",
        "address": "22 Race Course Road, Kochi, Kerala 682011",
        "case_id": "CASE-2026-ARB-0241",
        "client_id": "CLIENT-2026-0274",
        "bank_account": "ACCT-TEST-4832",
        "pan": "PQRSX2345K",
        "dob": "Date of Birth: 03/09/1988",
        "organization_contact": "Harbor Notice Team",
    },
    {
        "name": "Meera Nair",
        "email": "meera.nair@example.test",
        "phone": "+91 90123 45678",
        "address": "8 Cathedral Garden Road, Chennai, Tamil Nadu 600086",
        "case_id": "CASE-2026-COM-0314",
        "client_id": "CLIENT-2026-0388",
        "bank_account": "ACCT-TEST-4833",
        "pan": "LMNOP6789Q",
        "dob": "Date of Birth: 21/11/1992",
        "organization_contact": "Blue Lotus Contact Cell",
    },
    {
        "name": "Arjun Menon",
        "email": "arjun.menon@example.test",
        "phone": "+91 99887 66554",
        "address": "31 Residency Road, Bengaluru, Karnataka 560025",
        "case_id": "CASE-2026-LAB-0412",
        "client_id": "CLIENT-2026-0456",
        "bank_account": "ACCT-TEST-4834",
        "pan": "RSTUV4321M",
        "dob": "Date of Birth: 08/02/1990",
        "organization_contact": "Cedar Escalation Office",
    },
]

ANCHOR_PATTERNS = [
    re.compile(r"\bnotice\b", re.IGNORECASE),
    re.compile(r"\bcontact\b", re.IGNORECASE),
    re.compile(r"\bsignature\b", re.IGNORECASE),
    re.compile(r"\bparty\b", re.IGNORECASE),
    re.compile(r"\baddress\b", re.IGNORECASE),
]


@dataclass
class RegexCandidate:
    pii_type: str
    start: int
    end: int
    confidence: float
    reason: str


@dataclass
class DraftRedaction:
    start: int
    end: int
    text: str
    pii_type: str
    confidence: float
    reason: str
    source: str
    suggested_status: str
    is_ground_truth: bool


def repo_root() -> Path:
    return Path(__file__).resolve().parents[2]


def sha256_text(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def stable_rank(document_ids: list[str], salt: str) -> list[str]:
    return sorted(document_ids, key=lambda document_id: sha256_text(f"{DETERMINISTIC_SEED}:{salt}:{document_id}"))


def load_documents(path: Path) -> tuple[list[dict[str, object]], str]:
    raw = path.read_bytes()
    sha = hashlib.sha256(raw).hexdigest()
    documents = []
    for line_number, line in enumerate(raw.decode("utf-8").splitlines(), start=1):
        document = json.loads(line)
        for key in ("id", "title", "source", "source_file", "text", "char_count"):
            if key not in document:
                raise ValueError(f"input line {line_number} missing key {key}")
        documents.append(document)
    if len(documents) < 200:
        raise ValueError(f"expected at least 200 documents, found {len(documents)}")
    return documents, sha


def phone_is_plausible(match_text: str) -> bool:
    digits = "".join(character for character in match_text if character.isdigit())
    return 9 <= len(digits) <= 15


def scan_regex_candidates(text: str) -> list[RegexCandidate]:
    candidates: list[RegexCandidate] = []

    for match in EMAIL_RE.finditer(text):
        candidates.append(
            RegexCandidate(
                pii_type="EMAIL",
                start=match.start(),
                end=match.end(),
                confidence=0.63,
                reason="Email-like token found by conservative regex scan",
            )
        )

    for match in PHONE_RE.finditer(text):
        if phone_is_plausible(match.group(0)):
            candidates.append(
                RegexCandidate(
                    pii_type="PHONE",
                    start=match.start(),
                    end=match.end(),
                    confidence=0.57,
                    reason="Phone-like number found by conservative regex scan",
                )
            )

    for pattern, pii_type, confidence, reason in (
        (ADDRESS_RE, "ADDRESS", 0.56, "Address-like phrase found by regex scan"),
        (CASE_ID_RE, "CASE_ID", 0.66, "Case-like identifier found by regex scan"),
        (CLIENT_ID_RE, "CLIENT_ID", 0.66, "Client-like identifier found by regex scan"),
        (BANK_ACCOUNT_RE, "BANK_ACCOUNT", 0.58, "Account-like token found by regex scan"),
        (PAN_RE, "PAN_LIKE_ID", 0.69, "PAN-like token found by regex scan"),
        (DOB_RE, "DATE_OF_BIRTH", 0.53, "Date-of-birth phrase found by regex scan"),
    ):
        for match in pattern.finditer(text):
            candidates.append(
                RegexCandidate(
                    pii_type=pii_type,
                    start=match.start(),
                    end=match.end(),
                    confidence=confidence,
                    reason=reason,
                )
            )

    candidates.sort(key=lambda candidate: (candidate.start, candidate.end, candidate.pii_type))
    return candidates


def summarize_exploration(documents: list[dict[str, object]]) -> tuple[dict[str, object], dict[str, list[RegexCandidate]]]:
    lengths = [len(document["text"]) for document in documents]
    counts_by_type = {
        "EMAIL": 0,
        "PHONE": 0,
        "ADDRESS": 0,
        "CASE_ID": 0,
        "CLIENT_ID": 0,
        "BANK_ACCOUNT": 0,
        "PAN_LIKE_ID": 0,
        "DATE_OF_BIRTH": 0,
    }
    regex_candidates_by_document: dict[str, list[RegexCandidate]] = {}
    sample_metadata: list[dict[str, object]] = []

    for document in documents:
        document_id = str(document["id"])
        candidates = scan_regex_candidates(str(document["text"]))
        regex_candidates_by_document[document_id] = candidates
        for candidate in candidates:
            counts_by_type[candidate.pii_type] += 1
            if len(sample_metadata) < 12:
                sample_metadata.append(
                    {
                        "document_id": document_id,
                        "type": candidate.pii_type,
                        "start": candidate.start,
                        "end": candidate.end,
                        "length": candidate.end - candidate.start,
                        "confidence": candidate.confidence,
                    }
                )

    documents_with_candidates = sum(1 for candidates in regex_candidates_by_document.values() if candidates)
    summary = {
        "documents_scanned": len(documents),
        "document_length_stats": {
            "min": min(lengths),
            "max": max(lengths),
            "mean": round(statistics.mean(lengths), 2),
            "median": statistics.median(lengths),
        },
        "documents_with_regex_candidates": documents_with_candidates,
        "regex_candidate_counts_by_type": counts_by_type,
        "sample_candidate_metadata": sample_metadata,
        "notes": "Existing CUAD text contains some useful email-like and phone-like candidates, but they are treated only as heuristic review signals and not as PII ground truth.",
    }
    return summary, regex_candidates_by_document


def choose_documents(documents: list[dict[str, object]], regex_candidates_by_document: dict[str, list[RegexCandidate]]) -> dict[str, set[str]]:
    document_ids = [str(document["id"]) for document in documents]
    failed_ids = set(stable_rank(document_ids, "failed")[:FAILED_COUNT])

    candidate_doc_ids = [
        document_id
        for document_id in document_ids
        if document_id not in failed_ids and regex_candidates_by_document[document_id]
    ]
    review_ids = set(stable_rank(candidate_doc_ids, "review_regex_docs")[:REVIEW_REGEX_DOC_COUNT])

    remaining_for_review = [
        document_id
        for document_id in document_ids
        if document_id not in failed_ids and document_id not in review_ids
    ]
    review_fill_count = REVIEW_COUNT - len(review_ids)
    review_ids.update(stable_rank(remaining_for_review, "review_fill_docs")[:review_fill_count])

    ready_pool = [
        document_id
        for document_id in document_ids
        if document_id not in failed_ids and document_id not in review_ids
    ]
    review_injected_ids = set(stable_rank(sorted(review_ids), "review_injected")[:INJECTED_REVIEW_COUNT])
    ready_injected_ids = set(stable_rank(ready_pool, "ready_injected")[:INJECTED_READY_COUNT])
    injected_ids = review_injected_ids | ready_injected_ids

    clean_pool = [
        document_id
        for document_id in ready_pool
        if document_id not in injected_ids and not regex_candidates_by_document[document_id]
    ]
    if len(clean_pool) < CLEAN_COUNT:
        clean_pool = [document_id for document_id in ready_pool if document_id not in injected_ids]
    clean_ids = set(stable_rank(clean_pool, "clean")[:CLEAN_COUNT])

    missed_ids = set(stable_rank(sorted(review_injected_ids), "missed")[:MISSED_CASE_COUNT])
    overlap_pool = [document_id for document_id in sorted(review_injected_ids) if document_id not in missed_ids]
    overlap_ids = set(stable_rank(overlap_pool, "overlap")[:OVERLAP_CASE_COUNT])
    high_pii_pool = [
        document_id
        for document_id in sorted(review_injected_ids)
        if document_id not in missed_ids and document_id not in overlap_ids
    ]
    high_pii_ids = set(stable_rank(high_pii_pool, "high_pii")[:HIGH_PII_CASE_COUNT])
    false_positive_ids = set(stable_rank(sorted(review_ids), "false_positive")[:FALSE_POSITIVE_DOC_COUNT])

    return {
        "failed": failed_ids,
        "review": review_ids,
        "clean": clean_ids,
        "injected": injected_ids,
        "review_injected": review_injected_ids,
        "ready_injected": ready_injected_ids,
        "missed": missed_ids,
        "overlap": overlap_ids,
        "high_pii": high_pii_ids,
        "false_positive": false_positive_ids,
    }


def choose_profile(document_id: str) -> dict[str, str]:
    rank = int(sha256_text(f"{DETERMINISTIC_SEED}:profile:{document_id}")[:8], 16)
    return PII_PROFILES[rank % len(PII_PROFILES)]


def choose_anchor(text: str) -> int:
    for pattern in ANCHOR_PATTERNS:
        match = pattern.search(text)
        if not match:
            continue
        line_end = text.find("\n", match.end())
        return len(text) if line_end == -1 else line_end + 1
    return min(len(text), max(0, len(text) - 1))


def build_injection_block(document_id: str, selections: dict[str, set[str]]) -> tuple[str, list[DraftRedaction]]:
    profile = choose_profile(document_id)
    fragments: list[str] = []
    drafts: list[DraftRedaction] = []
    cursor = 0

    def append_plain(value: str) -> None:
        nonlocal cursor
        fragments.append(value)
        cursor += len(value)

    def append_field(
        label: str,
        value: str,
        pii_type: str,
        confidence: float,
        reason: str,
        source: str,
        suggested_status: str,
        is_ground_truth: bool,
    ) -> None:
        nonlocal cursor
        append_plain(label)
        start = cursor
        append_plain(value)
        end = cursor
        drafts.append(
            DraftRedaction(
                start=start,
                end=end,
                text=value,
                pii_type=pii_type,
                confidence=confidence,
                reason=reason,
                source=source,
                suggested_status=suggested_status,
                is_ground_truth=is_ground_truth,
            )
        )
        append_plain("\n")

    append_plain("\n\nWorkflow contact note:\n")
    append_field(
        "Primary contact: ",
        profile["name"],
        "PERSON",
        0.96,
        "Synthetic person name inserted into notice/contact context",
        "synthetic_injection",
        "ACCEPTED",
        True,
    )
    append_field(
        "Email: ",
        profile["email"],
        "EMAIL",
        0.98,
        "Synthetic email inserted into notice/contact context",
        "synthetic_injection",
        "ACCEPTED",
        True,
    )
    append_field(
        "Phone: ",
        profile["phone"],
        "PHONE",
        0.97,
        "Synthetic phone inserted into notice/contact context",
        "synthetic_injection",
        "ACCEPTED",
        True,
    )

    if document_id in selections["high_pii"] or document_id in selections["ready_injected"]:
        append_field(
            "Client ID: ",
            profile["client_id"],
            "CLIENT_ID",
            0.94,
            "Synthetic client identifier inserted for batch workflow seeding",
            "synthetic_injection",
            "ACCEPTED",
            True,
        )

    if document_id in selections["high_pii"]:
        append_field(
            "Case ID: ",
            profile["case_id"],
            "CASE_ID",
            0.95,
            "Synthetic case identifier inserted for higher-count review scenario",
            "synthetic_injection",
            "ACCEPTED",
            True,
        )
        append_field(
            "Address: ",
            profile["address"],
            "ADDRESS",
            0.93,
            "Synthetic address inserted for higher-count review scenario",
            "synthetic_injection",
            "ACCEPTED",
            True,
        )

    if document_id in selections["overlap"]:
        label = "Organization contact: "
        phrase = f"{profile['name']}, {profile['organization_contact']}"
        append_plain(label)
        start = cursor
        append_plain(phrase)
        end = cursor
        name_end = start + len(profile["name"])
        drafts.append(
            DraftRedaction(
                start=start,
                end=name_end,
                text=profile["name"],
                pii_type="PERSON",
                confidence=0.95,
                reason="Synthetic person name inserted inside organization contact phrase",
                source="synthetic_injection",
                suggested_status="ACCEPTED",
                is_ground_truth=True,
            )
        )
        drafts.append(
            DraftRedaction(
                start=start,
                end=end,
                text=phrase,
                pii_type="ORGANIZATION_CONTACT",
                confidence=0.52,
                reason="Overlapping organization-contact span intentionally kept for review",
                source="synthetic_injection",
                suggested_status="REVIEW",
                is_ground_truth=True,
            )
        )
        append_plain("\n")

    if document_id in selections["missed"]:
        append_plain("Compliance note: ")
        start = cursor
        append_plain(profile["pan"])
        end = cursor
        drafts.append(
            DraftRedaction(
                start=start,
                end=end,
                text=profile["pan"],
                pii_type="PAN_LIKE_ID",
                confidence=0.33,
                reason="Synthetic PAN-like identifier intentionally surfaced as a possible missed PII warning",
                source="controlled_missed_pii",
                suggested_status="REVIEW",
                is_ground_truth=False,
            )
        )
        append_plain("\n")

    return "".join(fragments), drafts


def shift_range(start: int, end: int, anchor: int, delta: int) -> tuple[int, int]:
    if start >= anchor:
        return start + delta, end + delta
    return start, end


def pick_false_positive(text: str) -> tuple[int, int, str, str] | None:
    for label, pattern, pii_type in FALSE_POSITIVE_PATTERNS:
        match = pattern.search(text)
        if match:
            return match.start(), match.end(), label, pii_type
    return None


def write_jsonl(path: Path, records: list[dict[str, object]]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8", newline="\n") as output_file:
        for record in records:
            output_file.write(json.dumps(record, ensure_ascii=False) + "\n")


def validate_outputs(
    original_sha: str,
    documents_seed: list[dict[str, object]],
    redactions: list[dict[str, object]],
    input_path: Path,
) -> None:
    current_sha = hashlib.sha256(input_path.read_bytes()).hexdigest()
    if current_sha != original_sha:
        raise ValueError("input dataset/processed/cuad_contracts.jsonl was modified during generation")

    document_ids = set()
    for document in documents_seed:
        document_id = str(document["id"])
        if document_id in document_ids:
            raise ValueError(f"duplicate document id {document_id}")
        document_ids.add(document_id)

    if len(document_ids) < 200:
        raise ValueError(f"expected at least 200 output documents, found {len(document_ids)}")

    redaction_ids = set()
    documents_by_id = {str(document["id"]): document for document in documents_seed}
    for redaction in redactions:
        redaction_id = str(redaction["id"])
        if redaction_id in redaction_ids:
            raise ValueError(f"duplicate redaction id {redaction_id}")
        redaction_ids.add(redaction_id)

        document_id = str(redaction["document_id"])
        if document_id not in documents_by_id:
            raise ValueError(f"redaction {redaction_id} references missing document {document_id}")

        document_text = str(documents_by_id[document_id]["text"])
        start = int(redaction["start"])
        end = int(redaction["end"])
        if start < 0 or end < 0 or start >= end or end > len(document_text):
            raise ValueError(f"redaction {redaction_id} has out-of-bounds span")
        if str(redaction["text"]) != document_text[start:end]:
            raise ValueError(f"redaction {redaction_id} text does not match document span")

    synthetic_ground_truth = sum(1 for redaction in redactions if redaction["source"] == "synthetic_injection" and redaction["is_ground_truth"])
    review_docs = sum(1 for document in documents_seed if document["workflow_hint"] == "NEEDS_REVIEW")
    failed_docs = sum(1 for document in documents_seed if document["workflow_hint"] == "FAILED")

    if synthetic_ground_truth == 0:
        raise ValueError("expected at least one synthetic ground-truth redaction")
    if review_docs == 0:
        raise ValueError("expected at least one NEEDS_REVIEW document")
    if failed_docs == 0:
        raise ValueError("expected at least one FAILED document hint")


def main() -> int:
    root = repo_root()
    input_path = root / INPUT_JSONL
    output_documents_path = root / OUTPUT_DOCUMENTS
    output_redactions_path = root / OUTPUT_REDACTIONS
    output_manifest_path = root / OUTPUT_MANIFEST
    output_exploration_path = root / OUTPUT_EXPLORATION

    documents, original_sha = load_documents(input_path)
    exploration_summary, regex_candidates_by_document = summarize_exploration(documents)
    if exploration_summary["documents_with_regex_candidates"] == 0:
        raise ValueError(
            "no useful regex candidates were found in CUAD text; stop and ask the user before generating broad fake suggestions"
        )

    selections = choose_documents(documents, regex_candidates_by_document)

    documents_seed: list[dict[str, object]] = []
    redactions: list[dict[str, object]] = []
    manifest_document_hints: list[dict[str, object]] = []
    redaction_sequence = 1

    for document in documents:
        document_id = str(document["id"])
        original_text = str(document["text"])
        anchor = len(original_text)
        injected_block = ""
        injected_drafts: list[DraftRedaction] = []

        if document_id in selections["injected"]:
            anchor = choose_anchor(original_text)
            injected_block, injected_drafts = build_injection_block(document_id, selections)
            seeded_text = original_text[:anchor] + injected_block + original_text[anchor:]
            synthetic_pii_injected = True
        else:
            seeded_text = original_text
            synthetic_pii_injected = False

        document_status = "READY"
        risk_level = "LOW"
        reasons: list[str] = []
        failure_reason = None

        if document_id in selections["failed"]:
            document_status = "FAILED"
            risk_level = "HIGH"
            failure_reason = "SIMULATED_DETECTION_TIMEOUT"
            reasons.append("simulated_failure_hint")
        elif document_id in selections["review"]:
            document_status = "NEEDS_REVIEW"
            risk_level = "MEDIUM"
        elif document_id in selections["clean"]:
            document_status = "CLEAN"
            reasons.append("no_mock_suggestions")
        else:
            document_status = "READY"

        shifted_regex_candidates: list[RegexCandidate] = []
        for candidate in regex_candidates_by_document[document_id]:
            start, end = shift_range(candidate.start, candidate.end, anchor, len(injected_block))
            shifted_regex_candidates.append(
                RegexCandidate(
                    pii_type=candidate.pii_type,
                    start=start,
                    end=end,
                    confidence=candidate.confidence,
                    reason=candidate.reason,
                )
            )

        for draft in injected_drafts:
            record = {
                "id": f"red_{redaction_sequence:06d}",
                "document_id": document_id,
                "start": anchor + draft.start,
                "end": anchor + draft.end,
                "text": draft.text,
                "type": draft.pii_type,
                "confidence": draft.confidence,
                "reason": draft.reason,
                "source": draft.source,
                "suggested_status": draft.suggested_status,
                "is_ground_truth": draft.is_ground_truth,
            }
            redaction_sequence += 1
            redactions.append(record)
            if draft.source == "controlled_missed_pii":
                reasons.append("controlled_missed_pii_warning")

        if document_id in selections["review"]:
            for candidate in shifted_regex_candidates[:2]:
                record = {
                    "id": f"red_{redaction_sequence:06d}",
                    "document_id": document_id,
                    "start": candidate.start,
                    "end": candidate.end,
                    "text": seeded_text[candidate.start:candidate.end],
                    "type": candidate.pii_type,
                    "confidence": candidate.confidence,
                    "reason": candidate.reason,
                    "source": "regex_candidate",
                    "suggested_status": "REVIEW",
                    "is_ground_truth": False,
                }
                redaction_sequence += 1
                redactions.append(record)
                reasons.append("regex_candidate_present")

        if document_id in selections["false_positive"]:
            false_positive = pick_false_positive(seeded_text)
            if false_positive:
                start, end, label, pii_type = false_positive
                record = {
                    "id": f"red_{redaction_sequence:06d}",
                    "document_id": document_id,
                    "start": start,
                    "end": end,
                    "text": seeded_text[start:end],
                    "type": pii_type,
                    "confidence": 0.28,
                    "reason": f"Harmless legal term '{label}' intentionally flagged as a controlled false positive",
                    "source": "controlled_false_positive",
                    "suggested_status": "REVIEW",
                    "is_ground_truth": False,
                }
                redaction_sequence += 1
                redactions.append(record)
                reasons.append("controlled_false_positive")

        synthetic_count = sum(
            1
            for redaction in redactions
            if redaction["document_id"] == document_id and redaction["source"] == "synthetic_injection"
        )
        regex_count = sum(
            1
            for redaction in redactions
            if redaction["document_id"] == document_id and redaction["source"] == "regex_candidate"
        )

        if document_id in selections["high_pii"]:
            reasons.append("unusual_high_pii_count")
            risk_level = "HIGH"
        if document_id in selections["overlap"]:
            reasons.append("overlapping_spans")
            risk_level = "HIGH"
        if document_id in selections["missed"]:
            risk_level = "HIGH"
        if document_status == "NEEDS_REVIEW" and regex_count > 0:
            reasons.append("low_confidence_candidate_review")
        if synthetic_count >= 5:
            reasons.append("bulk_review_exception")

        document_record = {
            "id": document_id,
            "title": document["title"],
            "source": document["source"],
            "source_file": document["source_file"],
            "text": seeded_text,
            "char_count": len(seeded_text),
            "synthetic_pii_injected": synthetic_pii_injected,
            "workflow_hint": document_status,
            "risk_level_hint": risk_level,
        }
        if failure_reason:
            document_record["failure_hint"] = failure_reason
        documents_seed.append(document_record)

        manifest_document_hints.append(
            {
                "document_id": document_id,
                "status": document_status,
                "risk_level": risk_level,
                "synthetic_pii_injected": synthetic_pii_injected,
                "redaction_suggestion_count": sum(1 for redaction in redactions if redaction["document_id"] == document_id),
                "synthetic_ground_truth_count": sum(
                    1
                    for redaction in redactions
                    if redaction["document_id"] == document_id
                    and redaction["source"] == "synthetic_injection"
                    and redaction["is_ground_truth"]
                ),
                "regex_candidate_count": regex_count,
                "failure_hint": failure_reason,
                "reason_codes": sorted(set(reasons)),
            }
        )

    validate_outputs(original_sha, documents_seed, redactions, input_path)

    write_jsonl(output_documents_path, documents_seed)
    write_jsonl(output_redactions_path, redactions)

    counts_by_type: dict[str, int] = {}
    for redaction in redactions:
        counts_by_type[str(redaction["type"])] = counts_by_type.get(str(redaction["type"]), 0) + 1

    status_counts: dict[str, int] = {}
    for document in documents_seed:
        status = str(document["workflow_hint"])
        status_counts[status] = status_counts.get(status, 0) + 1

    manifest = {
        "input_file": str(INPUT_JSONL),
        "output_files": [
            str(OUTPUT_DOCUMENTS),
            str(OUTPUT_REDACTIONS),
            str(OUTPUT_MANIFEST),
            str(OUTPUT_EXPLORATION),
        ],
        "generation_timestamp": datetime.now(timezone.utc).isoformat(),
        "deterministic_seed_value": DETERMINISTIC_SEED,
        "input_document_count": len(documents),
        "output_document_count": len(documents_seed),
        "total_redaction_suggestions": len(redactions),
        "total_synthetic_ground_truth_spans": sum(
            1
            for redaction in redactions
            if redaction["source"] == "synthetic_injection" and redaction["is_ground_truth"]
        ),
        "total_regex_candidates": sum(1 for redaction in redactions if redaction["source"] == "regex_candidate"),
        "total_controlled_false_positives": sum(
            1 for redaction in redactions if redaction["source"] == "controlled_false_positive"
        ),
        "total_controlled_missed_pii_cases": sum(
            1 for redaction in redactions if redaction["source"] == "controlled_missed_pii"
        ),
        "total_failed_document_hints": status_counts.get("FAILED", 0),
        "counts_by_pii_type": counts_by_type,
        "counts_by_suggested_document_status": status_counts,
        "existing_candidate_notes": exploration_summary["notes"],
        "input_sha256": original_sha,
        "document_hints": manifest_document_hints,
    }

    output_manifest_path.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    output_exploration_path.write_text(json.dumps(exploration_summary, indent=2) + "\n", encoding="utf-8")

    print(f"[generate_mock_redactions] input documents: {len(documents)}")
    print(f"[generate_mock_redactions] output documents: {len(documents_seed)}")
    print(f"[generate_mock_redactions] redaction suggestions: {len(redactions)}")
    print(
        "[generate_mock_redactions] status counts: "
        + ", ".join(f"{status}={count}" for status, count in sorted(status_counts.items()))
    )
    print(
        "[generate_mock_redactions] source counts: "
        + ", ".join(
            f"{source}={sum(1 for redaction in redactions if redaction['source'] == source)}"
            for source in ("synthetic_injection", "regex_candidate", "controlled_false_positive", "controlled_missed_pii")
        )
    )
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:  # noqa: BLE001
        print(f"[generate_mock_redactions] error: {exc}", file=sys.stderr)
        raise
