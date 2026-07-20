package http

import (
	"SWPUCAT/internal/application/announcement"
	"strconv"

	"github.com/gin-gonic/gin"
)

type AnnouncementHandler struct {
	annSvc *announcement.AnnouncementService
}

func NewAnnouncementHandler(annSvc *announcement.AnnouncementService) *AnnouncementHandler {
	return &AnnouncementHandler{annSvc: annSvc}
}

func (h *AnnouncementHandler) Create(c *gin.Context) {
	var req announcement.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request body")
		return
	}

	operatorID := GetUserID(c)
	operatorName := GetUsername(c)

	dto, err := h.annSvc.Create(c.Request.Context(), operatorID, operatorName, req)
	if err != nil {
		InternalError(c, "failed to create announcement")
		return
	}

	Created(c, dto)
}

func (h *AnnouncementHandler) Update(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "invalid announcement id")
		return
	}

	var req announcement.UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request body")
		return
	}

	if err := h.annSvc.Update(c.Request.Context(), id, req); err != nil {
		InternalError(c, "failed to update announcement")
		return
	}

	Success(c, nil)
}

func (h *AnnouncementHandler) Delete(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "invalid announcement id")
		return
	}

	if err := h.annSvc.Delete(c.Request.Context(), id); err != nil {
		InternalError(c, "failed to delete announcement")
		return
	}

	Success(c, nil)
}

func (h *AnnouncementHandler) List(c *gin.Context) {
	anns, err := h.annSvc.List(c.Request.Context())
	if err != nil {
		InternalError(c, "failed to list announcements")
		return
	}

	Success(c, anns)
}
