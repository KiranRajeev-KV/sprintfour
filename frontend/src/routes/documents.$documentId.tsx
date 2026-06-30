import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  AlertTriangle,
  ArrowLeft,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Highlighter,
  Keyboard,
  SearchCheck,
  ShieldAlert,
  ShieldCheck,
  ShieldEllipsis,
  XCircle,
} from "lucide-react";
import { PageAlert } from "#/components/page-alert";
import { RiskBadge } from "#/components/risk-badge";
import { StatusBadge } from "#/components/status-badge";
import { Button } from "#/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "#/components/ui/card";
import { Input } from "#/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "#/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "#/components/ui/tabs";
import {
  acceptRedaction,
  addManualRedaction,
  ApiError,
  approveDocument,
  documentDetailQueryOptions,
  documentRedactionsQueryOptions,
  documentReviewSummaryQueryOptions,
  rejectRedaction,
  reviewQueueQueryOptions,
} from "#/lib/api";
import {
  buildHighlightRenderModel,
  getRedactionTone,
  getSelectedDocumentRange,
  PII_TYPE_OPTIONS,
  sortRedactionsForReview,
  truncatePreview,
  type SelectedDocumentRange,
} from "#/lib/redaction-review";
import type {
  DocumentDetail,
  DocumentStatus,
  PIIType,
  Redaction,
  RedactionsResponse,
  ReviewSummary,
} from "#/lib/schemas";
import { cn } from "#/lib/utils";

type FeedbackState = {
  tone: "success" | "error";
  message: string;
};

type SuggestionsTabValue = "suggestions" | "stats";

export const Route = createFileRoute("/documents/$documentId")({
  loader: async ({ context, params }) => {
    await Promise.all([
      context.queryClient.ensureQueryData(
        documentDetailQueryOptions(params.documentId),
      ),
      context.queryClient.ensureQueryData(
        documentRedactionsQueryOptions(params.documentId),
      ),
      context.queryClient.ensureQueryData(
        documentReviewSummaryQueryOptions(params.documentId),
      ),
      context.queryClient.ensureQueryData(reviewQueueQueryOptions()),
    ]);

    return { documentId: params.documentId };
  },
  pendingComponent: DocumentDetailPending,
  errorComponent: ({ error }) => {
    const apiError =
      error instanceof ApiError
        ? error
        : new ApiError(500, "unknown", "request failed");
    return (
      <div className="page-wrap px-4 py-8 lg:px-0 lg:py-10">
        <PageAlert
          title={
            apiError.status === 404
              ? "Document not found"
              : "Unable to load review workspace"
          }
          message={
            apiError.status === 404
              ? "The requested document could not be found in the current batch."
              : "The backend returned an error while loading the review workspace."
          }
        />
      </div>
    );
  },
  component: DocumentReviewEditorPage,
});

