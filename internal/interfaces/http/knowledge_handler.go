package http

import (
	"SWPUCAT/internal/application/knowledge"
	"SWPUCAT/internal/domain/shared"
	"SWPUCAT/internal/infrastructure/storage"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

// maxUploadSize 单个文件上传大小上限（1GB）
const maxUploadSize = 1 << 30

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
	// The file limit excludes multipart headers and bounded metadata fields.
	const maxRequestSize = maxUploadSize + (1 << 20)
	if c.Request.ContentLength > maxRequestSize {
		RequestTooLarge(c, "file too large: maximum 1GB")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestSize)
	pending, req, err := h.receiveUpload(c.Request, maxUploadSize)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		var pathErr *os.PathError
		if errors.Is(err, storage.ErrFileTooLarge) || errors.As(err, &maxBytesErr) {
			RequestTooLarge(c, "file too large: maximum 1GB")
		} else if errors.As(err, &pathErr) {
			InternalError(c, "failed to save file")
		} else {
			BadRequest(c, "invalid or incomplete upload")
		}
		return
	}
	defer pending.Discard()
	fileKey, err := pending.Commit(c.Request.Context())
	if err != nil {
		InternalError(c, "failed to save file")
		return
	}
	req.FileKey = fileKey
	uploaderID := GetUserID(c)
	uploaderName := GetUsername(c)
	isCaptain := IsCaptain(c)
	dto, err := h.knowledgeSvc.UploadFile(c.Request.Context(), uploaderID, uploaderName, isCaptain, req)
	if err != nil {
		// Clean up saved file on error
		h.storage.Delete(fileKey)
		InternalError(c, "failed to upload file")
		return
	}

	Created(c, dto)
}

// Fields may follow the file, as they do in existing clients. Keep the file
// staged until all parts and metadata have been received and validated.
func (h *KnowledgeHandler) receiveUpload(r *http.Request, limit int64) (*storage.PendingFile, knowledge.UploadFileRequest, error) {
	var req knowledge.UploadFileRequest
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, req, err
	}
	var pending *storage.PendingFile
	keep := false
	defer func() {
		if pending != nil && !keep {
			pending.Discard()
		}
	}()
	fields := make(map[string]string)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, req, err
		}
		name := part.FormName()
		if name == "file" {
			if pending != nil || part.FileName() == "" {
				return nil, req, errors.New("exactly one file is required")
			}
			req.FileName = part.FileName()
			pending, err = h.storage.Stage(r.Context(), req.FileName, part, limit)
			if err != nil {
				return nil, req, err
			}
		} else {
			if (name != "category_id" && name != "description") || part.FileName() != "" {
				return nil, req, errors.New("unexpected upload field")
			}
			if _, exists := fields[name]; exists {
				return nil, req, errors.New("duplicate upload field")
			}
			const maxFieldSize = 4 << 10
			value, err := io.ReadAll(io.LimitReader(part, maxFieldSize+1))
			if err != nil {
				return nil, req, err
			}
			if len(value) > maxFieldSize {
				return nil, req, errors.New("upload field too large")
			}
			fields[name] = string(value)
		}
		if err := part.Close(); err != nil {
			return nil, req, err
		}
	}
	if pending == nil {
		return nil, req, errors.New("file is required")
	}
	req.CategoryID, err = strconv.ParseInt(fields["category_id"], 10, 64)
	if err != nil || req.CategoryID <= 0 {
		return nil, req, errors.New("invalid category_id")
	}
	req.Description = fields["description"]
	if !utf8.ValidString(req.Description) || utf8.RuneCountInString(req.Description) > 1000 {
		return nil, req, errors.New("description exceeds 1000 characters")
	}
	if err := r.Context().Err(); err != nil {
		return nil, req, err
	}
	req.FileSize = fmt.Sprintf("%.2f MB", float64(pending.Size)/(1024*1024))
	keep = true
	return pending, req, nil
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

	// 使用 RFC 5987 编码处理中文文件名
	encodedName := url.QueryEscape(item.Name)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, item.Name, encodedName))
	c.Header("Content-Type", "application/octet-stream")
	c.File(filePath)
}
