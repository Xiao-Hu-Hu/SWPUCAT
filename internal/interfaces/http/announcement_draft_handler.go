package http

import (
	announcementdraftapp "SWPUCAT/internal/application/announcementdraft"
	"SWPUCAT/internal/domain/announcementdraft"
	"SWPUCAT/internal/domain/shared"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AnnouncementDraftHandler exposes the human approval boundary for Agent
// generated announcements. The Agent only calls Create; confirmation and
// publication remain manager actions on these endpoints.
type AnnouncementDraftHandler struct {
	service *announcementdraftapp.Service
}

func NewAnnouncementDraftHandler(service *announcementdraftapp.Service) *AnnouncementDraftHandler {
	return &AnnouncementDraftHandler{service: service}
}

func (h *AnnouncementDraftHandler) Create(c *gin.Context) {
	var req announcementdraftapp.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid announcement draft request")
		return
	}
	item, err := h.service.Create(c.Request.Context(), GetUserID(c), req)
	if err != nil {
		handleAnnouncementDraftError(c, err)
		return
	}
	Created(c, item)
}

func (h *AnnouncementDraftHandler) Get(c *gin.Context) {
	id, ok := announcementDraftIDParam(c)
	if !ok {
		return
	}
	item, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		handleAnnouncementDraftError(c, err)
		return
	}
	Success(c, item)
}

func (h *AnnouncementDraftHandler) List(c *gin.Context) {
	items, err := h.service.List(c.Request.Context(), c.Query("status"))
	if err != nil {
		handleAnnouncementDraftError(c, err)
		return
	}
	Success(c, items)
}

func (h *AnnouncementDraftHandler) Update(c *gin.Context) {
	id, ok := announcementDraftIDParam(c)
	if !ok {
		return
	}
	var req announcementdraftapp.UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid announcement draft update")
		return
	}
	item, err := h.service.Update(c.Request.Context(), GetUserID(c), id, req)
	if err != nil {
		handleAnnouncementDraftError(c, err)
		return
	}
	Success(c, item)
}

func (h *AnnouncementDraftHandler) Confirm(c *gin.Context) {
	id, ok := announcementDraftIDParam(c)
	if !ok {
		return
	}
	item, err := h.service.Confirm(c.Request.Context(), GetUserID(c), id)
	if err != nil {
		handleAnnouncementDraftError(c, err)
		return
	}
	Success(c, item)
}

func (h *AnnouncementDraftHandler) Publish(c *gin.Context) {
	id, ok := announcementDraftIDParam(c)
	if !ok {
		return
	}
	item, err := h.service.Publish(c.Request.Context(), GetUserID(c), GetUsername(c), id)
	if err != nil {
		handleAnnouncementDraftError(c, err)
		return
	}
	Success(c, item)
}

func (h *AnnouncementDraftHandler) Discard(c *gin.Context) {
	id, ok := announcementDraftIDParam(c)
	if !ok {
		return
	}
	if err := h.service.Discard(c.Request.Context(), GetUserID(c), id); err != nil {
		handleAnnouncementDraftError(c, err)
		return
	}
	Success(c, nil)
}

func announcementDraftIDParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		BadRequest(c, "invalid announcement draft id")
		return 0, false
	}
	return id, true
}

func handleAnnouncementDraftError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		NotFound(c, "announcement draft not found")
	case errors.Is(err, announcementdraft.ErrInvalidDraft), errors.Is(err, shared.ErrConflict):
		Conflict(c, "announcement draft state conflicts with this request")
	case errors.Is(err, shared.ErrValidation):
		BadRequest(c, "invalid announcement draft data")
	default:
		InternalError(c, "announcement draft operation failed")
	}
}