function DocumentReviewEditorPage() {
  const queryClient = useQueryClient();
  const { documentId } = Route.useLoaderData();
  const navigate = Route.useNavigate();

  const [activeTab, setActiveTab] =
    useState<SuggestionsTabValue>("suggestions");
  const [selectedRedactionID, setSelectedRedactionID] = useState<string | null>(
    null,
  );
  const [selectedRange, setSelectedRange] =
    useState<SelectedDocumentRange | null>(null);
  const [selectedType, setSelectedType] = useState<PIIType>("PERSON");
  const [manualReason, setManualReason] = useState("");
  const [sidebarFeedback, setSidebarFeedback] = useState<FeedbackState | null>(
    null,
  );
  const [approveFeedback, setApproveFeedback] = useState<FeedbackState | null>(
    null,
  );
  const [showShortcuts, setShowShortcuts] = useState(false);

  const { data: document } = useSuspenseQuery(
    documentDetailQueryOptions(documentId),
  );
  const { data: redactions } = useSuspenseQuery(
    documentRedactionsQueryOptions(documentId),
  );
  const { data: reviewSummary } = useSuspenseQuery(
    documentReviewSummaryQueryOptions(documentId),
  );
  const { data: reviewQueue } = useSuspenseQuery(reviewQueueQueryOptions());

  const invalidateReviewQueries = useCallback(async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["document", documentId] }),
      queryClient.invalidateQueries({
        queryKey: ["document-redactions", documentId],
      }),
      queryClient.invalidateQueries({
        queryKey: ["document-review-summary", documentId],
      }),
      queryClient.invalidateQueries({ queryKey: ["batch-summary"] }),
      queryClient.invalidateQueries({ queryKey: ["documents"] }),
      queryClient.invalidateQueries({ queryKey: ["latest-export"] }),
    ]);
  }, [documentId, queryClient]);

  const sortedRedactions = useMemo(
    () => [...redactions.items].sort(sortRedactionsForReview),
    [redactions.items],
  );
  const selectedRedactionIndex = useMemo(
    () => sortedRedactions.findIndex((item) => item.id === selectedRedactionID),
    [selectedRedactionID, sortedRedactions],
  );
  const selectedRedaction =
    selectedRedactionIndex >= 0 ? sortedRedactions[selectedRedactionIndex] : null;
  const renderModel = useMemo(
    () => buildHighlightRenderModel(document.text, redactions.items),
    [document.text, redactions.items],
  );
  const reviewQueueIndex = useMemo(
    () => reviewQueue.items.findIndex((item) => item.id === documentId),
    [documentId, reviewQueue.items],
  );
  const previousQueueDocument =
    reviewQueueIndex > 0 ? reviewQueue.items[reviewQueueIndex - 1] : null;
  const nextQueueDocument =
    reviewQueueIndex >= 0 && reviewQueueIndex < reviewQueue.items.length - 1
      ? reviewQueue.items[reviewQueueIndex + 1]
      : null;

  useEffect(() => {
    if (sortedRedactions.length === 0) {
      setSelectedRedactionID(null);
      return;
    }
    setSelectedRedactionID((current) =>
      current && sortedRedactions.some((item) => item.id === current)
        ? current
        : sortedRedactions[0]!.id,
    );
  }, [sortedRedactions]);

  const goToNeedsReviewQueue = useCallback(() => {
    navigate({
      to: "/",
      search: {
        status: "NEEDS_REVIEW",
        risk: undefined,
        q: "",
        limit: 50,
        offset: 0,
      },
    });
  }, [navigate]);

  const goToQueueDocument = useCallback(
    (targetDocumentId: string | null) => {
      if (!targetDocumentId) {
        return;
      }
      navigate({
        to: "/documents/$documentId",
        params: { documentId: targetDocumentId },
      });
    },
    [navigate],
  );

  const selectRelativeRedaction = useCallback(
    (direction: 1 | -1) => {
      if (sortedRedactions.length === 0) {
        return;
      }
      const baseIndex = selectedRedactionIndex >= 0 ? selectedRedactionIndex : 0;
      const nextIndex = Math.min(
        sortedRedactions.length - 1,
        Math.max(0, baseIndex + direction),
      );
      const nextRedaction = sortedRedactions[nextIndex];
      if (nextRedaction) {
        setSelectedRedactionID(nextRedaction.id);
        setActiveTab("suggestions");
      }
    },
    [selectedRedactionIndex, sortedRedactions],
  );

  const reviewMutation = useMutation({
    mutationFn: ({
      redactionId,
      action,
    }: {
      redactionId: string;
      action: "accept" | "reject";
    }) =>
      action === "accept"
        ? acceptRedaction(redactionId)
        : rejectRedaction(redactionId),
    onSuccess: async (result, variables) => {
      setSidebarFeedback({
        tone: "success",
        message:
          variables.action === "accept"
            ? `Accepted ${result.redaction_id}.`
            : `Rejected ${result.redaction_id}.`,
      });
      setApproveFeedback(null);
      await invalidateReviewQueries();
    },
    onError: (error, variables) => {
      setSidebarFeedback({
        tone: "error",
        message:
          error instanceof ApiError
            ? error.message
            : `${variables.action === "accept" ? "Accept" : "Reject"} failed.`,
      });
    },
  });

  const addManualRedactionMutation = useMutation({
    mutationFn: () => {
      if (!selectedRange) {
        throw new ApiError(
          400,
          "invalid_selection",
          "Select document text before adding a redaction.",
        );
      }

      return addManualRedaction({
        documentId,
        start: selectedRange.start,
        end: selectedRange.end,
        type: selectedType,
        reason: manualReason,
        selectedText: selectedRange.text,
      });
    },
    onSuccess: async (result) => {
      setSidebarFeedback({
        tone: "success",
        message: `Added manual ${result.type.toLowerCase().replace("_", " ")} redaction.`,
      });
      setApproveFeedback(null);
      setSelectedRange(null);
      setManualReason("");
      setSelectedRedactionID(result.id);
      setActiveTab("suggestions");
      window.getSelection()?.removeAllRanges();
      await invalidateReviewQueries();
    },
    onError: (error) => {
      setSidebarFeedback({
        tone: "error",
        message:
          error instanceof ApiError ? error.message : "Add redaction failed.",
      });
    },
  });

  const approveMutation = useMutation({
    mutationFn: () => approveDocument(documentId),
    onSuccess: async (result) => {
      setApproveFeedback({
        tone: "success",
        message: result.changed
          ? "Document approved and removed from the exception queue."
          : `Document remains ${result.status.toLowerCase().replace("_", " ")}.`,
      });
      setSidebarFeedback(null);
      await invalidateReviewQueries();
    },
    onError: (error) => {
      setApproveFeedback({
        tone: "error",
        message:
          error instanceof ApiError
            ? error.message
            : "Approve failed. Please try again.",
      });
    },
  });

  const approveState = getApproveState(document.status, reviewSummary);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (isEditableTarget(event.target)) {
        return;
      }

      if (event.key === "?") {
        event.preventDefault();
        setShowShortcuts((current) => !current);
        return;
      }
      if (event.key === "Escape") {
        setShowShortcuts(false);
        return;
      }
      if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
        if (approveState.canApprove && !approveMutation.isPending) {
          event.preventDefault();
          setApproveFeedback(null);
          setSidebarFeedback(null);
          approveMutation.mutate();
        }
        return;
      }
      if (event.key === "u" || event.key === "U") {
        event.preventDefault();
        goToNeedsReviewQueue();
        return;
      }
      if (event.key === "J") {
        event.preventDefault();
        goToQueueDocument(nextQueueDocument?.id ?? null);
        return;
      }
      if (event.key === "K") {
        event.preventDefault();
        goToQueueDocument(previousQueueDocument?.id ?? null);
        return;
      }
      if (event.key === "j") {
        event.preventDefault();
        selectRelativeRedaction(1);
        return;
      }
      if (event.key === "k") {
        event.preventDefault();
        selectRelativeRedaction(-1);
        return;
      }
      if (event.key === "a" || event.key === "A") {
        if (
          selectedRedaction &&
          selectedRedaction.review_state !== "ACCEPTED" &&
          selectedRedaction.review_state !== "ADDED" &&
          !reviewMutation.isPending
        ) {
          event.preventDefault();
          setSidebarFeedback(null);
          setApproveFeedback(null);
          reviewMutation.mutate({
            redactionId: selectedRedaction.id,
            action: "accept",
          });
        }
        return;
      }
      if (event.key === "r" || event.key === "R") {
        if (
          selectedRedaction &&
          selectedRedaction.review_state !== "REJECTED" &&
          !reviewMutation.isPending
        ) {
          event.preventDefault();
          setSidebarFeedback(null);
          setApproveFeedback(null);
          reviewMutation.mutate({
            redactionId: selectedRedaction.id,
            action: "reject",
          });
        }
      }
    };

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [
    approveMutation,
    approveState.canApprove,
    goToNeedsReviewQueue,
    goToQueueDocument,
    nextQueueDocument?.id,
    previousQueueDocument?.id,
    reviewMutation,
    selectRelativeRedaction,
    selectedRedaction,
  ]);

  return (
    <main className="page-wrap min-h-[calc(100vh-2rem)] space-y-5 px-4 pt-12 pb-8 lg:px-0 lg:pt-14 lg:pb-10">
      <header className="space-y-4">
        <div className="flex flex-wrap items-center gap-3">
          <Link
            to="/"
            className="inline-flex items-center gap-2 rounded-full border border-white/60 bg-white/72 px-4 py-2 text-sm font-semibold text-[var(--sea-ink)] no-underline transition hover:bg-white"
          >
            <ArrowLeft className="size-4" />
            Back to batch dashboard
          </Link>
          <Link
            to="/"
            search={{
              status: "NEEDS_REVIEW",
              risk: undefined,
              q: "",
              limit: 50,
              offset: 0,
            }}
            className="inline-flex items-center gap-2 rounded-full border border-white/50 bg-white/58 px-4 py-2 text-sm text-[var(--sea-ink-soft)] no-underline transition hover:bg-white/75"
          >
            <SearchCheck className="size-4" />
            Return to Needs Review queue
          </Link>
          <button
            type="button"
            onClick={() => setShowShortcuts((current) => !current)}
            className="inline-flex items-center gap-2 rounded-full border border-white/50 bg-white/58 px-4 py-2 text-sm text-[var(--sea-ink-soft)] transition hover:bg-white/75"
          >
            <Keyboard className="size-4" />
            Shortcuts
          </button>
        </div>

        <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
          <div className="space-y-3">
            <div className="inline-flex items-center gap-2 rounded-full border border-white/55 bg-white/70 px-3 py-1 text-xs tracking-[0.16em] text-[var(--kicker)] uppercase">
              <ShieldEllipsis className="size-4" />
              Exception editor
            </div>
            <h1
              className="display-title max-w-6xl text-3xl leading-tight text-[var(--sea-ink)] md:text-4xl"
              title={document.title}
            >
              {document.title}
            </h1>
            <div className="flex flex-wrap items-center gap-2 text-sm text-[var(--sea-ink-soft)]">
              <span>{document.source_file}</span>
              <span className="rounded-full border border-white/50 bg-white/55 px-3 py-1">
                {document.char_count.toLocaleString()} chars
              </span>
              <span className="rounded-full border border-white/50 bg-white/55 px-3 py-1">
                {reviewSummary.blocking_review_items.toLocaleString()} blocking
                item
                {reviewSummary.blocking_review_items === 1 ? "" : "s"}
              </span>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <StatusBadge status={document.status} />
            <RiskBadge risk={document.risk_level} />
            <ApprovalStatePill
              status={document.status}
              reviewSummary={reviewSummary}
            />
          </div>
        </div>
      </header>

      {showShortcuts ? (
        <Card className="island-shell border-white/45 py-0">
          <CardContent className="grid gap-3 px-5 py-4 md:grid-cols-2 xl:grid-cols-4">
            <ShortcutHint keys="j / k" description="Next or previous suggestion" />
            <ShortcutHint keys="Shift+J / Shift+K" description="Next or previous Needs Review document" />
            <ShortcutHint keys="a / r" description="Accept or reject the selected suggestion" />
            <ShortcutHint keys="Ctrl/Cmd+Enter" description="Approve document when allowed" />
            <ShortcutHint keys="u" description="Return to the Needs Review queue" />
            <ShortcutHint keys="?" description="Toggle this shortcuts panel" />
          </CardContent>
        </Card>
      ) : null}

      {document.failure_hint ? (
        <PageAlert
          title="Failure hint present"
          message={document.failure_hint}
        />
      ) : null}

      <Card className="island-shell h-[calc(100vh-12rem)] min-h-[calc(100vh-12rem)] overflow-hidden border-white/45 py-0">
        <CardHeader className="shrink-0 border-b border-black/5 px-5 pt-5 pb-4">
          <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
            <div className="space-y-1 pt-1">
              <h2 className="text-2xl leading-[1.2] font-semibold text-[var(--sea-ink)]">
                Redaction review workspace
              </h2>
              <p className="pt-0.5 text-sm leading-6 text-[var(--sea-ink-soft)]">
                Resolve suggestions, add missed redactions, and approve only
                when the blocking count reaches zero.
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-2 text-xs text-[var(--sea-ink-soft)]">
              <span className="rounded-full border border-white/60 bg-white/60 px-3 py-1">
                {redactions.total.toLocaleString()} suggestions
              </span>
              {selectedRange ? (
                <span className="rounded-full border border-[var(--lagoon-deep)]/30 bg-[var(--lagoon)]/10 px-3 py-1 text-[var(--sea-ink)]">
                  Selection {selectedRange.start}-{selectedRange.end}
                </span>
              ) : null}
              <span className="rounded-full border border-white/60 bg-white/60 px-3 py-1 text-[var(--sea-ink)]">
                {reviewSummary.blocking_review_items} blocking
              </span>
            </div>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          <section className="grid h-[calc(100vh-18rem)] min-h-[calc(100vh-18rem)] items-stretch gap-0 xl:grid-cols-[minmax(0,1fr)_31rem]">
            <FullPageDocumentEditor
              document={document}
              renderModel={renderModel}
              selectedRedactionID={selectedRedactionID}
              onSelectRedaction={setSelectedRedactionID}
              selectedRange={selectedRange}
              onSelectionChange={setSelectedRange}
            />

            <ReviewSidebar
              activeTab={activeTab}
              onTabChange={setActiveTab}
              document={document}
              reviewQueueIndex={reviewQueueIndex}
              reviewQueueCount={reviewQueue.items.length}
              previousQueueDocumentId={previousQueueDocument?.id ?? null}
              nextQueueDocumentId={nextQueueDocument?.id ?? null}
              onGoToPreviousQueueDocument={() =>
                goToQueueDocument(previousQueueDocument?.id ?? null)
              }
              onGoToNextQueueDocument={() =>
                goToQueueDocument(nextQueueDocument?.id ?? null)
              }
              redactions={sortedRedactions}
              overlappingRedactionIDs={renderModel.overlappingRedactionIDs}
              reviewSummary={reviewSummary}
              selectedRedactionID={selectedRedactionID}
              onSelectRedaction={setSelectedRedactionID}
              selectedRange={selectedRange}
              selectedType={selectedType}
              manualReason={manualReason}
              onSelectedTypeChange={setSelectedType}
              onManualReasonChange={setManualReason}
              onClearSelection={() => {
                setSelectedRange(null);
                setManualReason("");
                setSidebarFeedback(null);
                window.getSelection()?.removeAllRanges();
              }}
              onAccept={(redactionId) => {
                setSidebarFeedback(null);
                setApproveFeedback(null);
                reviewMutation.mutate({ redactionId, action: "accept" });
              }}
              onReject={(redactionId) => {
                setSidebarFeedback(null);
                setApproveFeedback(null);
                reviewMutation.mutate({ redactionId, action: "reject" });
              }}
              onAddManualRedaction={() => {
                setSidebarFeedback(null);
                setApproveFeedback(null);
                addManualRedactionMutation.mutate();
              }}
              onApprove={() => {
                setApproveFeedback(null);
                setSidebarFeedback(null);
                approveMutation.mutate();
              }}
              isReviewMutationPending={reviewMutation.isPending}
              reviewMutationRedactionID={
                reviewMutation.variables?.redactionId ?? null
              }
              isAddPending={addManualRedactionMutation.isPending}
              isApprovePending={approveMutation.isPending}
              sidebarFeedback={sidebarFeedback}
              approveFeedback={approveFeedback}
              approveState={approveState}
            />
          </section>
        </CardContent>
      </Card>
    </main>
  );
}

