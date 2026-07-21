package http

import (
	"SWPUCAT/internal/application/user"
	"SWPUCAT/internal/domain/shared"
	"strconv"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	userSvc *user.UserApplicationService
}

func NewUserHandler(userSvc *user.UserApplicationService) *UserHandler {
	return &UserHandler{userSvc: userSvc}
}

func (h *UserHandler) GetProfile(c *gin.Context) {
	userID := GetUserID(c)

	dto, err := h.userSvc.GetUser(c.Request.Context(), userID)
	if err != nil {
		NotFound(c, "user not found")
		return
	}

	Success(c, dto)
}

func (h *UserHandler) ChangePassword(c *gin.Context) {
	userID := GetUserID(c)

	var req user.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request body")
		return
	}

	if err := h.userSvc.ChangePassword(c.Request.Context(), userID, req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	Success(c, nil)
}

func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userID := GetUserID(c)

	var req struct {
		Nickname string `json:"nickname" validate:"required,max=32"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Nickname == "" {
		BadRequest(c, "invalid request body")
		return
	}

	if err := h.userSvc.UpdateNickname(c.Request.Context(), userID, req.Nickname); err != nil {
		InternalError(c, "failed to update profile")
		return
	}

	Success(c, nil)
}

func (h *UserHandler) ListMembers(c *gin.Context) {
	members, err := h.userSvc.ListMembers(c.Request.Context())
	if err != nil {
		InternalError(c, "failed to list members")
		return
	}

	Success(c, members)
}

func (h *UserHandler) TransferCaptain(c *gin.Context) {
	operatorID := GetUserID(c)

	targetIDStr := c.Param("id")
	targetID, err := strconv.ParseInt(targetIDStr, 10, 64)
	if err != nil {
		BadRequest(c, "invalid user id")
		return
	}

	if err := h.userSvc.TransferCaptain(c.Request.Context(), operatorID, targetID); err != nil {
		if err == shared.ErrForbidden {
			Forbidden(c, "only captain can transfer role")
			return
		}
		InternalError(c, "transfer failed")
		return
	}

	Success(c, nil)
}

func (h *UserHandler) RemoveMember(c *gin.Context) {
	operatorID := GetUserID(c)

	targetIDStr := c.Param("id")
	targetID, err := strconv.ParseInt(targetIDStr, 10, 64)
	if err != nil {
		BadRequest(c, "invalid user id")
		return
	}

	if err := h.userSvc.RemoveMember(c.Request.Context(), operatorID, targetID); err != nil {
		if err == shared.ErrForbidden {
			Forbidden(c, "cannot remove captain")
			return
		}
		InternalError(c, "remove failed")
		return
	}

	Success(c, nil)
}
