package http

import (
	evalapp "SWPUCAT/internal/application/evaluation"
	evaldomain "SWPUCAT/internal/domain/evaluation"
	"SWPUCAT/internal/domain/shared"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type EvaluationHandler struct{ service *evalapp.Service }

func NewEvaluationHandler(service *evalapp.Service) *EvaluationHandler {
	return &EvaluationHandler{service: service}
}

func (h *EvaluationHandler) CreateDraft(c *gin.Context) {
	var req evalapp.CreateDraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid evaluation draft request")
		return
	}
	item, err := h.service.CreateDraft(c.Request.Context(), GetUserID(c), req)
	if err != nil {
		handleEvaluationError(c, err)
		return
	}
	Created(c, item)
}

func (h *EvaluationHandler) GetDraft(c *gin.Context) {
	id, ok := evaluationIDParam(c)
	if !ok {
		return
	}
	item, err := h.service.GetDraft(c.Request.Context(), id)
	if err != nil {
		handleEvaluationError(c, err)
		return
	}
	Success(c, item)
}

func (h *EvaluationHandler) UpdateDraft(c *gin.Context) {
	id, ok := evaluationIDParam(c)
	if !ok {
		return
	}
	var req evalapp.UpdateDraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid evaluation draft update")
		return
	}
	item, err := h.service.UpdateDraft(c.Request.Context(), id, GetUserID(c), req)
	if err != nil {
		handleEvaluationError(c, err)
		return
	}
	Success(c, item)
}

func (h *EvaluationHandler) ReviewDraft(c *gin.Context) {
	id, ok := evaluationIDParam(c)
	if !ok {
		return
	}
	var req struct {
		Approved bool `json:"approved"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid evaluation review request")
		return
	}
	item, err := h.service.ReviewDraft(c.Request.Context(), id, GetUserID(c), req.Approved)
	if err != nil {
		handleEvaluationError(c, err)
		return
	}
	Success(c, item)
}

func (h *EvaluationHandler) PublishDraft(c *gin.Context) {
	id, ok := evaluationIDParam(c)
	if !ok {
		return
	}
	item, err := h.service.PublishDraft(c.Request.Context(), id, GetUserID(c))
	if err != nil {
		handleEvaluationError(c, err)
		return
	}
	Success(c, item)
}

func (h *EvaluationHandler) SaveMemberComment(c *gin.Context) {
	draftID, ok := evaluationIDParam(c)
	if !ok {
		return
	}
	userID, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil || userID <= 0 {
		BadRequest(c, "invalid member id")
		return
	}
	var req evalapp.MemberCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid evaluation member comment")
		return
	}
	if err := h.service.SaveMemberComment(c.Request.Context(), draftID, GetUserID(c), userID, req); err != nil {
		handleEvaluationError(c, err)
		return
	}
	Success(c, nil)
}

func evaluationIDParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		BadRequest(c, "invalid evaluation id")
		return 0, false
	}
	return id, true
}

func handleEvaluationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, shared.ErrForbidden):
		Forbidden(c, "not allowed to access this evaluation")
	case errors.Is(err, shared.ErrConflict), errors.Is(err, evaldomain.ErrSnapshotChanged):
		Conflict(c, "evaluation state conflicts with this request")
	case errors.Is(err, shared.ErrValidation), errors.Is(err, evaldomain.ErrInvalidSnapshot):
		BadRequest(c, "invalid evaluation data")
	case errors.Is(err, gorm.ErrRecordNotFound):
		NotFound(c, "evaluation not found")
	default:
		InternalError(c, "evaluation operation failed")
	}
}
