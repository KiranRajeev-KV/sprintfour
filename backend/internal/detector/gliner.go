package detector

import (
	"backend/internal/document"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type glinerClient struct {
	logger     *slog.Logger
	baseURL    string
	httpClient *http.Client
	permits    chan struct{}
}

type glinerDetectRequest struct {
	DocumentID string `json:"document_id"`
	Text       string `json:"text"`
}

type glinerDetectResponse struct {
	Model string             `json:"model"`
	Items []glinerDetectItem `json:"items"`
}

type glinerDetectItem struct {
	Start int     `json:"start"`
	End   int     `json:"end"`
	Text  string  `json:"text"`
	Label string  `json:"label"`
	Score float64 `json:"score"`
}

func newGLiNERClient(logger *slog.Logger, baseURL string, timeoutMS int, maxConcurrency int) *glinerClient {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "http://127.0.0.1:8090"
	}
	if timeoutMS <= 0 {
		timeoutMS = 2500
	}
	if maxConcurrency <= 0 {
		maxConcurrency = 1
	}

	return &glinerClient{
		logger:  logger,
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutMS) * time.Millisecond,
		},
		permits: make(chan struct{}, maxConcurrency),
	}
}

func (c *glinerClient) Detect(ctx context.Context, documentID, text string) ([]document.RuntimeDetection, error) {
	select {
	case c.permits <- struct{}{}:
	case <-ctx.Done():
		return nil, fmt.Errorf("wait for gliner permit: %w", ctx.Err())
	}
	defer func() {
		<-c.permits
	}()

	requestBody, err := json.Marshal(glinerDetectRequest{
		DocumentID: documentID,
		Text:       text,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal gliner request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/detect", bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("create gliner request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	startedAt := time.Now()
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("post gliner detect: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gliner detect returned status %d", response.StatusCode)
	}

	var payload glinerDetectResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode gliner response: %w", err)
	}

	detections := make([]document.RuntimeDetection, 0, len(payload.Items))
	for _, item := range payload.Items {
		mappedType := mapGLiNERLabel(item.Label)
		if mappedType == "" {
			continue
		}
		if item.Start < 0 || item.End <= item.Start {
			continue
		}

		reviewState := "REVIEW"
		if shouldAutoAcceptGLiNERDetection(mappedType, item.Score) {
			reviewState = "ACCEPTED"
		}

		detections = append(detections, document.RuntimeDetection{
			Start:           item.Start,
			End:             item.End,
			Text:            item.Text,
			Type:            mappedType,
			Confidence:      float64Pointer(item.Score),
			Reason:          fmt.Sprintf("Detected %s via local GLiNER sidecar", strings.ToLower(strings.ReplaceAll(mappedType, "_", " "))),
			Source:          "gliner_local",
			SuggestedStatus: reviewState,
		})
	}

	c.logger.Info("gliner_detect_succeeded",
		slog.String("document_id", documentID),
		slog.String("model", payload.Model),
		slog.Int("detections", len(detections)),
		slog.Int("active_requests", len(c.permits)),
		slog.Duration("duration", time.Since(startedAt)),
	)

	return detections, nil
}

func float64Pointer(value float64) *float64 {
	return &value
}

func mapGLiNERLabel(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "person":
		return "PERSON"
	case "email":
		return "EMAIL"
	case "phone":
		return "PHONE"
	case "address":
		return "ADDRESS"
	case "organization_contact":
		return "ORGANIZATION_CONTACT"
	default:
		return ""
	}
}

func shouldAutoAcceptGLiNERDetection(label string, score float64) bool {
	switch label {
	case "EMAIL":
		return score >= 0.75
	case "PHONE":
		return score >= 0.72
	case "ADDRESS":
		return score >= 0.68
	case "PERSON":
		return score >= 0.72
	case "ORGANIZATION_CONTACT":
		return score >= 0.78
	default:
		return false
	}
}
