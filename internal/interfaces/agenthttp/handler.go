package agenthttp

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"SWPUCAT/internal/agent"
	"SWPUCAT/internal/agent/telemetry"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	engine         chatEngine
	internalSecret string
}

type chatEngine interface {
	Enabled() bool
	Chat(context.Context, agent.ChatRequest) (agent.ChatResponse, error)
}

type streamingChatEngine interface {
	chatEngine
	ChatStream(context.Context, agent.ChatRequest, agent.ChatStreamEmitter) (agent.ChatResponse, error)
}

type responseEnvelope struct {
	Code        int                     `json:"code"`
	Message     string                  `json:"message"`
	FailureCode failureCode             `json:"failure_code,omitempty"`
	Data        any                     `json:"data,omitempty"`
	Audit       *telemetry.AuditSummary `json:"audit,omitempty"`
}

type streamEnvelope struct {
	Type        string                  `json:"type"`
	Delta       string                  `json:"delta,omitempty"`
	Code        int                     `json:"code,omitempty"`
	Message     string                  `json:"message,omitempty"`
	FailureCode failureCode             `json:"failure_code,omitempty"`
	Data        any                     `json:"data,omitempty"`
	Audit       *telemetry.AuditSummary `json:"audit,omitempty"`
}

// failureCode is the private HTTP contract consumed by agentclient. Keep these
// identifiers stable; callers must never derive them from Message.
type failureCode string

const (
	failureAgentDisabled         failureCode = "agent_disabled"
	failureInvalidInternalSecret failureCode = "invalid_internal_secret"
	failureInvalidRequestBody    failureCode = "invalid_request_body"
	failureInvalidMessage        failureCode = "invalid_message"
	failureInvalidSession        failureCode = "invalid_session"
	failureInvalidHistory        failureCode = "invalid_history"
	failureAgentBusy             failureCode = "agent_busy"
	failureQueueTimeout          failureCode = "queue_timeout"
	failureRequestCanceled       failureCode = "request_canceled"
	failureExecutionTimeout      failureCode = "execution_timeout"
	failureExecutionFailed       failureCode = "execution_failed"
)

func NewHandler(engine *agent.Engine, internalSecret string) *Handler {
	var boundary chatEngine
	if engine != nil {
		boundary = engine
	}
	return newHandler(boundary, internalSecret)
}

func newHandler(engine chatEngine, internalSecret string) *Handler {
	return &Handler{engine: engine, internalSecret: internalSecret}
}

func (h *Handler) Health(c *gin.Context) {
	enabled := h != nil && h.engine != nil && h.engine.Enabled()
	c.JSON(http.StatusOK, gin.H{"status": "ok", "enabled": enabled})
}

func (h *Handler) Chat(c *gin.Context) {
	requestID := normalizeRequestID(c.GetHeader("X-Request-ID"))
	if requestID != "" {
		c.Header("X-Request-ID", requestID)
	}
	if h == nil || h.engine == nil {
		writeResponse(c, http.StatusServiceUnavailable, 503, "agent is disabled", failureAgentDisabled, nil, requestAudit(requestID, nil))
		return
	}
	if h.engine.Enabled() && (h.internalSecret == "" || c.GetHeader("X-Agent-Internal-Secret") != h.internalSecret) {
		writeResponse(c, http.StatusUnauthorized, 401, "invalid internal secret", failureInvalidInternalSecret, nil, nil)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	var req agent.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeResponse(c, http.StatusBadRequest, 400, "invalid request body", failureInvalidRequestBody, nil, requestAudit(requestID, nil))
		return
	}
	req.Authorization = c.GetHeader("Authorization")
	req.RequestID = requestID
	result, err := h.engine.Chat(c.Request.Context(), req)
	audit := requestAudit(requestID, &result.Telemetry)
	if err != nil {
		var netErr net.Error
		switch {
		case errors.Is(err, agent.ErrDisabled):
			writeResponse(c, http.StatusServiceUnavailable, 503, "agent is disabled", failureAgentDisabled, nil, audit)
		case errors.Is(err, agent.ErrInvalidMessage):
			writeResponse(c, http.StatusBadRequest, 400, err.Error(), failureInvalidMessage, nil, audit)
		case errors.Is(err, agent.ErrInvalidSession):
			writeResponse(c, http.StatusBadRequest, 400, err.Error(), failureInvalidSession, nil, audit)
		case errors.Is(err, agent.ErrInvalidHistory):
			writeResponse(c, http.StatusBadRequest, 400, err.Error(), failureInvalidHistory, nil, audit)
		case errors.Is(err, agent.ErrBusy):
			writeResponse(c, http.StatusServiceUnavailable, 503, "agent is busy; please retry shortly", failureAgentBusy, nil, audit)
		case errors.Is(err, agent.ErrQueueTimeout):
			writeResponse(c, http.StatusServiceUnavailable, 503, "agent queue wait timed out; please retry shortly", failureQueueTimeout, nil, audit)
		case errors.Is(err, context.Canceled):
			writeResponse(c, http.StatusBadGateway, 502, "agent execution failed", failureRequestCanceled, nil, audit)
		case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
			writeResponse(c, http.StatusGatewayTimeout, 504, "agent execution timed out", failureExecutionTimeout, nil, audit)
		default:
			writeResponse(c, http.StatusBadGateway, 502, "agent execution failed", failureExecutionFailed, nil, audit)
		}
		return
	}
	writeResponse(c, http.StatusOK, 0, "success", "", result, audit)
}

