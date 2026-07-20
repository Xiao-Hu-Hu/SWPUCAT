package http

import (
	"SWPUCAT/internal/application/dashboard"

	"github.com/gin-gonic/gin"
)

type DashboardHandler struct {
	dashboardSvc *dashboard.DashboardService
}

func NewDashboardHandler(dashboardSvc *dashboard.DashboardService) *DashboardHandler {
	return &DashboardHandler{dashboardSvc: dashboardSvc}
}

func (h *DashboardHandler) GetDashboard(c *gin.Context) {
	dto, err := h.dashboardSvc.GetDashboard(c.Request.Context())
	if err != nil {
		InternalError(c, "failed to get dashboard")
		return
	}

	Success(c, dto)
}
