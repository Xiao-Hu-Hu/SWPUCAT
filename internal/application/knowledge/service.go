package knowledge

import (
	"SWPUCAT/internal/application/knowledgeindex"
	"SWPUCAT/internal/domain/knowledge"
	"SWPUCAT/internal/domain/shared"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type CreateLinkRequest struct {
	Name        string `json:"name" validate:"required,max=256"`
	URL         string `json:"url" validate:"required,url"`
	Description string `json:"description" validate:"max=20000"`
	CategoryID  int64  `json:"category_id" validate:"required"`
}

type UploadFileRequest struct {
	FileName    string `json:"file_name" validate:"required,max=256"`
	Description string `json:"description" validate:"max=20000"`
	FileSize    string `json:"file_size"`
	FileKey     string `json:"file_key"`
	CategoryID  int64  `json:"category_id" validate:"required"`
}

type CreateCategoryRequest struct {
	Name string `json:"name" validate:"required,max=64"`
}

type CreateInsightRequest struct {
	SummaryMarkdown string     `json:"summary_markdown"`
	Tags            []string   `json:"tags,omitempty"`
	Model           string     `json:"model,omitempty"`
	PromptVersion   string     `json:"prompt_version,omitempty"`
	ReviewDueAt     *time.Time `json:"review_due_at,omitempty"`
}

type InsightDTO struct {
	ID                int64    `json:"id"`
	KnowledgeItemID   int64    `json:"knowledge_item_id"`
	SourceFingerprint string   `json:"source_fingerprint"`
	Status            string   `json:"status"`
	SummaryMarkdown   string   `json:"summary_markdown"`
	Tags              []string `json:"tags"`
	Model             string   `json:"model,omitempty"`
	PromptVersion     string   `json:"prompt_version,omitempty"`
	ReviewDueAt       string   `json:"review_due_at,omitempty"`
	CreatedBy         int64    `json:"created_by"`
	ReviewedBy        int64    `json:"reviewed_by,omitempty"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
}

type RecommendationDTO struct {
	ItemID          int64    `json:"item_id"`
	Type            string   `json:"type"`
	Name            string   `json:"name"`
	Description     string   `json:"description,omitempty"`
	URL             string   `json:"url,omitempty"`
	DownloadURL     string   `json:"download_url,omitempty"`
	CategoryName    string   `json:"category_name"`
	SourceKind      string   `json:"source_kind,omitempty"`
	Excerpt         string   `json:"excerpt,omitempty"`
	Score           float64  `json:"score"`
	MatchMode       string   `json:"match_mode"`
	SummaryMarkdown string   `json:"summary_markdown,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Reason          string   `json:"reason"`
}

type RecommendationFeedbackRequest struct {
	Helpful bool   `json:"helpful"`
	Note    string `json:"note,omitempty"`
}

type KnowledgeItemDTO struct {
	ID                   int64  `json:"id"`
	Type                 string `json:"type"`
	Name                 string `json:"name"`
	Description          string `json:"description,omitempty"`
	URL                  string `json:"url,omitempty"`
	FileKey              string `json:"file_key,omitempty"`
	FileSize             string `json:"file_size,omitempty"`
	CategoryID           int64  `json:"category_id"`
	CategoryName         string `json:"category_name"`
	UploaderID           int64  `json:"uploader_id"`
	UploaderName         string `json:"uploader_name"`
	Status               string `json:"status"`
	Approved             bool   `json:"approved"`
	Rejected             bool   `json:"rejected"`
	RejectReason         string `json:"reject_reason,omitempty"`
	ReviewerID           int64  `json:"reviewer_id,omitempty"`
	ReviewerName         string `json:"reviewer_name,omitempty"`
	SourceOrigin         string `json:"source_origin"`
	SourceKey            string `json:"source_key,omitempty"`
	Revision             int    `json:"revision"`
	QuarantinedAt        string `json:"quarantined_at,omitempty"`
	QuarantinedBy        int64  `json:"quarantined_by,omitempty"`
	QuarantineReasonCode string `json:"quarantine_reason_code,omitempty"`
	QuarantineReason     string `json:"quarantine_reason,omitempty"`
	CreatedAt            string `json:"created_at"`
}

type CategoryDTO struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	IsSystem bool   `json:"is_system"`
	Count    int64  `json:"count"`
}

