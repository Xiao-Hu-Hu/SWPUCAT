package http

import (
	"SWPUCAT/internal/application/teamquery"
	"SWPUCAT/internal/domain/shared"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
)

type TeamQueryHandler struct{ service *teamquery.Service }

func NewTeamQueryHandler(service *teamquery.Service) *TeamQueryHandler {
	return &TeamQueryHandler{service: service}
}

func (h *TeamQueryHandler) Query(c *gin.Context) {
	var minHours *float64
	if value := c.Query("min_hours"); value != "" {
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			BadRequest(c, "invalid min_hours")
			return
		}
		minHours = &parsed
	}
	periodID := int64(0)
	if value := c.Query("period_id"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 0 {
			BadRequest(c, "invalid period_id")
			return
		}
		periodID = parsed
	}
	result, err := h.service.Query(c.Request.Context(), teamquery.Request{Period: c.Query("period"), PeriodID: periodID, MinHours: minHours, ReportStatus: c.Query("report_status"), Limit: positiveQueryInt(c, "limit", 100)})
	if err != nil {
		if errors.Is(err, shared.ErrValidation) {
			BadRequest(c, "invalid team query")
			return
		}
		InternalError(c, "team query failed")
		return
	}
	Success(c, result)
}
