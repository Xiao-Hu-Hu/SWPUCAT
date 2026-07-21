package http

import (
	"SWPUCAT/internal/application/invitation"
	invitationDomain "SWPUCAT/internal/domain/invitation"

	"github.com/gin-gonic/gin"
)

type InvitationHandler struct {
	invitationSvc *invitation.InvitationService
}

func NewInvitationHandler(invitationSvc *invitation.InvitationService) *InvitationHandler {
	return &InvitationHandler{invitationSvc: invitationSvc}
}

type GenerateCodeRequest struct {
	Type string `json:"type" binding:"required"`
}

func (h *InvitationHandler) GenerateCode(c *gin.Context) {
	var req GenerateCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request body")
		return
	}

	creatorID := GetUserID(c)
	codeType := invitationDomain.InvitationType(req.Type)

	dto, err := h.invitationSvc.GenerateCode(c.Request.Context(), creatorID, codeType)
	if err != nil {
		Forbidden(c, err.Error())
		return
	}

	Created(c, dto)
}

func (h *InvitationHandler) GetMyCodes(c *gin.Context) {
	creatorID := GetUserID(c)

	codes, err := h.invitationSvc.GetMyCodes(c.Request.Context(), creatorID)
	if err != nil {
		InternalError(c, "failed to get invitation codes")
		return
	}

	Success(c, codes)
}
