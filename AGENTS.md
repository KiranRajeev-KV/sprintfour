# AGENTS.md

## Project decision

Build the Sprintfour Hackathon Problem 2 product: Working at volume.

The user is Maya, a paralegal who has 200 case files to anonymize before end of day. The product must optimize for throughput, triage, partial failure recovery, and fast batch approval. Do not design this as a one-document-at-a-time redaction editor.

Primary product thesis:

Maya needs a batch command center that processes the safe majority quickly and routes only risky, uncertain, or failed documents into review.

The hackathon requires a runnable full-stack application with a real frontend and backend. PII detection is not the core scoring area. Start with a mock backend for PII detections because the handout explicitly allows mock detections and does not score cloud LLM detection higher than mock detection.

## Final stack choices

Frontend:

- Vite + React + TypeScript.
- Bun as the package manager and script runner.
- TanStack Query for server state, mutations, cache invalidation, retries, and loading/error state.
- TanStack Router for routing and URL/search-param driven UI state.
- Zod for runtime validation of API responses, route search params, and form/action inputs.
- TanStack Table for the batch document table.
- shadcn/ui Data Table pattern and shadcn/ui components for the UI.
- A rich text editor package may be used only after the user explicitly chooses the package. Do not select or install one independently.

Backend:

- Go.
- Gin Gonic for HTTP routing.
- Use Go standard library features wherever sufficient.
- Use `log/slog` for structured logging.
- Use `context.Context` for cancellation, timeouts, and request/job lifecycle control.
- Use goroutines, channels, and bounded worker pools for batch processing.
- SQLite may be used if persistence is needed.
- If SQLite is used, use sqlc for typed SQL access.
- Do not introduce Redis for the 8-hour MVP unless the user explicitly approves it.

PII detection path:

- Start with deterministic mock detections.
- Mock detector returns spans, PII type, confidence, and reason.
- Document-level risk score is computed from mock detector output.
- Low-confidence, high-risk, failed, or ambiguous documents are routed to `NEEDS_REVIEW`.
- AI/LLM fallback may be added later only for uncertain documents and only after the mock workflow works.
- AI output is treated as another suggestion source, not as truth.

## Dependency policy

Do not add new packages, libraries, frameworks, CLIs, UI kits, database tools, editor packages, queue systems, or validation libraries without explicit user permission.

Before touching code that uses a new library or package:

1. Look up the latest official documentation and best practices online.
2. Summarize the relevant choice.
3. Ask the user for approval.
4. Stop until the user decides.

If online access is unavailable, stop and ask the user to provide the docs or approve proceeding from existing knowledge.

Do not make product, architecture, persistence, AI-provider, package, deployment, or UI-library decisions independently. If a decision is required mid-implementation, stop and ask the user.

## 8-hour MVP scope

Build only the smallest product that proves the high-volume workflow.

Required MVP features:

- Seeded or mock batch of approximately 200 documents.
- Batch dashboard with counts:
  - total documents
  - queued
  - processing
  - ready
  - needs review
  - approved
  - failed
  - exported
- Batch document table with:
  - document title
  - status
  - risk level
  - PII count
  - low-confidence count
  - last error
  - selectable rows
- Filters for:
  - ready
  - needs review
  - approved
  - failed
  - high risk
- Bulk actions:
  - process batch
  - approve selected ready documents
  - retry failed documents
  - export approved documents
- Review detail view only for exception documents.
- Redacted preview for a selected document.
- Per-document suggested redactions with type, confidence, and reason.
- Export summary showing approved, skipped, needs-review, and failed counts.
- Partial failure recovery: failed documents must not block successfully processed documents.
- Idempotent backend actions.

The table/batch workflow is the primary product. The editor is secondary.

## UX rules

Optimize for Maya’s time.

The UI must always answer:

- What can be safely approved now?
- What needs attention?
- What failed?
- What is export-ready?
- How much work remains?

Do not force Maya to open all 200 documents.

Use exception-first triage:

- `READY` documents can be bulk-approved.
- `NEEDS_REVIEW` documents require focused review.
- `FAILED` documents can be retried.
- `APPROVED` documents are exportable.

Bulk approve must default to safe behavior:

- Approve only `READY` documents by default.
- Do not silently approve `NEEDS_REVIEW` documents.
- Show clear counts before and after bulk actions.

A Grammarly-like editor is allowed only as a review surface for `NEEDS_REVIEW` documents. Do not make it the central workflow.

Use stable document IDs for all table row selection and bulk actions. Never use visible row index as identity.

## Backend behavior rules

Use explicit state transitions.

Document statuses:

- `QUEUED`
- `PROCESSING`
- `READY`
- `NEEDS_REVIEW`
- `APPROVED`
- `FAILED`
- `EXPORTED`

Job statuses:

