#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"
FIXTURE_DIR="$ROOT_DIR/dataset/raw/manual_synthetic_txt"
BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
SERVER_LOG="$(mktemp)"

SERVER_PID=""

cleanup() {
  if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  rm -f "$SERVER_LOG"
}
trap cleanup EXIT

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  if [[ -f "$SERVER_LOG" ]]; then
    printf '\nServer log:\n' >&2
    cat "$SERVER_LOG" >&2
  fi
  exit 1
}

note() {
  printf '%s\n' "$1"
}

assert_jq() {
  local body="$1"
  local expr="$2"
  local message="$3"
  shift 3
  if ! jq -e "$@" "$expr" >/dev/null <<<"$body"; then
    printf 'Body:\n%s\n' "$body" >&2
    fail "$message"
  fi
}

request() {
  local method="$1"
  local url="$2"
  local expected_status="$3"
  shift 3

  local response
  response="$(curl -sS -X "$method" "$url" "$@" -w $'\n%{http_code}')"
  HTTP_BODY="$(printf '%s' "$response" | sed '$d')"
  HTTP_STATUS="$(printf '%s' "$response" | tail -n1)"

  if [[ "$HTTP_STATUS" != "$expected_status" ]]; then
    printf 'Unexpected status for %s %s: got %s want %s\n' "$method" "$url" "$HTTP_STATUS" "$expected_status" >&2
    printf 'Body:\n%s\n' "$HTTP_BODY" >&2
    fail "request failed"
  fi
}

start_server() {
  (
    cd "$BACKEND_DIR/cmd/server"
    env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-modcache go run .
  ) >"$SERVER_LOG" 2>&1 &
  SERVER_PID="$!"
}

wait_for_server() {
  local attempt
  for attempt in $(seq 1 60); do
    if curl -sS "$BASE_URL/healthz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.25
  done
  fail "server did not become ready"
}

wait_for_processing() {
  local attempt
  for attempt in $(seq 1 120); do
    request GET "$BASE_URL/api/batch/summary" 200
    if jq -e '.total_documents == 3 and .queued == 0 and .processing == 0 and (.ready + .clean) == 3' >/dev/null <<<"$HTTP_BODY"; then
      return 0
    fi
    sleep 0.25
  done
  fail "documents did not finish processing"
}

note "starting backend server"
start_server
wait_for_server

note "checking /healthz"
request GET "$BASE_URL/healthz" 200
assert_jq "$HTTP_BODY" '.status == "ok"' "healthz payload mismatch"

note "checking initial export/latest"
request GET "$BASE_URL/api/export/latest" 200
assert_jq "$HTTP_BODY" '.has_export == false' "expected no export before test"

note "checking initial batch summary"
request GET "$BASE_URL/api/batch/summary" 200
assert_jq "$HTTP_BODY" '.total_documents == 0 and .total_redactions == 0' "expected empty summary"

note "uploading fixtures"
request POST "$BASE_URL/api/uploads/documents" 200 \
  -F mode=replace \
  -F "files=@$FIXTURE_DIR/personal_info.txt" \
  -F "files=@$FIXTURE_DIR/clean.txt" \
  -F "files=@$FIXTURE_DIR/legal_document.txt"
assert_jq "$HTTP_BODY" '.uploaded == 3 and .accepted == 3 and .documents_created == 3 and (.items | length) == 3' "upload result mismatch"

PERSONAL_ID="$(jq -r '.items[] | select(.filename == "personal_info.txt") | .document_id' <<<"$HTTP_BODY")"
CLEAN_ID="$(jq -r '.items[] | select(.filename == "clean.txt") | .document_id' <<<"$HTTP_BODY")"
LEGAL_ID="$(jq -r '.items[] | select(.filename == "legal_document.txt") | .document_id' <<<"$HTTP_BODY")"
[[ -n "$PERSONAL_ID" && -n "$CLEAN_ID" && -n "$LEGAL_ID" ]] || fail "missing uploaded document ids"

note "waiting for async processing"
wait_for_processing

note "checking document list"
request GET "$BASE_URL/api/documents" 200
assert_jq "$HTTP_BODY" '.total == 3 and (.items | length) == 3' "document list mismatch"

note "checking single document"
request GET "$BASE_URL/api/documents/$CLEAN_ID" 200
assert_jq "$HTTP_BODY" '.id == $id and (.status == "CLEAN" or .status == "READY" or .status == "APPROVED")' "document detail mismatch" --arg id "$CLEAN_ID"

