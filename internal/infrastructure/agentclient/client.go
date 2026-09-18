package agentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"SWPUCAT/internal/agent/telemetry"
)

const maxAgentResponseBytes = 2 << 20

// Client is the main server's narrow HTTP boundary to the Agent service.
// Authentication is resolved by the main server; the client only forwards the
// resulting identity and request correlation headers to the private Agent API.
type Client struct {
	baseURL        string
	internalSecret string
	httpClient     *http.Client
}

func NewClient(baseURL, internalSecret string, timeout time.Duration) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("agent base URL is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("agent base URL is invalid")
	}
	if timeout <= 0 {
		timeout = 75 * time.Second
	}
	return &Client{
		baseURL:        baseURL,
		internalSecret: internalSecret,
		httpClient:     &http.Client{Timeout: timeout},
	}, nil
}

type ChatRequest struct {
	Message          string            `json:"message"`
	SessionID        string            `json:"session_id,omitempty"`
	UserID           int64             `json:"user_id"`
	Role             string            `json:"role"`
	HistoryProvided  bool              `json:"history_provided,omitempty"`
	History          []HistoryMessage  `json:"history,omitempty"`
	RollingSummary   string            `json:"rolling_summary,omitempty"`
	KnowledgeContext []KnowledgeSource `json:"knowledge_context,omitempty"`
	PreviousSources  []KnowledgeSource `json:"previous_sources,omitempty"`
}

// HistoryMessage is the server-owned, already authorized conversation context
// sent to the private Agent process. The Agent never uses a session identifier
// as an authorization boundary; the main server has filtered these messages by
// (user_id, session_id) before making this request.
type HistoryMessage struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolName   string `json:"tool_name,omitempty"`
	ToolResult string `json:"tool_result,omitempty"`
}

type ChatResponse struct {
	Answer    string                  `json:"answer"`
	SessionID string                  `json:"session_id"`
	Sources   []KnowledgeSource       `json:"sources,omitempty"`
	RequestID string                  `json:"-"`
	Audit     *telemetry.AuditSummary `json:"-"`
}

type KnowledgeSource struct {
	ID           int64   `json:"id"`
	Type         string  `json:"type"`
	Name         string  `json:"name"`
	Description  string  `json:"description,omitempty"`
	URL          string  `json:"url,omitempty"`
	DownloadURL  string  `json:"download_url,omitempty"`
	CategoryName string  `json:"category_name"`
	SourceKind   string  `json:"source_kind,omitempty"`
	Excerpt      string  `json:"excerpt,omitempty"`
	Score        float64 `json:"score,omitempty"`
	MatchMode    string  `json:"match_mode,omitempty"`
	CreatedAt    string  `json:"created_at,omitempty"`
}

type apiResponse struct {
	Code    int                     `json:"code"`
	Message string                  `json:"message"`
	Data    json.RawMessage         `json:"data"`
	Audit   *telemetry.AuditSummary `json:"audit,omitempty"`
}

// HTTPError preserves the Agent HTTP status so the API handler can distinguish
// an unavailable/disabled Agent from a malformed request or upstream failure.
type HTTPError struct {
	StatusCode int
	Message    string
	RequestID  string
	Audit      *telemetry.AuditSummary
}

func (e *HTTPError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("agent service returned HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("agent service returned HTTP %d: %s", e.StatusCode, e.Message)
}

func (c *Client) Chat(ctx context.Context, req ChatRequest, authorization, requestID string) (ChatResponse, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("encode agent request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat", bytes.NewReader(payload))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("create agent request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		httpReq.Header.Set("Authorization", authorization)
	}
	if requestID != "" {
		httpReq.Header.Set("X-Request-ID", requestID)
	}
	if c.internalSecret != "" {
		httpReq.Header.Set("X-Agent-Internal-Secret", c.internalSecret)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("agent service request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAgentResponseBytes+1))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("read agent response: %w", err)
	}
	if len(body) > maxAgentResponseBytes {
		return ChatResponse{}, &HTTPError{StatusCode: http.StatusBadGateway, Message: "agent response is too large", RequestID: requestID}
	}

	var envelope apiResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ChatResponse{}, &HTTPError{StatusCode: resp.StatusCode, Message: "invalid agent response", RequestID: requestID}
	}
	if envelope.Audit != nil && strings.TrimSpace(requestID) != "" && envelope.Audit.RequestID != requestID {
		// Treat a correlation mismatch as an upstream protocol failure. Never
		// attach the mismatched audit record to the trusted request.
		return ChatResponse{}, &HTTPError{StatusCode: http.StatusBadGateway, Message: "invalid agent response", RequestID: requestID}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return ChatResponse{}, &HTTPError{StatusCode: resp.StatusCode, Message: envelope.Message, RequestID: requestID, Audit: envelope.Audit}
	}
	if envelope.Code != 0 {
		return ChatResponse{}, &HTTPError{StatusCode: resp.StatusCode, Message: envelope.Message, RequestID: requestID, Audit: envelope.Audit}
	}
	var result ChatResponse
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return ChatResponse{}, fmt.Errorf("agent response data is missing")
	}
	if err := json.Unmarshal(envelope.Data, &result); err != nil {
		return ChatResponse{}, fmt.Errorf("decode agent response: %w", err)
	}
	if strings.TrimSpace(result.Answer) == "" || strings.TrimSpace(result.SessionID) == "" {
		return ChatResponse{}, fmt.Errorf("agent response is incomplete")
	}
	result.RequestID = requestID
	result.Audit = envelope.Audit
	return result, nil
}
