package agentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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
	Code        int                     `json:"code"`
	Message     string                  `json:"message"`
	FailureCode FailureCode             `json:"failure_code,omitempty"`
	Data        json.RawMessage         `json:"data"`
	Audit       *telemetry.AuditSummary `json:"audit,omitempty"`
}

// ErrorKind classifies where an Agent client call failed. It is intentionally
// independent of human-readable error messages.
type ErrorKind string

const (
	KindTransport  ErrorKind = "transport"
	KindTimeout    ErrorKind = "timeout"
	KindCanceled   ErrorKind = "canceled"
	KindDownstream ErrorKind = "downstream"
	KindProtocol   ErrorKind = "protocol"
)

// FailureCode is the stable, machine-readable failure identifier exchanged
// with the private Agent API. FailureCodeLegacy marks an older Agent response
// that did not include a failure_code; callers must not infer one from Message.
type FailureCode string

const (
	FailureCodeTransport             FailureCode = "transport_error"
	FailureCodeProtocol              FailureCode = "protocol_error"
	FailureCodeLegacy                FailureCode = "legacy"
	FailureCodeAgentDisabled         FailureCode = "agent_disabled"
	FailureCodeInvalidInternalSecret FailureCode = "invalid_internal_secret"
	FailureCodeInvalidRequestBody    FailureCode = "invalid_request_body"
	FailureCodeInvalidMessage        FailureCode = "invalid_message"
	FailureCodeInvalidSession        FailureCode = "invalid_session"
	FailureCodeInvalidHistory        FailureCode = "invalid_history"
	FailureCodeAgentBusy             FailureCode = "agent_busy"
	FailureCodeQueueTimeout          FailureCode = "queue_timeout"
	FailureCodeRequestCanceled       FailureCode = "request_canceled"
	FailureCodeExecutionTimeout      FailureCode = "execution_timeout"
	FailureCodeExecutionFailed       FailureCode = "execution_failed"
)

// ClientError is the stable error boundary exposed by Client. StatusCode is
// set only for a downstream HTTP response or a local protocol failure. Err is
// retained internally and exposed through Unwrap so context cancellation and
// deadlines remain detectable with errors.Is.
type ClientError struct {
	Kind        ErrorKind
	FailureCode FailureCode
	StatusCode  int
	Message     string
	RequestID   string
	Audit       *telemetry.AuditSummary
	err         error
}

func (e *ClientError) Error() string {
	if e == nil {
		return "agent client error"
	}
	if e.err != nil {
		return e.err.Error()
	}
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("agent client failure: %s", e.FailureCode)
}

