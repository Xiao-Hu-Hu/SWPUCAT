package agenthttp

import (
	"errors"
	"net/http"

	"SWPUCAT/internal/agent"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	engine         *agent.Engine
	internalSecret string
}

func NewHandler(engine *agent.Engine, internalSecret string) *Handler {
	return &Handler{engine: engine, internalSecret: internalSecret}
}

func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "enabled": h.engine.Enabled()})
}

func (h *Handler) Chat(c *gin.Context) {
	if h.engine.Enabled() && (h.internalSecret == "" || c.GetHeader("X-Agent-Internal-Secret") != h.internalSecret) {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "invalid internal secret"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	var req agent.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
	}
	req.Authorization = c.GetHeader("Authorization")
	req.RequestID = c.GetHeader("X-Request-ID")
	result, err := h.engine.Chat(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, agent.ErrDisabled):
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "agent is disabled"})
		case errors.Is(err, agent.ErrInvalidMessage):
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		case errors.Is(err, agent.ErrInvalidSession), errors.Is(err, agent.ErrInvalidHistory):
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		case errors.Is(err, agent.ErrBusy):
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "agent is busy; please retry shortly"})
		default:
			c.JSON(http.StatusBadGateway, gin.H{"code": 502, "message": "agent execution failed"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": result})
}
