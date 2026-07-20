package http

import (
	"SWPUCAT/internal/application/approval"
	"strconv"

	"github.com/gin-gonic/gin"
)

type ApprovalHandler struct {
	approvalSvc *approval.ApprovalService
}

func NewApprovalHandler(approvalSvc *approval.ApprovalService) *ApprovalHandler {
	return &ApprovalHandler{approvalSvc: approvalSvc}
}

func (h *ApprovalHandler) Submit(c *gin.Context) {
	var req struct {
		FileName   string `json:"file_name" binding:"required"`
		FileSize   string `json:"file_size"`
		FileKey    string `json:"file_key"`
		CategoryID int64  `json:"category_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request body")
		return
	}

	uploaderID := GetUserID(c)
	uploaderName := GetUsername(c)

	dto, err := h.approvalSvc.Submit(c.Request.Context(), req.FileName, req.FileSize, req.FileKey, req.CategoryID, uploaderID, uploaderName)
	if err != nil {
		InternalError(c, "failed to submit approval")
		return
	}

	Created(c, dto)
}

func (h *ApprovalHandler) Approve(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "invalid approval id")
		return
	}

	reviewerID := GetUserID(c)

	if err := h.approvalSvc.Approve(c.Request.Context(), id, reviewerID); err != nil {
		InternalError(c, "failed to approve")
		return
	}

	Success(c, nil)
}

func (h *ApprovalHandler) Reject(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "invalid approval id")
		return
	}

	reviewerID := GetUserID(c)

	if err := h.approvalSvc.Reject(c.Request.Context(), id, reviewerID); err != nil {
		InternalError(c, "failed to reject")
		return
	}

	Success(c, nil)
}

func (h *ApprovalHandler) ListPending(c *gin.Context) {
	approvals, err := h.approvalSvc.ListPending(c.Request.Context())
	if err != nil {
		InternalError(c, "failed to list approvals")
		return
	}

	Success(c, approvals)
}