type RecommendationSearcher interface {
	Search(ctx context.Context, query string) ([]knowledgeindex.SearchResult, error)
}

type KnowledgeService struct {
	repo                   knowledge.Repository
	publisher              shared.EventPublisher
	recommendationSearcher RecommendationSearcher
}

func NewKnowledgeService(repo knowledge.Repository, publisher shared.EventPublisher, searchers ...RecommendationSearcher) *KnowledgeService {
	service := &KnowledgeService{repo: repo, publisher: publisher}
	if len(searchers) > 0 {
		service.recommendationSearcher = searchers[0]
	}
	return service
}

func (s *KnowledgeService) CreateLink(ctx context.Context, uploaderID int64, uploaderName string, isCaptain bool, req CreateLinkRequest) (*KnowledgeItemDTO, error) {
	item, err := knowledge.NewLink(req.Name, req.URL, req.Description, req.CategoryID, uploaderID, uploaderName)
	if err != nil {
		return nil, err
	}
	if isCaptain {
		item.Approve()
	} else {
		item.SetStatus(knowledge.ItemStatusPending)
	}
	if err := s.repo.CreateItem(ctx, item); err != nil {
		return nil, err
	}
	return toItemDTO(item), nil
}

func (s *KnowledgeService) UploadFile(ctx context.Context, uploaderID int64, uploaderName string, isCaptain bool, req UploadFileRequest) (*KnowledgeItemDTO, error) {
	item, err := knowledge.NewFile(req.FileName, req.Description, req.FileSize, req.FileKey, req.CategoryID, uploaderID, uploaderName, isCaptain)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateItem(ctx, item); err != nil {
		return nil, err
	}
	s.publisher.Publish(knowledge.NewFileUploaded(item.ID, item.Name, item.UploaderID, !isCaptain))
	return toItemDTO(item), nil
}

func (s *KnowledgeService) DeleteItem(ctx context.Context, itemID, userID int64, isCaptain bool) error {
	item, err := s.repo.FindItemByID(ctx, itemID)
	if err != nil {
		return err
	}
	// Official catalog entries are versioned by migrations. Keep them outside
	// the generic item deletion path so a cited source cannot disappear because
	// of an accidental UI action.
	if item.IsOfficial() || item.EffectiveStatus() == knowledge.ItemStatusQuarantined {
		return shared.ErrForbidden
	}
	if !isCaptain && !item.IsOwnedBy(userID) {
		return shared.ErrForbidden
	}
	return s.repo.DeleteItem(ctx, itemID, userID)
}

func (s *KnowledgeService) QuarantineItem(ctx context.Context, itemID, actorID int64, actorName, reason string) error {
	reason = strings.TrimSpace(reason)
	if itemID <= 0 || actorID <= 0 || reason == "" || len([]rune(reason)) > 512 {
		return shared.ErrValidation
	}
	item, err := s.repo.FindItemByID(ctx, itemID)
	if err != nil {
		return err
	}
	if item.IsOfficial() {
		return shared.ErrForbidden
	}
	expected := item.EffectiveStatus()
	if err := item.Quarantine(actorID, "manual_review", reason); err != nil {
		return err
	}
	return s.repo.UpdateLifecycle(ctx, item, expected, "quarantine", actorID, strings.TrimSpace(actorName), reason)
}

func (s *KnowledgeService) RestoreItem(ctx context.Context, itemID, actorID int64, actorName, reason string) error {
	reason = strings.TrimSpace(reason)
	if itemID <= 0 || actorID <= 0 || reason == "" || len([]rune(reason)) > 512 {
		return shared.ErrValidation
	}
	item, err := s.repo.FindItemByID(ctx, itemID)
	if err != nil {
		return err
	}
	expected := item.EffectiveStatus()
	if err := item.RestoreFromQuarantine(actorID, reason); err != nil {
		return err
	}
	return s.repo.UpdateLifecycle(ctx, item, expected, "restore", actorID, strings.TrimSpace(actorName), reason)
}