func (h *Handler) ChatStream(c *gin.Context) {
	requestID := normalizeRequestID(c.GetHeader("X-Request-ID"))
	if requestID != "" {
		c.Header("X-Request-ID", requestID)
	}
	if h == nil || h.engine == nil {
		writeResponse(c, http.StatusServiceUnavailable, 503, "agent streaming is disabled", failureAgentDisabled, nil, requestAudit(requestID, nil))
		return
	}
	streamEngine, ok := h.engine.(streamingChatEngine)
	if !ok {
		writeResponse(c, http.StatusServiceUnavailable, 503, "agent streaming is disabled", failureAgentDisabled, nil, requestAudit(requestID, nil))
		return
	}
	if h.engine.Enabled() && (h.internalSecret == "" || c.GetHeader("X-Agent-Internal-Secret") != h.internalSecret) {
		writeResponse(c, http.StatusUnauthorized, 401, "invalid internal secret", failureInvalidInternalSecret, nil, nil)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	var req agent.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeResponse(c, http.StatusBadRequest, 400, "invalid request body", failureInvalidRequestBody, nil, requestAudit(requestID, nil))
		return
	}
	req.Authorization = c.GetHeader("Authorization")
	req.RequestID = requestID
	c.Header("Content-Type", "application/x-ndjson")
	c.Header("Cache-Control", "no-cache, no-store")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	encoder := json.NewEncoder(c.Writer)
	writeEvent := func(event streamEnvelope) error {
		if err := encoder.Encode(event); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	}
	result, err := streamEngine.ChatStream(c.Request.Context(), req, func(event agent.ChatStreamEvent) error {
		if event.Delta == "" {
			return nil
		}
		return writeEvent(streamEnvelope{Type: "delta", Delta: event.Delta})
	})
	audit := requestAudit(requestID, &result.Telemetry)
	if err != nil {
		code, message, failure := classifyAgentError(err)
		_ = writeEvent(streamEnvelope{Type: "error", Code: code, Message: message, FailureCode: failure, Audit: audit})
		return
	}
	_ = writeEvent(streamEnvelope{Type: "done", Data: result, Audit: audit})
}

func classifyAgentError(err error) (code int, message string, failure failureCode) {
	var netErr net.Error
	switch {
	case errors.Is(err, agent.ErrDisabled):
		return http.StatusServiceUnavailable, "agent is disabled", failureAgentDisabled
	case errors.Is(err, agent.ErrInvalidMessage):
		return http.StatusBadRequest, err.Error(), failureInvalidMessage
	case errors.Is(err, agent.ErrInvalidSession):
		return http.StatusBadRequest, err.Error(), failureInvalidSession
	case errors.Is(err, agent.ErrInvalidHistory):
		return http.StatusBadRequest, err.Error(), failureInvalidHistory
	case errors.Is(err, agent.ErrBusy):
		return http.StatusServiceUnavailable, "agent is busy; please retry shortly", failureAgentBusy
	case errors.Is(err, agent.ErrQueueTimeout):
		return http.StatusServiceUnavailable, "agent queue wait timed out; please retry shortly", failureQueueTimeout
	case errors.Is(err, context.Canceled):
		return http.StatusBadGateway, "agent execution failed", failureRequestCanceled
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return http.StatusGatewayTimeout, "agent execution timed out", failureExecutionTimeout
	default:
		return http.StatusBadGateway, "agent execution failed", failureExecutionFailed
	}
}

func writeResponse(c *gin.Context, status, code int, message string, failure failureCode, data any, audit *telemetry.AuditSummary) {
	c.JSON(status, responseEnvelope{Code: code, Message: message, FailureCode: failure, Data: data, Audit: audit})
}

func requestAudit(requestID string, snapshot *telemetry.Snapshot) *telemetry.AuditSummary {
	if requestID == "" && snapshot == nil {
		return nil
	}
	return &telemetry.AuditSummary{RequestID: requestID, Telemetry: snapshot}
}

// normalizeRequestID accepts only identifiers created by the main server. An
// invalid or legacy identifier is not echoed or used for write idempotency;
// Engine.Chat will create an internal fallback identifier when this is empty.
func normalizeRequestID(value string) string {
	if len(value) != 32 {
		return ""
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return ""
		}
	}
	return value
}