function FullPageDocumentEditor({
  document,
  renderModel,
  selectedRedactionID,
  onSelectRedaction,
  selectedRange,
  onSelectionChange,
}: {
  document: DocumentDetail;
  renderModel: ReturnType<typeof buildHighlightRenderModel>;
  selectedRedactionID: string | null;
  onSelectRedaction: (redactionID: string) => void;
  selectedRange: SelectedDocumentRange | null;
  onSelectionChange: (range: SelectedDocumentRange | null) => void;
}) {
  return (
    <div className="flex min-h-0 flex-col border-b border-black/5 xl:border-r xl:border-b-0">
      <div className="flex min-h-0 flex-1 flex-col px-4 py-4 md:px-5 md:py-5">
        <DocumentTextSurface
          document={document}
          renderModel={renderModel}
          selectedRedactionID={selectedRedactionID}
          onSelectRedaction={onSelectRedaction}
          selectedRange={selectedRange}
          onSelectionChange={onSelectionChange}
        />
      </div>
    </div>
  );
}

function DocumentTextSurface({
  document,
  renderModel,
  selectedRedactionID,
  onSelectRedaction,
  selectedRange,
  onSelectionChange,
}: {
  document: DocumentDetail;
  renderModel: ReturnType<typeof buildHighlightRenderModel>;
  selectedRedactionID: string | null;
  onSelectRedaction: (redactionID: string) => void;
  selectedRange: SelectedDocumentRange | null;
  onSelectionChange: (range: SelectedDocumentRange | null) => void;
}) {
  const rootRef = useRef<HTMLDivElement | null>(null);
  const highlightRefs = useRef<Record<string, HTMLSpanElement | null>>({});

  const syncSelection = useCallback(() => {
    const root = rootRef.current;
    if (!root) {
      return;
    }
    const selection = window.getSelection();
    const range = getSelectedDocumentRange(root, document.text, selection);
    if (range) {
      onSelectionChange(range);
      return;
    }

    const anchorNode = selection?.anchorNode ?? null;
    const focusNode = selection?.focusNode ?? null;
    if (
      (anchorNode && root.contains(anchorNode)) ||
      (focusNode && root.contains(focusNode))
    ) {
      onSelectionChange(null);
    }
  }, [document.text, onSelectionChange]);
  useEffect(() => {
    document.getSelection;
    const handler = () => syncSelection();
    window.document.addEventListener("selectionchange", handler);
    return () => {
      window.document.removeEventListener("selectionchange", handler);
    };
  }, [syncSelection]);

  useEffect(() => {
    if (!selectedRedactionID) {
      return;
    }
    const target = highlightRefs.current[selectedRedactionID];
    if (target) {
      target.scrollIntoView({ behavior: "smooth", block: "center" });
    }
  }, [selectedRedactionID]);

  const overlapCount = renderModel.overlappingRedactionIDs.size;

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      {overlapCount > 0 ? (
        <div className="inline-flex w-fit items-center gap-2 rounded-full border border-amber-500/35 bg-amber-500/12 px-3 py-1 text-[11px] uppercase text-amber-950">
          <AlertTriangle className="size-3.5" />
          {overlapCount} overlap conflict{overlapCount === 1 ? "" : "s"} kept in
          sidebar
        </div>
      ) : null}

      <div className="min-h-0 flex-1 rounded-[1.5rem] border border-white/60 bg-[linear-gradient(180deg,rgba(255,255,255,0.88),rgba(244,249,250,0.82))] p-3 shadow-inner md:p-4">
        <div
          ref={rootRef}
          className="h-full min-h-[50vh] overflow-auto rounded-[1.15rem] border border-[rgba(23,58,64,0.08)] bg-white/75 px-4 py-5 md:px-6 md:py-6"
          onMouseUp={syncSelection}
          onKeyUp={syncSelection}
        >
          <pre className="whitespace-pre-wrap break-words font-[Georgia,serif] text-[15px] leading-7 text-[var(--sea-ink)]">
            {renderModel.segments.map((segment) => {
              const commonProps = {
                "data-segment-start": segment.start,
                "data-segment-end": segment.end,
              };

              if (segment.kind === "text") {
                const isSelected =
                  selectedRange &&
                  selectedRange.start < segment.end &&
                  selectedRange.end > segment.start;
                return (
                  <span
                    key={segment.key}
                    {...commonProps}
                    className={cn(
                      "rounded-[0.45rem] transition",
                      isSelected && "bg-[var(--lagoon)]/14",
                    )}
                  >
                    {segment.text}
                  </span>
                );
              }

              const tone = getRedactionTone(segment.redaction);
              const isSelected = selectedRedactionID === segment.redaction.id;
              return (
                <span
                  key={segment.key}
                  ref={(node) => {
                    highlightRefs.current[segment.redaction.id] = node;
                  }}
                  {...commonProps}
                  onClick={() => onSelectRedaction(segment.redaction.id)}
                  className={cn(
                    "mx-[1px] cursor-pointer rounded-[0.55rem] px-1 py-0.5 transition",
                    tone === "accepted" &&
                      "bg-emerald-300/58 ring-1 ring-emerald-700/15",
                    tone === "added" &&
                      "bg-cyan-300/58 ring-1 ring-cyan-700/20",
                    tone === "pending" &&
                      "bg-amber-300/45 ring-1 ring-amber-700/18",
                    tone === "false_positive" &&
                      "bg-sky-300/42 ring-1 ring-sky-700/16",
                    tone === "missed" &&
                      "bg-rose-300/46 ring-1 ring-rose-700/16",
                    tone === "rejected" &&
                      "bg-slate-200/70 text-slate-500 ring-1 ring-slate-400/20",
                    isSelected &&
                      "outline-2 outline-offset-2 outline-[var(--lagoon-deep)]",
                  )}
                  title={`${segment.redaction.review_state} · ${segment.redaction.type} · ${segment.redaction.start}-${segment.redaction.end}`}
                >
                  {segment.text}
                </span>
              );
            })}
          </pre>
        </div>
      </div>
    </div>
  );
}

