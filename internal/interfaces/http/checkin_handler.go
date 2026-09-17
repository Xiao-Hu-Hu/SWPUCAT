package http

import (
	"SWPUCAT/internal/application/announcement"
	"SWPUCAT/internal/application/checkin"
	domaincheckin "SWPUCAT/internal/domain/checkin"
	"SWPUCAT/internal/domain/shared"
	"SWPUCAT/internal/domain/user"
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type CheckinHandler struct {
	checkinSvc *checkin.CheckinService
	annSvc     *announcement.AnnouncementService
}

func NewCheckinHandler(checkinSvc *checkin.CheckinService, annSvc *announcement.AnnouncementService) *CheckinHandler {
	return &CheckinHandler{checkinSvc: checkinSvc, annSvc: annSvc}
}

func (h *CheckinHandler) ClockIn(c *gin.Context) {
	userID := GetUserID(c)

	idempotencyKey, ok := checkinIdempotencyKey(c)
	if !ok {
		return
	}
	dto, err := h.checkinSvc.ClockIn(c.Request.Context(), userID, idempotencyKey)
	if err != nil {
		if errors.Is(err, domaincheckin.ErrAlreadyClockedIn) {
			Conflict(c, "already clocked in today")
			return
		}
		if errors.Is(err, shared.ErrForbidden) {
			Forbidden(c, "account is disabled")
			return
		}
		InternalError(c, "clock in failed")
		return
	}

	Success(c, dto)
}

func (h *CheckinHandler) ClockOut(c *gin.Context) {
	userID := GetUserID(c)

	idempotencyKey, ok := checkinIdempotencyKey(c)
	if !ok {
		return
	}
	dto, err := h.checkinSvc.ClockOut(c.Request.Context(), userID, idempotencyKey)
	if err != nil {
		if errors.Is(err, domaincheckin.ErrNotClockedIn) {
			Conflict(c, "not clocked in")
			return
		}
		if errors.Is(err, shared.ErrForbidden) {
			Forbidden(c, "account is disabled")
			return
		}
		InternalError(c, "clock out failed")
		return
	}

	Success(c, dto)
}

func checkinIdempotencyKey(c *gin.Context) (string, bool) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if len(key) > 128 {
		BadRequest(c, "Idempotency-Key must be at most 128 characters")
		return "", false
	}
	return key, true
}

func (h *CheckinHandler) GetRecords(c *gin.Context) {
	userID := GetUserID(c)

	limitStr := c.DefaultQuery("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		BadRequest(c, "limit must be a positive integer")
		return
	}
	if limit > 100 {
		limit = 100
	}

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

func (h *CheckinHandler) GetStats(c *gin.Context) {
	period := c.DefaultQuery("period", "week")
	stats, err := h.checkinSvc.GetStatsByPeriod(c.Request.Context(), period)
	if err != nil {
		InternalError(c, "failed to get stats")
		return
	}

	Success(c, stats)
}

func (h *CheckinHandler) GetPersonalWorkHours(c *gin.Context) {
	result, err := h.checkinSvc.GetPersonalWorkHours(c.Request.Context(), GetUserID(c))
	if err != nil {
		InternalError(c, "failed to get personal work hours")
		return
	}
	Success(c, result)
}

func (h *CheckinHandler) GetOnlineMembers(c *gin.Context) {
	members, err := h.checkinSvc.GetOnlineMembers(c.Request.Context())
	if err != nil {
		InternalError(c, "failed to get online members")
		return
	}

	Success(c, members)
}

func (h *CheckinHandler) GetAllTodayRecords(c *gin.Context) {
	records, err := h.checkinSvc.GetAllTodayRecords(c.Request.Context())
	if err != nil {
		InternalError(c, "failed to get today records")
		return
	}

	Success(c, records)
}

func (h *CheckinHandler) Makeup(c *gin.Context) {
	role := GetRole(c)
	if role != "captain" && role != "super_admin" {
		Forbidden(c, "captain or super_admin required")
		return
	}

	var req checkin.MakeupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request body")
		return
	}

	if err := h.checkinSvc.Makeup(c.Request.Context(), req); err != nil {
		if errors.Is(err, shared.ErrValidation) {
			BadRequest(c, err.Error())
			return
		}
		if errors.Is(err, shared.ErrConflict) {
			Conflict(c, "checkin records already exist for this date")
			return
		}
		if errors.Is(err, shared.ErrForbidden) {
			Forbidden(c, "account is disabled")
			return
		}
		if errors.Is(err, user.ErrUserNotFound) {
			NotFound(c, "user not found")
			return
		}
		InternalError(c, err.Error())
		return
	}

	Success(c, nil)
}

func (h *CheckinHandler) GetRequirements(c *gin.Context) {
	result := h.checkinSvc.GetRequirements(c.Request.Context())
	Success(c, result)
}

func (h *CheckinHandler) SetRequirements(c *gin.Context) {
	role := GetRole(c)
	if role != "captain" && role != "super_admin" {
		Forbidden(c, "captain or super_admin required")
		return
	}

	var req checkin.RequirementsDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request body")
		return
	}

	if err := h.checkinSvc.SetRequirements(c.Request.Context(), req); err != nil {
		InternalError(c, "failed to save requirements")
		return
	}

	Success(c, nil)
}

func (h *CheckinHandler) PublishReport(c *gin.Context) {
	role := GetRole(c)
	if role != "captain" && role != "super_admin" {
		Forbidden(c, "captain or super_admin required")
		return
	}

	operatorID := GetUserID(c)
	operatorName := GetUsername(c)

	title, content, err := h.checkinSvc.PublishReport(c.Request.Context())
	if err != nil {
		InternalError(c, "failed to generate report")
		return
	}

	dto, err := h.annSvc.Create(c.Request.Context(), operatorID, operatorName, announcement.CreateRequest{
		Title:   title,
		Content: content,
	})
	if err != nil {
		InternalError(c, "failed to publish announcement")
		return
	}

	Success(c, dto)
}
