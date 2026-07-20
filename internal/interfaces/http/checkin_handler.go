package http

import (
	"SWPUCAT/internal/application/checkin"
	"SWPUCAT/internal/domain/shared"
	"strconv"

	"github.com/gin-gonic/gin"
)

type CheckinHandler struct {
	checkinSvc *checkin.CheckinService
}

func NewCheckinHandler(checkinSvc *checkin.CheckinService) *CheckinHandler {
	return &CheckinHandler{checkinSvc: checkinSvc}
}

func (h *CheckinHandler) ClockIn(c *gin.Context) {
	userID := GetUserID(c)

	dto, err := h.checkinSvc.ClockIn(c.Request.Context(), userID)
	if err != nil {
		if err == shared.ErrConflict {
			Conflict(c, "already clocked in today")
			return
		}
		InternalError(c, "clock in failed")
		return
	}

	Success(c, dto)
}

func (h *CheckinHandler) ClockOut(c *gin.Context) {
	userID := GetUserID(c)

	dto, err := h.checkinSvc.ClockOut(c.Request.Context(), userID)
	if err != nil {
		if err == shared.ErrConflict {
			Conflict(c, "not clocked in")
			return
		}
		InternalError(c, "clock out failed")
		return
	}

	Success(c, dto)
}

func (h *CheckinHandler) GetRecords(c *gin.Context) {
	userID := GetUserID(c)

	limitStr := c.DefaultQuery("limit", "10")
	limit, _ := strconv.Atoi(limitStr)

	records, err := h.checkinSvc.GetRecords(c.Request.Context(), userID, limit)
	if err != nil {
		InternalError(c, "failed to get records")
		return
	}

	Success(c, records)
}

func (h *CheckinHandler) GetStatus(c *gin.Context) {
	userID := GetUserID(c)

	status, err := h.checkinSvc.GetStatus(c.Request.Context(), userID)
	if err != nil {
		InternalError(c, "failed to get status")
		return
	}

	Success(c, status)
}
