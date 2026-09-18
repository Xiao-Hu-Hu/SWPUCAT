package http

import (
	"errors"
	"strconv"
	"strings"
	"time"

	agentauditapp "SWPUCAT/internal/application/agentaudit"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AgentAuditAnalyticsHandler struct {
	service *agentauditapp.AnalyticsService
}

func NewAgentAuditAnalyticsHandler(service *agentauditapp.AnalyticsService) *AgentAuditAnalyticsHandler {
	return &AgentAuditAnalyticsHandler{service: service}
}

func (h *AgentAuditAnalyticsHandler) List(c *gin.Context) {
	query, ok := parseAgentAuditAnalyticsQuery(c)
	if !ok {
		return
	}
	result, err := h.service.List(c.Request.Context(), query)
	if err != nil {
		handleAgentAuditAnalyticsError(c, err)
		return
	}
	Success(c, result)
}

func (h *AgentAuditAnalyticsHandler) Summary(c *gin.Context) {
	query, ok := parseAgentAuditAnalyticsQuery(c)
	if !ok {
		return
	}
	result, err := h.service.Summary(c.Request.Context(), query)
	if err != nil {
		handleAgentAuditAnalyticsError(c, err)
		return
	}
	Success(c, result)
}

func (h *AgentAuditAnalyticsHandler) Detail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		BadRequest(c, "invalid audit run id")
		return
	}
	result, err := h.service.Detail(c.Request.Context(), id)
	if err != nil {
		handleAgentAuditAnalyticsError(c, err)
		return
	}
	Success(c, result)
}

func parseAgentAuditAnalyticsQuery(c *gin.Context) (agentauditapp.AnalyticsQuery, bool) {
	result := agentauditapp.AnalyticsQuery{Status: strings.TrimSpace(c.Query("status"))}
	for key, target := range map[string]*time.Time{"from": &result.From, "to": &result.To} {
		value := strings.TrimSpace(c.Query(key))
		if value == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			BadRequest(c, key+" must be an RFC3339 timestamp")
			return agentauditapp.AnalyticsQuery{}, false
		}
		*target = parsed
	}
	for key, target := range map[string]*int{"page": &result.Page, "page_size": &result.PageSize} {
		value := strings.TrimSpace(c.Query(key))
		if value == "" {
			continue
		}
		parsed, err := strconv.Atoi(value)
		if err != nil {
			BadRequest(c, key+" must be an integer")
			return agentauditapp.AnalyticsQuery{}, false
		}
		*target = parsed
	}
	return result, true
}

func handleAgentAuditAnalyticsError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, agentauditapp.ErrInvalidAnalyticsQuery):
		BadRequest(c, "invalid agent audit query")
	case errors.Is(err, gorm.ErrRecordNotFound):
		NotFound(c, "agent audit run not found")
	default:
		InternalError(c, "agent audit query failed")
	}
}
