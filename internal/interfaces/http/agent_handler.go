package http

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"SWPUCAT/internal/infrastructure/agentclient"

	"github.com/gin-gonic/gin"
)

type AgentHandler struct {
	client *agentclient.Client
}

func NewAgentHandler(client *agentclient.Client) *AgentHandler {
	return &AgentHandler{client: client}
}

type agentChatRequest struct {
	Message   string `json:"message"`
	SessionID string `json:"session_id,omitempty"`
}

func (h *AgentHandler) Chat(c *gin.Context) {
	if h == nil || h.client == nil {
		Error(c, http.StatusServiceUnavailable, 503, "agent service is not configured")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	var req agentChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request body")
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" || utf8.RuneCountInString(req.Message) > 4000 {
		BadRequest(c, "message is required and must be at most 4000 characters")
		return
	}
	if utf8.RuneCountInString(req.SessionID) > 128 {
		BadRequest(c, "session_id must be at most 128 characters")
		return
	}

	result, err := h.client.Chat(c.Request.Context(), agentclient.ChatRequest{
		Message:   req.Message,
		SessionID: strings.TrimSpace(req.SessionID),
		UserID:    GetUserID(c),
		Role:      GetRole(c),
	}, c.GetHeader("Authorization"), c.GetHeader("X-Request-ID"))
	if err != nil {
		var httpErr *agentclient.HTTPError
		if errors.As(err, &httpErr) {
			switch httpErr.StatusCode {
			case http.StatusBadRequest:
				BadRequest(c, httpErr.Message)
			case http.StatusUnauthorized:
				Unauthorized(c, "agent authentication failed")
			case http.StatusForbidden:
				Forbidden(c, "agent access denied")
			case http.StatusServiceUnavailable:
				Error(c, http.StatusServiceUnavailable, 503, "agent service is unavailable")
			default:
				Error(c, http.StatusBadGateway, 502, "agent service request failed")
			}
			return
		}
		InternalError(c, "agent service request failed")
		return
	}
	Success(c, result)
}