function ReviewSidebar({
  activeTab,
  onTabChange,
  document,
  reviewQueueIndex,
  reviewQueueCount,
  previousQueueDocumentId,
  nextQueueDocumentId,
  onGoToPreviousQueueDocument,
  onGoToNextQueueDocument,
  redactions,
  overlappingRedactionIDs,
  reviewSummary,
  selectedRedactionID,
  onSelectRedaction,
  selectedRange,
  selectedType,
  manualReason,
  onSelectedTypeChange,
  onManualReasonChange,
  onClearSelection,
  onAccept,
  onReject,
  onAddManualRedaction,
  onApprove,
  isReviewMutationPending,
  reviewMutationRedactionID,
  isAddPending,
  isApprovePending,
  sidebarFeedback,
  approveFeedback,
  approveState,
}: {
  activeTab: SuggestionsTabValue;
  onTabChange: (value: SuggestionsTabValue) => void;
  document: DocumentDetail;
  reviewQueueIndex: number;
  reviewQueueCount: number;
  previousQueueDocumentId: string | null;
  nextQueueDocumentId: string | null;
  onGoToPreviousQueueDocument: () => void;
  onGoToNextQueueDocument: () => void;
  redactions: Redaction[];
  overlappingRedactionIDs: Set<string>;
  reviewSummary: ReviewSummary;
  selectedRedactionID: string | null;
  onSelectRedaction: (redactionID: string) => void;
  selectedRange: SelectedDocumentRange | null;
  selectedType: PIIType;
  manualReason: string;
  onSelectedTypeChange: (value: PIIType) => void;
  onManualReasonChange: (value: string) => void;
  onClearSelection: () => void;
  onAccept: (redactionID: string) => void;
  onReject: (redactionID: string) => void;
  onAddManualRedaction: () => void;
  onApprove: () => void;
  isReviewMutationPending: boolean;
  reviewMutationRedactionID: string | null;
  isAddPending: boolean;
  isApprovePending: boolean;
  sidebarFeedback: FeedbackState | null;
  approveFeedback: FeedbackState | null;
  approveState: ApproveState;
}) {
  return (
    <aside className="flex h-full min-h-0 flex-col bg-[linear-gradient(180deg,rgba(248,251,252,0.92),rgba(241,247,248,0.92))]">
      <div className="sticky top-0 z-10 border-b border-black/5 bg-[linear-gradient(180deg,rgba(248,251,252,0.98),rgba(244,249,250,0.96))] px-5 py-4 backdrop-blur">
        <div className="flex items-start justify-between gap-3">
          <div className="space-y-1">
            <h2 className="text-2xl font-semibold text-[var(--sea-ink)]">
              Review controls
            </h2>
            <p className="text-sm text-[var(--sea-ink-soft)]">
              Resolve suggestions, add missed redactions, then approve when
              the blocking count reaches zero.
            </p>
          </div>
          <div className="flex flex-col items-end gap-2">
            <Button
              type="button"
              className="rounded-full"
              disabled={!approveState.canApprove || isApprovePending}
              onClick={onApprove}
            >
              {approveState.buttonLabel(isApprovePending)}
            </Button>
            <div className="rounded-full border border-white/55 bg-white/65 px-3 py-1 text-xs font-semibold text-[var(--sea-ink)]">
              {reviewSummary.blocking_review_items} blocking
            </div>
          </div>
        </div>
        <p className="mt-3 text-xs leading-5 text-[var(--sea-ink-soft)]">
          {approveState.helperText}
        </p>
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="rounded-full border-white/60 bg-white/72"
            disabled={!previousQueueDocumentId}
            onClick={onGoToPreviousQueueDocument}
          >
            <ChevronLeft className="size-4" />
            Previous in queue
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="rounded-full border-white/60 bg-white/72"
            disabled={!nextQueueDocumentId}
            onClick={onGoToNextQueueDocument}
          >
            Next in queue
            <ChevronRight className="size-4" />
          </Button>
          {reviewQueueIndex >= 0 ? (
            <span className="rounded-full border border-white/60 bg-white/65 px-3 py-1 text-xs text-[var(--sea-ink-soft)]">
              Review queue position {reviewQueueIndex + 1} of {reviewQueueCount}
            </span>
          ) : null}
        </div>
        {approveFeedback ? <InlineFeedback feedback={approveFeedback} /> : null}
        {sidebarFeedback ? <InlineFeedback feedback={sidebarFeedback} /> : null}
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <Tabs
          value={activeTab}
          onValueChange={(value) => onTabChange(value as SuggestionsTabValue)}
          className="flex flex-col"
        >
          <div className="border-b border-black/5 bg-white/22 px-5 py-3">
            <TabsList variant="line" className="w-full justify-start gap-2">
              <TabsTrigger value="suggestions" className="flex-none px-3">
                Suggestions
              </TabsTrigger>
              <TabsTrigger value="stats" className="flex-none px-3">
                Stats
              </TabsTrigger>
            </TabsList>
          </div>

          <TabsContent value="suggestions" className="flex flex-col">
            <SuggestionsTab
              documentStatus={document.status}
              redactions={redactions}
              overlappingRedactionIDs={overlappingRedactionIDs}
              selectedRedactionID={selectedRedactionID}
              onSelectRedaction={onSelectRedaction}
              selectedRange={selectedRange}
              selectedType={selectedType}
              manualReason={manualReason}
              onSelectedTypeChange={onSelectedTypeChange}
              onManualReasonChange={onManualReasonChange}
              onClearSelection={onClearSelection}
              onAccept={onAccept}
              onReject={onReject}
              onAddManualRedaction={onAddManualRedaction}
              isReviewMutationPending={isReviewMutationPending}
              reviewMutationRedactionID={reviewMutationRedactionID}
              isAddPending={isAddPending}
            />
          </TabsContent>

          <TabsContent value="stats" className="flex flex-col">
            <StatsTab reviewSummary={reviewSummary} />
          </TabsContent>
        </Tabs>
      </div>
    </aside>
  );
}