- `QUEUED`
- `RUNNING`
- `SUCCEEDED`
- `FAILED`
- `CANCELLED`

Backend actions must be idempotent:

- Processing the same batch twice must not duplicate jobs for already queued or processed documents.
- Approving an already-approved document should return success.
- Retrying a failed job should create a controlled new attempt, not duplicate active work.
- Exporting the same approved state should produce the same result.

Worker pool rules:

- Use bounded concurrency.
- Track attempts.
- Set max retry attempts.
- Store clear failure reasons.
- Support cancellation through context.
- Never let one failed document block the whole batch.

Rate limiting is not part of the MVP. Add rate limiting only for AI fallback calls if the user approves AI integration.

## Logging rules

Use structured logs with `log/slog`.

Log useful operational fields:

- request id if available
- batch id
- document id
- job id
- status transition
- duration
- attempt count
- error category

Never log raw document content.

Never log raw PII values.

Never log full redaction spans if they expose sensitive text. Use counts, types, offsets, and IDs instead.

Logging should help debug batch state, queue behavior, retries, and failures.

## Code quality rules

Keep code clear, small, and separated by responsibility.

Prefer simple, explicit code over clever abstractions.

Use concise comments only when they explain why a decision exists. Do not comment obvious syntax.

Use clear names for product concepts:

- batch
- document
- redaction
- detection
- risk
- review
- job
- export

Avoid vague names like `data`, `item`, `thing`, `manager`, or `processor` unless the scope is tiny and obvious.

Validate external inputs at boundaries.

Frontend validation:

- Use Zod for API response validation where useful.
- Use Zod for search params, filters, and forms/actions.

Backend validation:

- Validate request bodies.
- Return structured JSON errors.
- Do not expose internal stack traces or raw implementation errors to the frontend.

## Persistence rules

Start with in-memory state if that is enough for the 8-hour MVP.

Use SQLite only if persistence becomes necessary.

If SQLite is used:

- Use sqlc.
- Keep SQL explicit.
- Do not add an ORM.
- Do not add database migration tools without user approval.

Redis is intentionally not part of the MVP. Do not add Redis unless the user explicitly chooses it.

## AI rules

Do not start with AI detection.

First implement:

- mock detector
- risk scoring
- batch status transitions
- review queue
- export summary

Only after the core product works, ask the user whether to add AI fallback.

If AI fallback is approved:

- Run it only for uncertain or risky documents.
- Limit concurrency.
- Add timeouts.
- Validate structured output.
- Treat AI output as suggestions.
- Never let malformed AI output crash the batch.
- If AI fails, mark the document as `NEEDS_REVIEW`, not `READY`.

## Intentionally left out for MVP

Do not build these in the 8-hour version unless the user explicitly changes scope:

- Full PDF redaction engine.
- OCR.
- Image redaction.
- Authentication.
- User accounts.
- Multi-user collaboration.
- Real file upload pipeline.
- Redis-backed distributed queue.
- Production deployment hardening.
- Full audit/compliance system.
- Custom ML model.
- Full rich text editor as the main workflow.
- Real-time websocket updates.
- Complex permissions.
- Email notifications.
- Rate limiter for normal user actions.

These are excluded because they do not directly prove the core Problem 2 workflow: processing many documents quickly while focusing human attention on exceptions.

## Post-MVP / extended submission ideas

After the MVP works, possible additions are:

- AI fallback for uncertain documents.
- Keyboard shortcuts for high-volume review.
- Audit timeline for batch actions.
- Persisted batch state with SQLite.
- Redis-backed queue if reliability demonstration becomes important.
- ZIP export of redacted documents.
- Search across documents.
- Duplicate PII clustering across the batch.
- Better risk scoring explanation.
- Batch cancellation and resume.
- WebSocket or SSE progress updates.
- E2E tests for the full batch flow.
- More polished video walkthrough.
- Half-page writeup explaining tradeoffs and intentionally excluded scope.

Ask the user before implementing any post-MVP feature.

## Verification expectations

Before finishing any coding task, run the relevant checks if commands exist.

For frontend work, prefer Bun scripts once configured:

- `bun install`
- `bun run dev`
- `bun run build`
- `bun run lint`
- `bun run typecheck`
- `bun test`

For backend work, prefer Go commands once configured:

- `go test ./...`
- `go vet ./...`
- `go run .`

Do not invent missing scripts silently. If scripts or commands are missing, ask the user before adding them.

## Completion criteria

A change is complete only when:

- The requested behavior works.
- The UI still supports the batch-first workflow.
- Risky documents are not silently approved.
- Failed documents do not block successful documents.
- Logs do not expose raw PII.
- No unapproved dependency was added.
- The user’s chosen stack and scope were respected.
- Any unresolved decision is surfaced clearly to the user instead of guessed.
