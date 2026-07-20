package http

import (
	"SWPUCAT/internal/application/user"
	"SWPUCAT/internal/domain/shared"
	"fmt"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	userSvc *user.UserApplicationService
}

func NewAuthHandler(userSvc *user.UserApplicationService) *AuthHandler {
	return &AuthHandler{userSvc: userSvc}
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req user.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request body")
		return
	}

	resp, err := h.userSvc.Register(c.Request.Context(), req)
	if err != nil {
		if err == shared.ErrConflict {
			Conflict(c, "username already exists")
			return
		}
		InternalError(c, "registration failed")
		return
	}

	Created(c, resp)
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req user.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request body")
		return
	}

	resp, err := h.userSvc.Login(c.Request.Context(), req)
	if err != nil {
		Unauthorized(c, "invalid credentials")
		return
	}

	Success(c, resp)
}

func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request body")
		return
	}

	resp, err := h.userSvc.RefreshToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		Unauthorized(c, "invalid refresh token")
		return
	}

	Success(c, resp)
}

func (h *AuthHandler) SendVerificationCode(c *gin.Context) {
	var req user.SendCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request body")
		return
	}

	if err := h.userSvc.SendVerificationCode(c.Request.Context(), req.Email); err != nil {
		InternalError(c, fmt.Sprintf("failed to send verification code: %v", err))
		return
	}

	Success(c, nil)
}