function SuggestionsTab({
  documentStatus,
  redactions,
  overlappingRedactionIDs,
  selectedRedactionID,
  onSelectRedaction,
  selectedRange,
  selectedType,
  manualReason,
  onSelectedTypeChange,
  onManualReasonChange,
  onClearSelection,
  onAccept,
  onReject,
  onAddManualRedaction,
  isReviewMutationPending,
  reviewMutationRedactionID,
  isAddPending,
}: {
  documentStatus: DocumentStatus;
  redactions: Redaction[];
  overlappingRedactionIDs: Set<string>;
  selectedRedactionID: string | null;
  onSelectRedaction: (redactionID: string) => void;
  selectedRange: SelectedDocumentRange | null;
  selectedType: PIIType;
  manualReason: string;
  onSelectedTypeChange: (value: PIIType) => void;
  onManualReasonChange: (value: string) => void;
  onClearSelection: () => void;
  onAccept: (redactionID: string) => void;
  onReject: (redactionID: string) => void;
  onAddManualRedaction: () => void;
  isReviewMutationPending: boolean;
  reviewMutationRedactionID: string | null;
  isAddPending: boolean;
}) {
  return (
    <div className="flex flex-col">
      <div className="border-b border-black/5 bg-white/18 px-5 py-4">
        <AddRedactionFromSelection
          documentStatus={documentStatus}
          selectedRange={selectedRange}
          selectedType={selectedType}
          manualReason={manualReason}
          onSelectedTypeChange={onSelectedTypeChange}
          onManualReasonChange={onManualReasonChange}
          onClearSelection={onClearSelection}
          onAddRedaction={onAddManualRedaction}
          isPending={isAddPending}
        />
      </div>

      <div className="space-y-3 px-5 py-4">
        {redactions.map((redaction) => (
          <RedactionReviewCard
            key={redaction.id}
            redaction={redaction}
            isSelected={selectedRedactionID === redaction.id}
            hasConflict={overlappingRedactionIDs.has(redaction.id)}
            onSelect={() => onSelectRedaction(redaction.id)}
            onAccept={() => onAccept(redaction.id)}
            onReject={() => onReject(redaction.id)}
            isMutationPending={
              isReviewMutationPending &&
              reviewMutationRedactionID === redaction.id
            }
          />
        ))}
      </div>
    </div>
  );
}