note "checking document redactions"
request GET "$BASE_URL/api/documents/$PERSONAL_ID/redactions" 200
assert_jq "$HTTP_BODY" '.document_id == $id and .total >= 1 and (.items | length) >= 1' "redaction list mismatch" --arg id "$PERSONAL_ID"
RED_1="$(jq -r '.items[0].id' <<<"$HTTP_BODY")"
RED_2="$(jq -r '.items[1].id' <<<"$HTTP_BODY")"
RED_3="$(jq -r '.items[2].id' <<<"$HTTP_BODY")"
RED_4="$(jq -r '.items[3].id' <<<"$HTTP_BODY")"
RED_5="$(jq -r '.items[4].id' <<<"$HTTP_BODY")"
RED_6="$(jq -r '.items[5].id' <<<"$HTTP_BODY")"

note "checking review summary"
request GET "$BASE_URL/api/documents/$PERSONAL_ID/review-summary" 200
assert_jq "$HTTP_BODY" '.document_id == $id and .total_redactions >= 1' "review summary mismatch" --arg id "$PERSONAL_ID"

note "adding manual redaction"
request POST "$BASE_URL/api/documents/$CLEAN_ID/redactions" 200 \
  -H 'Content-Type: application/json' \
  --data '{"start":0,"end":5,"type":"PERSON","reason":"smoke test"}'
assert_jq "$HTTP_BODY" '.document_id == $id and .review_state == "ADDED" and .is_user_added == true' "manual redaction mismatch" --arg id "$CLEAN_ID"
MANUAL_RED_ID="$(jq -r '.id' <<<"$HTTP_BODY")"

note "accepting one redaction"
request POST "$BASE_URL/api/redactions/$RED_1/accept" 200
assert_jq "$HTTP_BODY" '.redaction_id == $id' "accept response mismatch" --arg id "$RED_1"

note "rejecting one redaction"
request POST "$BASE_URL/api/redactions/$RED_2/reject" 200
assert_jq "$HTTP_BODY" '.redaction_id == $id and .review_state == "REJECTED"' "reject response mismatch" --arg id "$RED_2"

note "bulk-accepting redactions"
request POST "$BASE_URL/api/redactions/bulk-accept" 200 \
  -H 'Content-Type: application/json' \
  --data "{\"redaction_ids\":[\"$RED_3\",\"$RED_4\"]}"
assert_jq "$HTTP_BODY" '.requested == 2 and (.items | length) == 2' "bulk accept mismatch"

note "bulk-rejecting redactions"
request POST "$BASE_URL/api/redactions/bulk-reject" 200 \
  -H 'Content-Type: application/json' \
  --data "{\"redaction_ids\":[\"$RED_5\",\"$RED_6\"]}"
assert_jq "$HTTP_BODY" '.requested == 2 and .rejected == 2 and (.items | length) == 2' "bulk reject mismatch"

note "approving one document"
request POST "$BASE_URL/api/documents/$CLEAN_ID/approve" 200
assert_jq "$HTTP_BODY" '.document_id == $id and .status == "APPROVED"' "single approve mismatch" --arg id "$CLEAN_ID"

note "bulk-approving documents"
request POST "$BASE_URL/api/documents/bulk-approve" 200 \
  -H 'Content-Type: application/json' \
  --data "{\"document_ids\":[\"$PERSONAL_ID\",\"$LEGAL_ID\",\"$CLEAN_ID\"]}"
assert_jq "$HTTP_BODY" '.requested == 3 and (.approved >= 2) and (.items | length) == 3' "bulk approve mismatch"

note "retrying approved document"
request POST "$BASE_URL/api/documents/$CLEAN_ID/retry" 200
assert_jq "$HTTP_BODY" '.document_id == $id and .changed == false' "retry response mismatch" --arg id "$CLEAN_ID"

note "bulk-retrying mixed documents"
request POST "$BASE_URL/api/documents/bulk-retry" 200 \
  -H 'Content-Type: application/json' \
  --data "{\"document_ids\":[\"$CLEAN_ID\",\"missing_doc\"]}"
assert_jq "$HTTP_BODY" '.requested == 2 and ((.retried // 0) == 0) and (.items | length) == 2' "bulk retry mismatch"

note "exporting approved documents"
request POST "$BASE_URL/api/export" 200
assert_jq "$HTTP_BODY" '.exported_documents == 3 and (.files | length) == 3' "export mismatch"

note "checking latest export"
request GET "$BASE_URL/api/export/latest" 200
assert_jq "$HTTP_BODY" '.has_export == true and .exported_documents == 3 and (.files | length) == 3' "latest export mismatch"

note "checking final batch summary"
request GET "$BASE_URL/api/batch/summary" 200
assert_jq "$HTTP_BODY" '.total_documents == 3 and .exported == 3' "final summary mismatch"

note "all endpoints passed"