func (s *KnowledgeService) ListItems(ctx context.Context, categoryID *int64, search string) ([]KnowledgeItemDTO, error) {
	approved := knowledge.ItemStatusApproved
	items, err := s.repo.FindItems(ctx, knowledge.ItemFilter{
		CategoryID: categoryID,
		Search:     search,
		Status:     &approved,
	})
	if err != nil {
		return nil, err
	}
	result := make([]KnowledgeItemDTO, 0, len(items))
	for _, item := range items {
		result = append(result, *toItemDTO(item))
	}
	return result, nil
}

func (s *KnowledgeService) ListPendingItems(ctx context.Context) ([]KnowledgeItemDTO, error) {
	pending := knowledge.ItemStatusPending
	items, err := s.repo.FindItems(ctx, knowledge.ItemFilter{
		Status: &pending,
	})
	if err != nil {
		return nil, err
	}
	result := make([]KnowledgeItemDTO, 0, len(items))
	for _, item := range items {
		result = append(result, *toItemDTO(item))
	}
	return result, nil
}

func (s *KnowledgeService) ListQuarantinedItems(ctx context.Context) ([]KnowledgeItemDTO, error) {
	quarantined := knowledge.ItemStatusQuarantined
	items, err := s.repo.FindItems(ctx, knowledge.ItemFilter{Status: &quarantined})
	if err != nil {
		return nil, err
	}
	result := make([]KnowledgeItemDTO, 0, len(items))
	for _, item := range items {
		result = append(result, *toItemDTO(item))
	}
	return result, nil
}

func (s *KnowledgeService) ListUserItems(ctx context.Context, userID int64) ([]KnowledgeItemDTO, error) {
	items, err := s.repo.FindItemsByUploader(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]KnowledgeItemDTO, 0, len(items))
	for _, item := range items {
		if !item.IsVisibleTo(userID, false) {
			continue
		}
		result = append(result, *toItemDTO(item))
	}
	return result, nil
}