func (e *ClientError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// HTTPError is retained for compatibility with callers that classify Agent
// failures by HTTP status. New callers should inspect ClientError first so
// stable FailureCode values such as agent_busy and agent_disabled are kept
// distinct even though both use HTTP 503.
type HTTPError struct {
	Kind        ErrorKind
	FailureCode FailureCode
	StatusCode  int
	Message     string
	RequestID   string
	Audit       *telemetry.AuditSummary
}

func (e *HTTPError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("agent service returned HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("agent service returned HTTP %d: %s", e.StatusCode, e.Message)
}

func newClientError(kind ErrorKind, failureCode FailureCode, statusCode int, message, requestID string, audit *telemetry.AuditSummary, err error) *ClientError {
	return &ClientError{
		Kind:        kind,
		FailureCode: failureCode,
		StatusCode:  statusCode,
		Message:     message,
		RequestID:   requestID,
		Audit:       audit,
		err:         err,
	}
}

func newTransportError(err error, requestID string) *ClientError {
	kind := KindTransport
	failureCode := FailureCodeTransport
	switch {
	case errors.Is(err, context.Canceled):
		kind = KindCanceled
		failureCode = FailureCodeRequestCanceled
	case errors.Is(err, context.DeadlineExceeded):
		kind = KindTimeout
		failureCode = FailureCodeExecutionTimeout
	default:
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			kind = KindTimeout
			failureCode = FailureCodeExecutionTimeout
		}
	}
	return newClientError(kind, failureCode, 0, "", requestID, nil, err)
}

func newDownstreamError(statusCode int, message, requestID string, audit *telemetry.AuditSummary, failureCode FailureCode) *ClientError {
	if failureCode == "" {
		failureCode = FailureCodeLegacy
	}
	httpErr := &HTTPError{
		Kind:        KindDownstream,
		FailureCode: failureCode,
		StatusCode:  statusCode,
		Message:     message,
		RequestID:   requestID,
		Audit:       audit,
	}
	return newClientError(KindDownstream, failureCode, statusCode, message, requestID, audit, httpErr)
}

func matchesDownstreamFailure(statusCode, envelopeCode int, code FailureCode) bool {
	switch code {
	case FailureCodeAgentDisabled, FailureCodeAgentBusy, FailureCodeQueueTimeout:
		return statusCode == http.StatusServiceUnavailable && envelopeCode == http.StatusServiceUnavailable
	case FailureCodeInvalidInternalSecret:
		return statusCode == http.StatusUnauthorized && envelopeCode == http.StatusUnauthorized
	case FailureCodeInvalidRequestBody,
		FailureCodeInvalidMessage,
		FailureCodeInvalidSession,
		FailureCodeInvalidHistory:
		return statusCode == http.StatusBadRequest && envelopeCode == http.StatusBadRequest
	case FailureCodeRequestCanceled, FailureCodeExecutionFailed:
		return statusCode == http.StatusBadGateway && envelopeCode == http.StatusBadGateway
	case FailureCodeExecutionTimeout:
		return statusCode == http.StatusGatewayTimeout && envelopeCode == http.StatusGatewayTimeout
	default:
		return false
	}
}

func newProtocolError(message, requestID string, audit *telemetry.AuditSummary, err error, legacyHTTPStatus int) *ClientError {
	if err == nil {
		err = errors.New(message)
	}
	if legacyHTTPStatus != 0 {
		err = &HTTPError{
			Kind:        KindProtocol,
			FailureCode: FailureCodeProtocol,
			StatusCode:  legacyHTTPStatus,
			Message:     message,
			RequestID:   requestID,
			Audit:       audit,
		}
	}
	return newClientError(KindProtocol, FailureCodeProtocol, http.StatusBadGateway, message, requestID, audit, err)
}

func (c *Client) Chat(ctx context.Context, req ChatRequest, authorization, requestID string) (ChatResponse, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return ChatResponse{}, newProtocolError("encode agent request", requestID, nil, fmt.Errorf("encode agent request: %w", err), 0)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat", bytes.NewReader(payload))
	if err != nil {
		return ChatResponse{}, newProtocolError("create agent request", requestID, nil, fmt.Errorf("create agent request: %w", err), 0)
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
		return ChatResponse{}, newTransportError(fmt.Errorf("agent service request: %w", err), requestID)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAgentResponseBytes+1))
	if err != nil {
		return ChatResponse{}, newTransportError(fmt.Errorf("read agent response: %w", err), requestID)
	}
	if len(body) > maxAgentResponseBytes {
		return ChatResponse{}, newProtocolError("agent response is too large", requestID, nil, nil, http.StatusBadGateway)
	}

	var envelope apiResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ChatResponse{}, newProtocolError("invalid agent response", requestID, nil, err, resp.StatusCode)
	}
	if envelope.Audit != nil && strings.TrimSpace(requestID) != "" && envelope.Audit.RequestID != requestID {
		// Treat a correlation mismatch as an upstream protocol failure. Never
		// attach the mismatched audit record to the trusted request.
		return ChatResponse{}, newProtocolError("invalid agent response", requestID, nil, nil, http.StatusBadGateway)
	}
	if envelope.FailureCode != "" {
		if !matchesDownstreamFailure(resp.StatusCode, envelope.Code, envelope.FailureCode) {
			return ChatResponse{}, newProtocolError("invalid agent response", requestID, envelope.Audit, nil, resp.StatusCode)
		}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return ChatResponse{}, newDownstreamError(resp.StatusCode, envelope.Message, requestID, envelope.Audit, envelope.FailureCode)
	}
	if envelope.Code != 0 {
		return ChatResponse{}, newDownstreamError(resp.StatusCode, envelope.Message, requestID, envelope.Audit, envelope.FailureCode)
	}
	var result ChatResponse
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return ChatResponse{}, newProtocolError("agent response data is missing", requestID, envelope.Audit, nil, 0)
	}
	if err := json.Unmarshal(envelope.Data, &result); err != nil {
		return ChatResponse{}, newProtocolError("decode agent response", requestID, envelope.Audit, fmt.Errorf("decode agent response: %w", err), 0)
	}
	if strings.TrimSpace(result.Answer) == "" || strings.TrimSpace(result.SessionID) == "" {
		return ChatResponse{}, newProtocolError("agent response is incomplete", requestID, envelope.Audit, nil, 0)
	}
	result.RequestID = requestID
	result.Audit = envelope.Audit
	return result, nil
}
