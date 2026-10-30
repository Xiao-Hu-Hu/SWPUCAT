package http

import (
	notificationapp "SWPUCAT/internal/application/notification"
	"SWPUCAT/internal/domain/notification"
	"SWPUCAT/internal/domain/shared"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type NotificationHandler struct{ service *notificationapp.Service }

func NewNotificationHandler(service *notificationapp.Service) *NotificationHandler {
	return &NotificationHandler{service: service}
}

func (h *NotificationHandler) List(c *gin.Context) {
	unreadOnly := strings.EqualFold(c.Query("unread"), "true") || c.Query("unread") == "1"
	page, pageSize := positiveQueryInt(c, "page", 1), positiveQueryInt(c, "page_size", 20)
	result, err := h.service.List(c.Request.Context(), GetUserID(c), unreadOnly, page, pageSize)
	if err != nil {
		handleNotificationError(c, err)
		return
	}
	Success(c, result)
}

func (h *NotificationHandler) MarkRead(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		BadRequest(c, "invalid notification id")
		return
	}
	if err := h.service.MarkRead(c.Request.Context(), GetUserID(c), id); err != nil {
		handleNotificationError(c, err)
		return
	}
	Success(c, nil)
}

func (h *NotificationHandler) GetPreferences(c *gin.Context) {
	result, err := h.service.GetPreferences(c.Request.Context(), GetUserID(c))
	if err != nil {
		handleNotificationError(c, err)
		return
	}
	Success(c, result)
}

func (h *NotificationHandler) UpdatePreferences(c *gin.Context) {
	var req notificationapp.UpdatePreferencesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid notification preferences")
		return
	}
	result, err := h.service.UpdatePreferences(c.Request.Context(), GetUserID(c), req)
	if err != nil {
		handleNotificationError(c, err)
		return
	}
	Success(c, result)
}

func positiveQueryInt(c *gin.Context, name string, fallback int) int {
	value := c.Query(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}

func handleNotificationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, shared.ErrValidation), errors.Is(err, notification.ErrInvalidNotification):
		BadRequest(c, "invalid notification request")
	case errors.Is(err, notification.ErrAlreadyRead):
		Success(c, nil)
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, shared.ErrNotFound):
		NotFound(c, "notification not found")
	default:
		Error(c, http.StatusInternalServerError, 500, "notification operation failed")
	}
}