func (s *KnowledgeService) CreateInsight(ctx context.Context, actorID, itemID int64, req CreateInsightRequest) (*InsightDTO, error) {
	item, err := s.repo.FindItemByID(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if item.EffectiveStatus() != knowledge.ItemStatusApproved {
		return nil, shared.ErrConflict
	}
	tagsJSON, err := marshalTags(req.Tags)
	if err != nil {
		return nil, err
	}
	insight, err := knowledge.NewInsight(item.ID, actorID, knowledgeSourceFingerprint(item), req.SummaryMarkdown, tagsJSON, req.Model, req.PromptVersion, req.ReviewDueAt)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateInsight(ctx, insight); err != nil {
		return nil, err
	}
	return toInsightDTO(insight), nil
}

func (s *KnowledgeService) GetInsight(ctx context.Context, itemID, viewerID int64, canReview bool) (*InsightDTO, error) {
	item, err := s.repo.FindItemByID(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if !item.IsVisibleTo(viewerID, canReview) {
		return nil, shared.ErrForbidden
	}
	insight, err := s.repo.FindInsightByItemID(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if insight.Status != knowledge.InsightInvalidated && insight.SourceFingerprint != knowledgeSourceFingerprint(item) {
		expected := insight.Status
		if invalidateErr := insight.Invalidate(); invalidateErr == nil {
			if updateErr := s.repo.UpdateInsight(ctx, insight, expected); updateErr != nil {
				return nil, updateErr
			}
		}
	}
	return toInsightDTO(insight), nil
}

func (s *KnowledgeService) PublishInsight(ctx context.Context, actorID, insightID int64) (*InsightDTO, error) {
	insight, err := s.findInsightByID(ctx, insightID)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.FindItemByID(ctx, insight.KnowledgeItemID)
	if err != nil {
		return nil, err
	}
	if insight.SourceFingerprint != knowledgeSourceFingerprint(item) {
		expected := insight.Status
		if invalidateErr := insight.Invalidate(); invalidateErr == nil {
			_ = s.repo.UpdateInsight(ctx, insight, expected)
		}
		return nil, shared.ErrConflict
	}
	expected := insight.Status
	if err := insight.Publish(actorID); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateInsight(ctx, insight, expected); err != nil {
		return nil, err
	}
	return toInsightDTO(insight), nil
}

func (s *KnowledgeService) InvalidateInsight(ctx context.Context, insightID int64) error {
	insight, err := s.findInsightByID(ctx, insightID)
	if err != nil {
		return err
	}
	expected := insight.Status
	if err := insight.Invalidate(); err != nil {
		return err
	}
	return s.repo.UpdateInsight(ctx, insight, expected)
}

func (s *KnowledgeService) ListDueInsights(ctx context.Context, now time.Time, page, pageSize int) ([]InsightDTO, error) {
	items, err := s.repo.ListDueInsights(ctx, now, page, pageSize)
	if err != nil {
		return nil, err
	}
	result := make([]InsightDTO, 0, len(items))
	for _, item := range items {
		result = append(result, *toInsightDTO(item))
	}
	return result, nil
}

func (s *KnowledgeService) Recommend(ctx context.Context, userID int64, query string) ([]RecommendationDTO, error) {
	query = strings.TrimSpace(query)
	if userID <= 0 || query == "" || len([]rune(query)) > 128 {
		return nil, shared.ErrValidation
	}
	if s.recommendationSearcher != nil {
		items, err := s.recommendationSearcher.Search(ctx, query)
		if err == nil {
			return s.recommendFromSearch(ctx, items)
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return s.recommendFromMetadata(ctx, query)
}

func (s *KnowledgeService) recommendFromSearch(ctx context.Context, items []knowledgeindex.SearchResult) ([]RecommendationDTO, error) {
	result := make([]RecommendationDTO, 0, len(items))
	seen := make(map[int64]struct{}, len(items))
	for _, match := range items {
		if match.ItemID <= 0 {
			continue
		}
		if _, exists := seen[match.ItemID]; exists {
			continue
		}
		seen[match.ItemID] = struct{}{}
		item, err := s.repo.FindItemByID(ctx, match.ItemID)
		if errors.Is(err, knowledge.ErrItemNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if item.EffectiveStatus() != knowledge.ItemStatusApproved {
			continue
		}
		mode := normalizeRecommendationMatchMode(match.MatchMode)
		recommendation := RecommendationDTO{
			ItemID: item.ID, Type: string(item.Type), Name: item.Name,
			Description: item.Description, URL: item.URL, CategoryName: item.CategoryName,
			SourceKind: match.SourceKind, Excerpt: match.Excerpt, Score: match.Score,
			MatchMode: mode, Reason: recommendationReason(mode),
		}
		if item.Type == knowledge.ItemTypeFile {
			recommendation.DownloadURL = "/api/knowledge/download/" + strconv.FormatInt(item.ID, 10)
		}
		s.attachPublishedInsight(ctx, item, &recommendation)
		result = append(result, recommendation)
	}
	return result, nil
}

func (s *KnowledgeService) recommendFromMetadata(ctx context.Context, query string) ([]RecommendationDTO, error) {
	approved := knowledge.ItemStatusApproved
	items, err := s.repo.FindItems(ctx, knowledge.ItemFilter{Search: query, Status: &approved, Page: 1, PageSize: 20})
	if err != nil {
		return nil, err
	}
	result := make([]RecommendationDTO, 0, len(items))
	for _, item := range items {
		if item.EffectiveStatus() != knowledge.ItemStatusApproved {
			continue
		}
		recommendation := RecommendationDTO{
			ItemID: item.ID, Type: string(item.Type), Name: item.Name,
			Description: item.Description, URL: item.URL, CategoryName: item.CategoryName,
			MatchMode: "metadata_keyword", Reason: recommendationReason("metadata_keyword"),
		}
		if item.Type == knowledge.ItemTypeFile {
			recommendation.DownloadURL = "/api/knowledge/download/" + strconv.FormatInt(item.ID, 10)
		}
		s.attachPublishedInsight(ctx, item, &recommendation)
		result = append(result, recommendation)
	}
	return result, nil
}

func (s *KnowledgeService) attachPublishedInsight(ctx context.Context, item *knowledge.KnowledgeItem, recommendation *RecommendationDTO) {
	insight, err := s.repo.FindInsightByItemID(ctx, item.ID)
	if err != nil || insight == nil || insight.Status != knowledge.InsightPublished || insight.SourceFingerprint != knowledgeSourceFingerprint(item) {
		return
	}
	recommendation.SummaryMarkdown = insight.SummaryMarkdown
	recommendation.Tags, _ = parseTags(insight.TagsJSON)
}

func normalizeRecommendationMatchMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "keyword", "vector", "hybrid":
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return "unknown"
	}
}

func recommendationReason(mode string) string {
	switch mode {
	case "hybrid":
		return "关键词与语义综合匹配"
	case "vector":
		return "正文语义与需求相关"
	case "keyword":
		return "名称、说明或正文关键词匹配"
	case "metadata_keyword":
		return "名称、说明或分类与需求匹配（检索服务降级）"
	default:
		return "知识库内容与需求相关"
	}
}

func (s *KnowledgeService) SaveRecommendationFeedback(ctx context.Context, userID, itemID int64, req RecommendationFeedbackRequest) error {
	if userID <= 0 || itemID <= 0 || len([]rune(req.Note)) > 512 {
		return shared.ErrValidation
	}
	return s.repo.SaveRecommendationFeedback(ctx, &knowledge.RecommendationFeedback{UserID: userID, KnowledgeItemID: itemID, Helpful: req.Helpful, Note: strings.TrimSpace(req.Note), CreatedAt: time.Now()})
}

func (s *KnowledgeService) findInsightByID(ctx context.Context, insightID int64) (*knowledge.Insight, error) {
	if insightID <= 0 {
		return nil, shared.ErrValidation
	}
	return s.repo.FindInsightByID(ctx, insightID)
}

func knowledgeSourceFingerprint(item *knowledge.KnowledgeItem) string {
	payload := strings.Join([]string{item.Name, item.Description, item.URL, item.FileKey, fmt.Sprintf("%d", item.CategoryID)}, "\x00")
	digest := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(digest[:])
}

func marshalTags(tags []string) (string, error) {
	seen := make(map[string]struct{}, len(tags))
	clean := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || len([]rune(tag)) > 32 {
			continue
		}
		if _, ok := seen[tag]; !ok {
			seen[tag] = struct{}{}
			clean = append(clean, tag)
		}
	}
	sort.Strings(clean)
	if len(clean) > 20 {
		clean = clean[:20]
	}
	data, err := json.Marshal(clean)
	if err != nil {
		return "", shared.ErrValidation
	}
	return string(data), nil
}

func parseTags(value string) ([]string, error) {
	var tags []string
	if err := json.Unmarshal([]byte(value), &tags); err != nil {
		return nil, err
	}
	return tags, nil
}

func toInsightDTO(item *knowledge.Insight) *InsightDTO {
	tags, _ := parseTags(item.TagsJSON)
	dto := &InsightDTO{ID: item.ID, KnowledgeItemID: item.KnowledgeItemID, SourceFingerprint: item.SourceFingerprint, Status: string(item.Status), SummaryMarkdown: item.SummaryMarkdown, Tags: tags, Model: item.Model, PromptVersion: item.PromptVersion, CreatedBy: item.CreatedBy, ReviewedBy: item.ReviewedBy, CreatedAt: item.CreatedAt.Format(time.RFC3339), UpdatedAt: item.UpdatedAt.Format(time.RFC3339)}
	if item.ReviewDueAt != nil {
		dto.ReviewDueAt = item.ReviewDueAt.Format(time.RFC3339)
	}
	return dto
}

func (s *KnowledgeService) ApproveItem(ctx context.Context, itemID, reviewerID int64, reviewerName string) error {
	item, err := s.repo.FindItemByID(ctx, itemID)
	if err != nil {
		return err
	}
	if item.EffectiveStatus() != knowledge.ItemStatusPending {
		return knowledge.ErrInvalidItemState
	}
	item.Approve()
	item.ReviewerID = reviewerID
	item.ReviewerName = reviewerName
	return s.repo.UpdateItem(ctx, item, knowledge.ItemStatusPending)
}

func (s *KnowledgeService) RejectItem(ctx context.Context, itemID, reviewerID int64, reviewerName, reason string) (string, error) {
	item, err := s.repo.FindItemByID(ctx, itemID)
	if err != nil {
		return "", err
	}
	if item.EffectiveStatus() != knowledge.ItemStatusPending {
		return "", knowledge.ErrInvalidItemState
	}
	item.Reject(reason)
	item.ReviewerID = reviewerID
	item.ReviewerName = reviewerName
	if err := s.repo.UpdateItem(ctx, item, knowledge.ItemStatusPending); err != nil {
		return "", err
	}
	// Rejected attachments remain available for audit and possible re-review.
	return "", nil
}

func (s *KnowledgeService) CreateCategory(ctx context.Context, name string) (*CategoryDTO, error) {
	cat := &knowledge.Category{Name: name, CreatedAt: time.Now()}
	if err := s.repo.CreateCategory(ctx, cat); err != nil {
		return nil, err
	}
	return &CategoryDTO{ID: cat.ID, Name: cat.Name, IsSystem: false}, nil
}

func (s *KnowledgeService) DeleteCategory(ctx context.Context, catID int64) error {
	cat, err := s.repo.FindCategoryByID(ctx, catID)
	if err != nil {
		return err
	}
	if cat.IsSystem {
		return knowledge.ErrCategoryInUse
	}
	count, err := s.repo.CountItemsByCategory(ctx, catID)
	if err != nil {
		return err
	}
	if count > 0 {
		return knowledge.ErrCategoryInUse
	}
	return s.repo.DeleteCategory(ctx, catID)
}

func (s *KnowledgeService) ListCategories(ctx context.Context) ([]CategoryDTO, error) {
	cats, err := s.repo.FindAllCategories(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]CategoryDTO, 0, len(cats))
	for _, c := range cats {
		count, _ := s.repo.CountItemsByCategory(ctx, c.ID)
		result = append(result, CategoryDTO{
			ID:       c.ID,
			Name:     c.Name,
			IsSystem: c.IsSystem,
			Count:    count,
		})
	}
	return result, nil
}

func (s *KnowledgeService) GetItem(ctx context.Context, id, viewerID int64, canReview bool) (*KnowledgeItemDTO, error) {
	item, err := s.repo.FindItemByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !item.IsVisibleTo(viewerID, canReview) {
		return nil, shared.ErrForbidden
	}
	return toItemDTO(item), nil
}

func toItemDTO(item *knowledge.KnowledgeItem) *KnowledgeItemDTO {
	dto := &KnowledgeItemDTO{
		ID:                   item.ID,
		Type:                 string(item.Type),
		Name:                 item.Name,
		Description:          item.Description,
		URL:                  item.URL,
		FileKey:              item.FileKey,
		FileSize:             item.FileSize,
		CategoryID:           item.CategoryID,
		CategoryName:         item.CategoryName,
		UploaderID:           item.UploaderID,
		UploaderName:         item.UploaderName,
		Status:               string(item.EffectiveStatus()),
		Approved:             item.Approved,
		Rejected:             item.Rejected,
		RejectReason:         item.RejectReason,
		ReviewerID:           item.ReviewerID,
		ReviewerName:         item.ReviewerName,
		SourceOrigin:         item.SourceOrigin,
		SourceKey:            item.SourceKey,
		Revision:             item.Revision,
		QuarantinedBy:        item.QuarantinedBy,
		QuarantineReasonCode: item.QuarantineReasonCode,
		QuarantineReason:     item.QuarantineReason,
		CreatedAt:            item.CreatedAt.Format("2006-01-02"),
	}
	if item.QuarantinedAt != nil {
		dto.QuarantinedAt = item.QuarantinedAt.Format(time.RFC3339)
	}
	return dto
}
