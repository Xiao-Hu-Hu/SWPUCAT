package http

import (
	"SWPUCAT/internal/application/knowledge"
	"SWPUCAT/internal/application/knowledgeindex"
	knowledgedomain "SWPUCAT/internal/domain/knowledge"
	"SWPUCAT/internal/domain/shared"
	"SWPUCAT/internal/infrastructure/storage"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

const (
	// maxUploadSize 单个文件上传大小上限（1GB）
	maxUploadSize                 = 1 << 30
	maxKnowledgeDescriptionRunes  = 20000
	maxKnowledgeMetadataFieldSize = maxKnowledgeDescriptionRunes*utf8.UTFMax + 1024
)

type KnowledgeHandler struct {
	knowledgeSvc *knowledge.KnowledgeService
	indexSvc     *knowledgeindex.Service
	storage      *storage.LocalStorage
}

func NewKnowledgeHandler(knowledgeSvc *knowledge.KnowledgeService, storage *storage.LocalStorage, indexServices ...*knowledgeindex.Service) *KnowledgeHandler {
	var indexSvc *knowledgeindex.Service
	if len(indexServices) > 0 {
		indexSvc = indexServices[0]
	}
	return &KnowledgeHandler{knowledgeSvc: knowledgeSvc, indexSvc: indexSvc, storage: storage}
}

func (h *KnowledgeHandler) CreateLink(c *gin.Context) {
	var req knowledge.CreateLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request body")
		return
	}
	if !validKnowledgeDescription(req.Description) {
		BadRequest(c, "description exceeds 20000 characters")
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
			value, err := io.ReadAll(io.LimitReader(part, maxKnowledgeMetadataFieldSize+1))
			if err != nil {
				return nil, req, err
			}
			if len(value) > maxKnowledgeMetadataFieldSize {
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
	if !validKnowledgeDescription(req.Description) {
		return nil, req, errors.New("description exceeds 20000 characters")
	}
	if err := r.Context().Err(); err != nil {
		return nil, req, err
	}
	req.FileSize = fmt.Sprintf("%.2f MB", float64(pending.Size)/(1024*1024))
	keep = true
	return pending, req, nil
}

func validKnowledgeDescription(description string) bool {
	return utf8.ValidString(description) && utf8.RuneCountInString(description) <= maxKnowledgeDescriptionRunes
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

func (h *KnowledgeHandler) Search(c *gin.Context) {
	if h == nil || h.indexSvc == nil {
		Error(c, http.StatusServiceUnavailable, 503, "knowledge retrieval is not configured")
		return
	}
	items, err := h.indexSvc.Search(c.Request.Context(), c.Query("q"))
	if err != nil {
		if errors.Is(err, knowledgeindex.ErrInvalidQuery) {
			BadRequest(c, "q is required and must be at most 512 characters")
			return
		}
		InternalError(c, "failed to search knowledge")
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

	item, err := h.knowledgeSvc.GetItem(c.Request.Context(), id, GetUserID(c), IsCaptain(c))
	if err != nil {
		if errors.Is(err, shared.ErrForbidden) {
			Forbidden(c, "not allowed to access this item")
			return
		}
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

	_, err = h.knowledgeSvc.RejectItem(c.Request.Context(), id, reviewerID, reviewerName, req.Reason)
	if err != nil {
		InternalError(c, "failed to reject item")
		return
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

	item, err := h.knowledgeSvc.GetItem(c.Request.Context(), id, GetUserID(c), IsCaptain(c))
	if err != nil {
		if errors.Is(err, shared.ErrForbidden) {
			Forbidden(c, "not allowed to download this item")
			return
		}
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
	safeName := safeDownloadFilename(item.Name)
	encodedName := url.QueryEscape(safeName)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, safeName, encodedName))
	c.Header("Content-Type", "application/octet-stream")
	c.File(filePath)
}

// safeDownloadFilename keeps the fallback filename inside a quoted
// Content-Disposition value. Control characters, quotes, and path separators
// are replaced so a stored filename cannot inject headers or alter the path.
func safeDownloadFilename(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if r < 0x20 || r == 0x7f || r == '"' || r == '\\' || r == '/' {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	name = strings.TrimSpace(b.String())
	if name == "" || name == "." || name == ".." {
		return "download"
	}
	return name
}

func (h *KnowledgeHandler) CreateInsight(c *gin.Context) {
	itemID, ok := parseKnowledgeID(c, "id")
	if !ok {
		return
	}
	var req knowledge.CreateInsightRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid knowledge insight request")
		return
	}
	item, err := h.knowledgeSvc.CreateInsight(c.Request.Context(), GetUserID(c), itemID, req)
	if err != nil {
		handleKnowledgeInsightError(c, err)
		return
	}
	Created(c, item)
}

func (h *KnowledgeHandler) GetInsight(c *gin.Context) {
	itemID, ok := parseKnowledgeID(c, "id")
	if !ok {
		return
	}
	item, err := h.knowledgeSvc.GetInsight(c.Request.Context(), itemID, GetUserID(c), IsCaptain(c))
	if err != nil {
		handleKnowledgeInsightError(c, err)
		return
	}
	Success(c, item)
}

func (h *KnowledgeHandler) PublishInsight(c *gin.Context) {
	insightID, ok := parseKnowledgeID(c, "id")
	if !ok {
		return
	}
	item, err := h.knowledgeSvc.PublishInsight(c.Request.Context(), GetUserID(c), insightID)
	if err != nil {
		handleKnowledgeInsightError(c, err)
		return
	}
	Success(c, item)
}

func (h *KnowledgeHandler) InvalidateInsight(c *gin.Context) {
	insightID, ok := parseKnowledgeID(c, "id")
	if !ok {
		return
	}
	if err := h.knowledgeSvc.InvalidateInsight(c.Request.Context(), insightID); err != nil {
		handleKnowledgeInsightError(c, err)
		return
	}
	Success(c, nil)
}

func (h *KnowledgeHandler) ListDueInsights(c *gin.Context) {
	items, err := h.knowledgeSvc.ListDueInsights(c.Request.Context(), time.Now(), positiveQueryInt(c, "page", 1), positiveQueryInt(c, "page_size", 20))
	if err != nil {
		handleKnowledgeInsightError(c, err)
		return
	}
	Success(c, items)
}

func (h *KnowledgeHandler) Recommend(c *gin.Context) {
	query := c.Query("q")
	items, err := h.knowledgeSvc.Recommend(c.Request.Context(), GetUserID(c), query)
	if err != nil {
		handleKnowledgeInsightError(c, err)
		return
	}
	Success(c, items)
}

func (h *KnowledgeHandler) RecommendationFeedback(c *gin.Context) {
	itemID, ok := parseKnowledgeID(c, "id")
	if !ok {
		return
	}
	var req knowledge.RecommendationFeedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid recommendation feedback")
		return
	}
	if err := h.knowledgeSvc.SaveRecommendationFeedback(c.Request.Context(), GetUserID(c), itemID, req); err != nil {
		handleKnowledgeInsightError(c, err)
		return
	}
	Success(c, nil)
}

func parseKnowledgeID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		BadRequest(c, "invalid knowledge id")
		return 0, false
	}
	return id, true
}

func handleKnowledgeInsightError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, shared.ErrForbidden):
		Forbidden(c, "not allowed to access this knowledge insight")
	case errors.Is(err, shared.ErrConflict):
		Conflict(c, "knowledge source changed or insight state conflicts")
	case errors.Is(err, shared.ErrValidation), errors.Is(err, knowledgedomain.ErrInvalidInsight):
		BadRequest(c, "invalid knowledge insight request")
	default:
		NotFound(c, "knowledge insight not found")
	}
}
