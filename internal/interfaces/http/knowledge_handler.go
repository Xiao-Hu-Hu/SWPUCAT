package http

import (
	"SWPUCAT/internal/application/knowledge"
	"SWPUCAT/internal/domain/shared"
	"strconv"

	"github.com/gin-gonic/gin"
)

type KnowledgeHandler struct {
	knowledgeSvc *knowledge.KnowledgeService
}

func NewKnowledgeHandler(knowledgeSvc *knowledge.KnowledgeService) *KnowledgeHandler {
	return &KnowledgeHandler{knowledgeSvc: knowledgeSvc}
}

func (h *KnowledgeHandler) CreateLink(c *gin.Context) {
	var req knowledge.CreateLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request body")
		return
	}

	uploaderID := GetUserID(c)
	uploaderName := GetUsername(c)

	dto, err := h.knowledgeSvc.CreateLink(c.Request.Context(), uploaderID, uploaderName, req)
	if err != nil {
		InternalError(c, "failed to create link")
		return
	}

	Created(c, dto)
}

func (h *KnowledgeHandler) UploadFile(c *gin.Context) {
	var req knowledge.UploadFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request body")
		return
	}

	uploaderID := GetUserID(c)
	uploaderName := GetUsername(c)
	isCaptain := IsCaptain(c)

	dto, err := h.knowledgeSvc.UploadFile(c.Request.Context(), uploaderID, uploaderName, isCaptain, req)
	if err != nil {
		InternalError(c, "failed to upload file")
		return
	}

	Created(c, dto)
}

func (h *KnowledgeHandler) DeleteItem(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "invalid item id")
		return
	}

	userID := GetUserID(c)
	isCaptain := IsCaptain(c)

	if err := h.knowledgeSvc.DeleteItem(c.Request.Context(), id, userID, isCaptain); err != nil {
		if err == shared.ErrForbidden {
			Forbidden(c, "not allowed to delete this item")
			return
		}
		InternalError(c, "failed to delete item")
		return
	}

	Success(c, nil)
}

func (h *KnowledgeHandler) ListItems(c *gin.Context) {
	var categoryID *int64
	if catIDStr := c.Query("category_id"); catIDStr != "" {
		catID, err := strconv.ParseInt(catIDStr, 10, 64)
		if err == nil {
			categoryID = &catID
		}
	}

	search := c.Query("search")

	items, err := h.knowledgeSvc.ListItems(c.Request.Context(), categoryID, search)
	if err != nil {
		InternalError(c, "failed to list items")
		return
	}

	Success(c, items)
}

func (h *KnowledgeHandler) GetItem(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "invalid item id")
		return
	}

	item, err := h.knowledgeSvc.GetItem(c.Request.Context(), id)
	if err != nil {
		NotFound(c, "item not found")
		return
	}

	Success(c, item)
}

func (h *KnowledgeHandler) CreateCategory(c *gin.Context) {
	var req knowledge.CreateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request body")
		return
	}

	dto, err := h.knowledgeSvc.CreateCategory(c.Request.Context(), req.Name)
	if err != nil {
		InternalError(c, "failed to create category")
		return
	}

	Created(c, dto)
}

func (h *KnowledgeHandler) DeleteCategory(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "invalid category id")
		return
	}

	if err := h.knowledgeSvc.DeleteCategory(c.Request.Context(), id); err != nil {
		InternalError(c, "failed to delete category")
		return
	}

	Success(c, nil)
}

func (h *KnowledgeHandler) ListCategories(c *gin.Context) {
	cats, err := h.knowledgeSvc.ListCategories(c.Request.Context())
	if err != nil {
		InternalError(c, "failed to list categories")
		return
	}

	Success(c, cats)
}