function AddRedactionFromSelection({
  documentStatus,
  selectedRange,
  selectedType,
  manualReason,
  onSelectedTypeChange,
  onManualReasonChange,
  onClearSelection,
  onAddRedaction,
  isPending,
}: {
  documentStatus: DocumentStatus;
  selectedRange: SelectedDocumentRange | null;
  selectedType: PIIType;
  manualReason: string;
  onSelectedTypeChange: (value: PIIType) => void;
  onManualReasonChange: (value: string) => void;
  onClearSelection: () => void;
  onAddRedaction: () => void;
  isPending: boolean;
}) {
  const isLocked =
    documentStatus === "QUEUED" ||
    documentStatus === "PROCESSING" ||
    documentStatus === "FAILED" ||
    documentStatus === "EXPORTED";

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2">
        <Highlighter className="size-4 text-[var(--lagoon-deep)]" />
        <h2 className="text-sm font-semibold text-[var(--sea-ink)]">
          Add redaction from selection
        </h2>
      </div>

      {isLocked ? (
        <div className="rounded-[1rem] border border-amber-500/25 bg-amber-500/10 px-3 py-3 text-sm text-amber-950">
          {documentStatus === "QUEUED" || documentStatus === "PROCESSING"
            ? "This document is still being processed by the worker pool. Please wait for it to complete."
            : documentStatus === "FAILED"
              ? "Failed documents must be retried before manual review."
              : "Exported documents are locked in this MVP."}
        </div>
      ) : selectedRange ? (
        <>
          <div className="rounded-[1rem] border border-white/60 bg-white/60 px-3 py-3">
            <div className="text-[11px] tracking-[0.14em] text-[var(--sea-ink-soft)] uppercase">
              Selected text
            </div>
            <p className="mt-2 text-sm leading-6 text-[var(--sea-ink)]">
              “{truncatePreview(selectedRange.text, 180)}”
            </p>
            <div className="mt-2 text-xs text-[var(--sea-ink-soft)]">
              Offsets {selectedRange.start}-{selectedRange.end}
            </div>
          </div>

          <div className="grid gap-3">
            <Select
              value={selectedType}
              onValueChange={(value) => onSelectedTypeChange(value as PIIType)}
            >
              <SelectTrigger className="w-full bg-white/65">
                <SelectValue placeholder="PII type" />
              </SelectTrigger>
              <SelectContent>
                {PII_TYPE_OPTIONS.map((value) => (
                  <SelectItem key={value} value={value}>
                    {value.replaceAll("_", " ")}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            <Input
              value={manualReason}
              onChange={(event) => onManualReasonChange(event.target.value)}
              placeholder="Reason (optional)"
              className="bg-white/65"
            />

            <div className="flex gap-2">
              <Button
                type="button"
                className="flex-1 rounded-full"
                disabled={isPending}
                onClick={onAddRedaction}
              >
                {isPending ? "Adding…" : "Add redaction"}
              </Button>
              <Button
                type="button"
                variant="outline"
                className="rounded-full"
                onClick={onClearSelection}
              >
                Clear
              </Button>
            </div>
          </div>
        </>
      ) : (
        <div className="rounded-[1rem] border border-dashed border-white/60 bg-white/50 px-3 py-3 text-sm leading-6 text-[var(--sea-ink-soft)]">
          Select text inside the document viewer to create a manual redaction
          with exact code-point offsets.
        </div>
      )}
    </div>
  );
}

function RedactionReviewCard({
  redaction,
  isSelected,
  hasConflict,
  onSelect,
  onAccept,
  onReject,
  isMutationPending,
}: {
  redaction: Redaction;
  isSelected: boolean;
  hasConflict: boolean;
  onSelect: () => void;
  onAccept: () => void;
  onReject: () => void;
  isMutationPending: boolean;
}) {
  const tone = getRedactionTone(redaction);
  const acceptDisabled =
    isMutationPending ||
    redaction.review_state === "ACCEPTED" ||
    redaction.review_state === "ADDED";
  const rejectDisabled =
    isMutationPending || redaction.review_state === "REJECTED";

  return (
    <article
      role="button"
      tabIndex={0}
      onClick={onSelect}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onSelect();
        }
      }}
      className={cn(
        "w-full rounded-[1.05rem] border px-4 py-4 text-left transition outline-none",
        "border-white/55 bg-white/68 hover:bg-white/82 focus-visible:ring-2 focus-visible:ring-[var(--lagoon-deep)]",
        tone === "accepted" && "shadow-[inset_4px_0_0_0_rgba(16,185,129,0.55)]",
        tone === "added" && "shadow-[inset_4px_0_0_0_rgba(6,182,212,0.55)]",
        tone === "pending" && "shadow-[inset_4px_0_0_0_rgba(245,158,11,0.55)]",
        tone === "false_positive" &&
          "shadow-[inset_4px_0_0_0_rgba(14,165,233,0.55)]",
        tone === "missed" && "shadow-[inset_4px_0_0_0_rgba(244,63,94,0.55)]",
        tone === "rejected" &&
          "opacity-80 shadow-[inset_4px_0_0_0_rgba(100,116,139,0.32)]",
        isSelected && "ring-2 ring-[var(--lagoon-deep)] bg-white/84",
      )}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <ReviewStateBadge state={redaction.review_state} />
            <span className="rounded-full border border-white/65 bg-white/70 px-2 py-0.5 text-[11px] uppercase text-[var(--sea-ink)]">
              {redaction.type.replaceAll("_", " ")}
            </span>
            <span className="rounded-full border border-white/65 bg-white/70 px-2 py-0.5 text-[11px] uppercase text-[var(--sea-ink-soft)]">
              {Math.round(redaction.confidence * 100)}%
            </span>
            {hasConflict ? (
              <span className="rounded-full border border-amber-500/30 bg-amber-500/12 px-2 py-0.5 text-[11px] uppercase text-amber-950">
                overlap
              </span>
            ) : null}
          </div>
          <p className="text-sm leading-6 text-[var(--sea-ink)]">
            “{truncatePreview(redaction.text, 86)}”
          </p>
        </div>
        {redaction.review_state === "REJECTED" ? (
          <XCircle className="mt-1 size-4 text-slate-500" />
        ) : redaction.review_state === "PENDING" ? (
          <ShieldAlert className="mt-1 size-4 text-amber-700" />
        ) : (
          <CheckCircle2 className="mt-1 size-4 text-emerald-700" />
        )}
      </div>

      <div className="mt-3 flex flex-wrap gap-2 text-[11px] uppercase text-[var(--sea-ink-soft)]">
        <span>{redaction.source}</span>
        <span>{redaction.suggested_status}</span>
        <span>
          {redaction.start}-{redaction.end}
        </span>
      </div>

      <p className="mt-3 text-sm leading-6 text-[var(--sea-ink-soft)]">
        {redaction.reason}
      </p>

      <div className="mt-4 flex gap-2">
        <Button
          type="button"
          size="sm"
          className="rounded-full"
          disabled={acceptDisabled}
          onClick={(event) => {
            event.stopPropagation();
            onAccept();
          }}
        >
          {redaction.review_state === "ADDED"
            ? "Already added"
            : redaction.review_state === "ACCEPTED"
              ? "Accepted"
              : isMutationPending
                ? "Saving…"
                : "Accept"}
        </Button>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="rounded-full"
          disabled={rejectDisabled}
          onClick={(event) => {
            event.stopPropagation();
            onReject();
          }}
        >
          {redaction.review_state === "ADDED"
            ? isMutationPending
              ? "Clearing…"
              : "Clear redaction"
            : redaction.review_state === "REJECTED"
              ? "Rejected"
              : isMutationPending
                ? "Saving…"
                : "Reject"}
        </Button>
      </div>
    </article>
  );
}

