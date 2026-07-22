package http

import (
	"SWPUCAT/internal/application/knowledge"
	knowledgeDomain "SWPUCAT/internal/domain/knowledge"
	"SWPUCAT/internal/domain/shared"
	"SWPUCAT/internal/infrastructure/storage"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
)

type KnowledgeHandler struct {
	knowledgeSvc *knowledge.KnowledgeService
	storage      *storage.LocalStorage
}

func NewKnowledgeHandler(knowledgeSvc *knowledge.KnowledgeService, storage *storage.LocalStorage) *KnowledgeHandler {
	return &KnowledgeHandler{knowledgeSvc: knowledgeSvc, storage: storage}
}

func (h *KnowledgeHandler) CreateLink(c *gin.Context) {
	var req knowledge.CreateLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request body")
		return
	}

	uploaderID := GetUserID(c)
	uploaderName := GetUsername(c)
	isCaptain := IsCaptain(c)

	dto, err := h.knowledgeSvc.CreateLink(c.Request.Context(), uploaderID, uploaderName, isCaptain, req)
	if err != nil {
		InternalError(c, "failed to create link")
		return
	}

	Created(c, dto)
}

func (h *KnowledgeHandler) UploadFile(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		BadRequest(c, "file is required")
		return
	}
	defer file.Close()

	// Validate file type
	if !knowledgeDomain.IsAllowedFileType(header.Filename) {
		BadRequest(c, "file type not allowed")
		return
	}

	// Get category ID from form
	categoryIDStr := c.PostForm("category_id")
	categoryID, err := strconv.ParseInt(categoryIDStr, 10, 64)
	if err != nil {
		BadRequest(c, "invalid category_id")
		return
	}

	// Save file to storage
	fileKey, err := h.storage.Save(header.Filename, file)
	if err != nil {
		InternalError(c, "failed to save file")
		return
	}

	// Format file size
	fileSize := fmt.Sprintf("%.2f MB", float64(header.Size)/(1024*1024))

	// Get description from form
	description := c.PostForm("description")

	uploaderID := GetUserID(c)
	uploaderName := GetUsername(c)
	isCaptain := IsCaptain(c)

	req := knowledge.UploadFileRequest{
		FileName:    header.Filename,
		Description: description,
		FileSize:    fileSize,
		FileKey:     fileKey,
		CategoryID:  categoryID,
	}

	dto, err := h.knowledgeSvc.UploadFile(c.Request.Context(), uploaderID, uploaderName, isCaptain, req)
	if err != nil {
		// Clean up saved file on error
		h.storage.Delete(fileKey)
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

func (h *KnowledgeHandler) ListPendingItems(c *gin.Context) {
	items, err := h.knowledgeSvc.ListPendingItems(c.Request.Context())
	if err != nil {
		InternalError(c, "failed to list pending items")
		return
	}

	Success(c, items)
}

func (h *KnowledgeHandler) ListUserItems(c *gin.Context) {
	userID := GetUserID(c)
	items, err := h.knowledgeSvc.ListUserItems(c.Request.Context(), userID)
	if err != nil {
		InternalError(c, "failed to list user items")
		return
	}

	Success(c, items)
}

func (h *KnowledgeHandler) ApproveItem(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "invalid item id")
		return
	}

	if !IsCaptain(c) {
		Forbidden(c, "only captain or super admin can approve items")
		return
	}

	reviewerID := GetUserID(c)
	reviewerName := GetUsername(c)

	if err := h.knowledgeSvc.ApproveItem(c.Request.Context(), id, reviewerID, reviewerName); err != nil {
		InternalError(c, "failed to approve item")
		return
	}

	Success(c, nil)
}

func (h *KnowledgeHandler) RejectItem(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "invalid item id")
		return
	}

	if !IsCaptain(c) {
		Forbidden(c, "only captain or super admin can reject items")
		return
	}

	var req struct {
		Reason string `json:"reason" validate:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Reason == "" {
		BadRequest(c, "reject reason is required")
		return
	}

	reviewerID := GetUserID(c)
	reviewerName := GetUsername(c)

	fileKey, err := h.knowledgeSvc.RejectItem(c.Request.Context(), id, reviewerID, reviewerName, req.Reason)
	if err != nil {
		InternalError(c, "failed to reject item")
		return
	}

	// Delete file from storage if it exists
	if fileKey != "" {
		h.storage.Delete(fileKey)
	}

	Success(c, nil)
}

func (h *KnowledgeHandler) DownloadFile(c *gin.Context) {
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

	if item.Type != "file" {
		BadRequest(c, "item is not a file")
		return
	}

	// Get file path from storage using the file key
	filePath, err := h.storage.Get(item.FileKey)
	if err != nil {
		NotFound(c, "file not found")
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", item.Name))
	c.Header("Content-Type", "application/octet-stream")
	c.File(filePath)
}