function StatsTab({ reviewSummary }: { reviewSummary: ReviewSummary }) {
  return (
    <div className="px-5 py-4">
      <div className="grid gap-3 sm:grid-cols-2">
        <ReviewMetric
          label="Status"
          value={reviewSummary.status.replace("_", " ")}
        />
        <ReviewMetric label="Risk level" value={reviewSummary.risk_level} />
        <ReviewMetric
          label="Total redactions"
          value={reviewSummary.total_redactions.toLocaleString()}
        />
        <ReviewMetric
          label="Pending"
          value={reviewSummary.pending.toLocaleString()}
        />
        <ReviewMetric
          label="Accepted"
          value={reviewSummary.accepted.toLocaleString()}
        />
        <ReviewMetric
          label="Rejected"
          value={reviewSummary.rejected.toLocaleString()}
        />
        <ReviewMetric
          label="Added"
          value={reviewSummary.added.toLocaleString()}
        />
        <ReviewMetric
          label="Low confidence"
          value={reviewSummary.low_confidence.toLocaleString()}
        />
        <ReviewMetric
          label="Regex candidates"
          value={reviewSummary.regex_candidates.toLocaleString()}
        />
        <ReviewMetric
          label="False positives"
          value={reviewSummary.controlled_false_positives.toLocaleString()}
        />
        <ReviewMetric
          label="Missed PII"
          value={reviewSummary.controlled_missed_pii.toLocaleString()}
        />
        <ReviewMetric
          label="Blocking items"
          value={reviewSummary.blocking_review_items.toLocaleString()}
        />
      </div>

      <div className="mt-4 rounded-[1.15rem] border border-white/60 bg-white/60 px-4 py-4">
        <div className="flex items-center gap-2 text-sm font-semibold text-[var(--sea-ink)]">
          {reviewSummary.can_approve ? (
            <ShieldCheck className="size-4 text-emerald-700" />
          ) : (
            <ShieldAlert className="size-4 text-amber-700" />
          )}
          Approval gate
        </div>
        <p className="mt-2 text-sm leading-6 text-[var(--sea-ink-soft)]">
          {reviewSummary.can_approve
            ? "All blocking review items are resolved. The document can now be approved."
            : "Approval remains disabled until the blocking review count reaches zero and the document is not in a failed state."}
        </p>
      </div>
    </div>
  );
}

function ReviewMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-[1.15rem] border border-white/60 bg-white/60 px-4 py-3">
      <div className="text-[11px] tracking-[0.16em] text-[var(--sea-ink-soft)] uppercase">
        {label}
      </div>
      <div className="mt-2 text-lg font-semibold text-[var(--sea-ink)]">
        {value}
      </div>
    </div>
  );
}

function ReviewStateBadge({ state }: { state: Redaction["review_state"] }) {
  const stateStyles: Record<Redaction["review_state"], string> = {
    PENDING: "border-amber-500/25 bg-amber-500/14 text-amber-950",
    ACCEPTED: "border-emerald-500/25 bg-emerald-500/14 text-emerald-950",
    REJECTED: "border-slate-500/20 bg-slate-500/10 text-slate-700",
    ADDED: "border-cyan-500/25 bg-cyan-500/14 text-cyan-950",
  };

  return (
    <span
      className={cn(
        "rounded-full border px-2 py-0.5 text-[11px] tracking-[0.18em] uppercase",
        stateStyles[state],
      )}
    >
      {state}
    </span>
  );
}

function ShortcutHint({
  keys,
  description,
}: {
  keys: string;
  description: string;
}) {
  return (
    <div className="rounded-[1rem] border border-white/60 bg-white/62 px-4 py-3">
      <div className="font-mono text-xs font-semibold text-[var(--sea-ink)]">
        {keys}
      </div>
      <div className="mt-1 text-sm text-[var(--sea-ink-soft)]">
        {description}
      </div>
    </div>
  );
}

function ApprovalStatePill({
  status,
  reviewSummary,
}: {
  status: DocumentStatus;
  reviewSummary: ReviewSummary;
}) {
  const isReady = reviewSummary.can_approve;
  return (
    <div
      className={cn(
        "inline-flex items-center gap-2 rounded-full border px-3 py-1 text-[11px] uppercase",
        isReady
          ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-950"
          : "border-amber-500/30 bg-amber-500/12 text-amber-950",
      )}
    >
      {isReady ? (
        <ShieldCheck className="size-3.5" />
      ) : (
        <ShieldAlert className="size-3.5" />
      )}
      {status === "QUEUED" || status === "PROCESSING"
        ? "Processing…"
        : status === "FAILED"
          ? "Retry required"
          : reviewSummary.can_approve
            ? "Approval unlocked"
            : `${reviewSummary.blocking_review_items} unresolved`}
    </div>
  );
}

function isEditableTarget(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) {
    return false;
  }
  const tagName = target.tagName.toLowerCase();
  return (
    target.isContentEditable ||
    tagName === "input" ||
    tagName === "textarea" ||
    tagName === "select" ||
    tagName === "button" ||
    Boolean(target.closest('[contenteditable="true"]'))
  );
}

function InlineFeedback({ feedback }: { feedback: FeedbackState }) {
  return (
    <div
      className={cn(
        "rounded-[1rem] border px-3 py-3 text-sm leading-6",
        feedback.tone === "success"
          ? "border-emerald-500/25 bg-emerald-500/10 text-emerald-950"
          : "border-rose-500/25 bg-rose-500/10 text-rose-950",
      )}
    >
      {feedback.message}
    </div>
  );
}

function DocumentDetailPending() {
  return (
    <div className="page-wrap px-4 py-8 lg:px-0 lg:py-10">
      <div className="rounded-[1.75rem] border border-white/55 bg-white/65 p-10 text-sm text-[var(--sea-ink-soft)]">
        Loading document detail, review summary, and redactions…
      </div>
    </div>
  );
}

type ApproveState = {
  canApprove: boolean;
  helperText: string;
  buttonLabel: (isPending: boolean) => string;
};

function getApproveState(
  status: DocumentStatus,
  reviewSummary: ReviewSummary,
): ApproveState {
  if (status === "FAILED") {
    return {
      canApprove: false,
      helperText:
        "Failed documents must be retried from the batch table before approval.",
      buttonLabel: () => "Retry required",
    };
  }

  if (status === "EXPORTED") {
    return {
      canApprove: false,
      helperText:
        "This document has already been exported and is locked in the current runtime state.",
      buttonLabel: () => "Already exported",
    };
  }

  if (status === "APPROVED") {
    return {
      canApprove: false,
      helperText: "This document is already approved and ready for export.",
      buttonLabel: () => "Already approved",
    };
  }

  if (!reviewSummary.can_approve) {
    return {
      canApprove: false,
      helperText: `Resolve ${reviewSummary.blocking_review_items} blocking review item${reviewSummary.blocking_review_items === 1 ? "" : "s"} before approval.`,
      buttonLabel: () => "Resolve review items first",
    };
  }

  return {
    canApprove: true,
    helperText:
      status === "NEEDS_REVIEW"
        ? "All blocking review items are cleared. This reviewed exception document is ready for approval."
        : "This document is safe to approve from the review surface.",
    buttonLabel: (isPending) => (isPending ? "Approving…" : "Approve document"),
  };
}
