package knowledge

import (
	"SWPUCAT/internal/application/knowledgeindex"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domain "SWPUCAT/internal/domain/knowledge"
	"SWPUCAT/internal/domain/shared"
	"SWPUCAT/internal/infrastructure/event"
)

type stateRepository struct {
	domain.Repository
	item           *domain.KnowledgeItem
	expectedStatus domain.ItemStatus
	updateCalls    int
	lifecycleEvent string
	lifecycleCalls int
}

type insightRepository struct {
	domain.Repository
	item     *domain.KnowledgeItem
	items    []*domain.KnowledgeItem
	insight  *domain.Insight
	feedback *domain.RecommendationFeedback
}

type recommendationSearcher struct {
	items []knowledgeindex.SearchResult
	err   error
	calls int
}

func (s *recommendationSearcher) Search(context.Context, string) ([]knowledgeindex.SearchResult, error) {
	s.calls++
	return s.items, s.err
}

func (r *insightRepository) FindItemByID(context.Context, int64) (*domain.KnowledgeItem, error) {
	return r.item, nil
}
func (r *insightRepository) FindItems(context.Context, domain.ItemFilter) ([]*domain.KnowledgeItem, error) {
	items := make([]*domain.KnowledgeItem, 0, len(r.items))
	for _, item := range r.items {
		if item.EffectiveStatus() == domain.ItemStatusApproved {
			items = append(items, item)
		}
	}
	return items, nil
}
func (r *insightRepository) FindItemsByUploader(context.Context, int64) ([]*domain.KnowledgeItem, error) {
	return r.items, nil
}
func (r *insightRepository) CreateInsight(_ context.Context, item *domain.Insight) error {
	item.ID = 11
	r.insight = item
	return nil
}
func (r *insightRepository) FindInsightByItemID(context.Context, int64) (*domain.Insight, error) {
	if r.insight == nil {
		return nil, errors.New("insight not found")
	}
	return r.insight, nil
}
func (r *insightRepository) FindInsightByID(context.Context, int64) (*domain.Insight, error) {
	return r.insight, nil
}
func (r *insightRepository) UpdateInsight(_ context.Context, item *domain.Insight, _ domain.InsightStatus) error {
	r.insight = item
	return nil
}
func (r *insightRepository) ListDueInsights(context.Context, time.Time, int, int) ([]*domain.Insight, error) {
	if r.insight == nil {
		return nil, nil
	}
	return []*domain.Insight{r.insight}, nil
}
func (r *insightRepository) SaveRecommendationFeedback(_ context.Context, feedback *domain.RecommendationFeedback) error {
	r.feedback = feedback
	return nil
}

func (r *stateRepository) CreateItem(_ context.Context, item *domain.KnowledgeItem) error {
	item.ID = 1
	r.item = item
	return nil
}

func (r *stateRepository) FindItemByID(context.Context, int64) (*domain.KnowledgeItem, error) {
	return r.item, nil
}

func (r *stateRepository) FindItems(_ context.Context, filter domain.ItemFilter) ([]*domain.KnowledgeItem, error) {
	if r.item == nil || filter.Status == nil || r.item.EffectiveStatus() != *filter.Status {
		return []*domain.KnowledgeItem{}, nil
	}
	return []*domain.KnowledgeItem{r.item}, nil
}

func (r *stateRepository) UpdateItem(_ context.Context, item *domain.KnowledgeItem, expected domain.ItemStatus) error {
	r.item = item
	r.expectedStatus = expected
	r.updateCalls++
	return nil
}

func (r *stateRepository) UpdateLifecycle(_ context.Context, item *domain.KnowledgeItem, expected domain.ItemStatus, eventType string, _ int64, _ string, _ string) error {
	r.item = item
	r.expectedStatus = expected
	r.lifecycleEvent = eventType
	r.lifecycleCalls++
	return nil
}

func TestCreateLinkUsesUnifiedStatus(t *testing.T) {
	for _, tc := range []struct {
		name     string
		captain  bool
		want     domain.ItemStatus
		approved bool
	}{
		{name: "member submission", want: domain.ItemStatusPending},
		{name: "captain submission", captain: true, want: domain.ItemStatusApproved, approved: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &stateRepository{}
			svc := NewKnowledgeService(repo, event.NewNoOpPublisher())
			dto, err := svc.CreateLink(context.Background(), 7, "member", tc.captain, CreateLinkRequest{
				Name: "guide", URL: "https://example.com", CategoryID: 2,
			})
			if err != nil {
				t.Fatal(err)
			}
			if repo.item.EffectiveStatus() != tc.want || dto.Status != string(tc.want) || dto.Approved != tc.approved {
				t.Fatalf("inconsistent status: item=%+v dto=%+v", repo.item, dto)
			}
		})
	}
}

func TestReviewOnlyTransitionsPendingItems(t *testing.T) {
	repo := &stateRepository{item: &domain.KnowledgeItem{ID: 1, Status: domain.ItemStatusPending}}
	svc := NewKnowledgeService(repo, event.NewNoOpPublisher())
	if err := svc.ApproveItem(context.Background(), 1, 9, "captain"); err != nil {
		t.Fatal(err)
	}
	if repo.expectedStatus != domain.ItemStatusPending || repo.item.EffectiveStatus() != domain.ItemStatusApproved {
		t.Fatalf("unexpected review transition: %+v", repo)
	}

	repo.item = &domain.KnowledgeItem{ID: 2, Status: domain.ItemStatusRejected, Rejected: true}
	if err := svc.ApproveItem(context.Background(), 2, 9, "captain"); !errors.Is(err, domain.ErrInvalidItemState) {
		t.Fatalf("approved a rejected item: %v", err)
	}
	if repo.updateCalls != 1 {
		t.Fatalf("unexpected update count: %d", repo.updateCalls)
	}
}

func TestRejectKeepsAttachmentForAudit(t *testing.T) {
	repo := &stateRepository{item: &domain.KnowledgeItem{
		ID: 1, Type: domain.ItemTypeFile, FileKey: "2026-09-10/report.md", Status: domain.ItemStatusPending,
	}}
	svc := NewKnowledgeService(repo, event.NewNoOpPublisher())
	fileKey, err := svc.RejectItem(context.Background(), 1, 9, "captain", "needs revision")
	if err != nil {
		t.Fatal(err)
	}
	if fileKey != "" || repo.item.FileKey == "" || repo.item.EffectiveStatus() != domain.ItemStatusRejected {
		t.Fatalf("rejection lost audit attachment: key=%q item=%+v", fileKey, repo.item)
	}
}

func TestOfficialCatalogItemCannotUseGenericDelete(t *testing.T) {
	repo := &stateRepository{item: &domain.KnowledgeItem{
		ID:           17,
		Type:         domain.ItemTypeArticle,
		Status:       domain.ItemStatusApproved,
		SourceOrigin: domain.SourceOriginOfficialCatalog,
		SourceKey:    "official.faq",
		UploaderID:   9,
	}}
	svc := NewKnowledgeService(repo, event.NewNoOpPublisher())
	if err := svc.DeleteItem(context.Background(), 17, 9, true); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("official catalog item deletion error = %v, want forbidden", err)
	}
}

func TestQuarantinedItemCannotUseGenericDelete(t *testing.T) {
	repo := &stateRepository{item: &domain.KnowledgeItem{
		ID: 9, Type: domain.ItemTypeFile, Status: domain.ItemStatusQuarantined,
		SourceOrigin: domain.SourceOriginUser, UploaderID: 7,
	}}
	svc := NewKnowledgeService(repo, event.NewNoOpPublisher())
	if err := svc.DeleteItem(context.Background(), 9, 7, true); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("quarantined item deletion error = %v, want forbidden", err)
	}
}

func TestKnowledgeQuarantineAndRestoreLifecycle(t *testing.T) {
	repo := &stateRepository{item: &domain.KnowledgeItem{
		ID: 9, Type: domain.ItemTypeFile, Status: domain.ItemStatusApproved, Approved: true,
		SourceOrigin: domain.SourceOriginUser, Revision: 1,
	}}
	svc := NewKnowledgeService(repo, event.NewNoOpPublisher())
	if err := svc.QuarantineItem(context.Background(), 9, 3, "captain", "许可证状态不明"); err != nil {
		t.Fatal(err)
	}
	if repo.lifecycleCalls != 1 || repo.lifecycleEvent != "quarantine" || repo.expectedStatus != domain.ItemStatusApproved || repo.item.EffectiveStatus() != domain.ItemStatusQuarantined || repo.item.QuarantineReasonCode != "manual_review" {
		t.Fatalf("unexpected quarantine transition: %+v", repo)
	}
	if err := svc.RestoreItem(context.Background(), 9, 3, "captain", "许可证已核验"); err != nil {
		t.Fatal(err)
	}
	if repo.lifecycleCalls != 2 || repo.lifecycleEvent != "restore" || repo.expectedStatus != domain.ItemStatusQuarantined || repo.item.EffectiveStatus() != domain.ItemStatusApproved || repo.item.QuarantinedAt != nil {
		t.Fatalf("unexpected restore transition: %+v", repo)
	}
}

func TestListQuarantinedItemsIncludesAuditReason(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	repo := &stateRepository{item: &domain.KnowledgeItem{
		ID: 9, Type: domain.ItemTypeFile, Name: "Typora.zip", Status: domain.ItemStatusQuarantined,
		SourceOrigin: domain.SourceOriginUser, Revision: 1, QuarantinedAt: &now,
		QuarantinedBy: 3, QuarantineReasonCode: "license_risk", QuarantineReason: "许可证状态不明",
	}}
	items, err := NewKnowledgeService(repo, event.NewNoOpPublisher()).ListQuarantinedItems(context.Background())
	if err != nil || len(items) != 1 {
		t.Fatalf("quarantined items = %+v, err = %v", items, err)
	}
	if items[0].QuarantineReasonCode != "license_risk" || items[0].QuarantinedAt == "" || items[0].Approved {
		t.Fatalf("quarantine audit metadata missing: %+v", items[0])
	}
}

func TestListUserItemsExcludesQuarantinedItem(t *testing.T) {
	repo := &insightRepository{items: []*domain.KnowledgeItem{
		{ID: 1, UploaderID: 7, Name: "待审资料", Status: domain.ItemStatusPending},
		{ID: 2, UploaderID: 7, Name: "已审核资料", Status: domain.ItemStatusApproved, Approved: true},
		{ID: 3, UploaderID: 7, Name: "隔离资料", Status: domain.ItemStatusQuarantined},
	}}
	items, err := NewKnowledgeService(repo, event.NewNoOpPublisher()).ListUserItems(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != 1 || items[1].ID != 2 {
		t.Fatalf("user-visible items = %+v, want pending and approved only", items)
	}
}

func TestInsightIsIndependentAndInvalidatedWhenSourceChanges(t *testing.T) {
	repo := &insightRepository{item: &domain.KnowledgeItem{ID: 7, Name: "安装指南", Description: "旧说明", Status: domain.ItemStatusApproved}}
	svc := NewKnowledgeService(repo, event.NewNoOpPublisher())
	created, err := svc.CreateInsight(context.Background(), 9, 7, CreateInsightRequest{SummaryMarkdown: "适用于新成员", Tags: []string{"Go", "入门"}})
	if err != nil || created.Status != string(domain.InsightProposed) {
		t.Fatalf("insight proposal failed: dto=%+v err=%v", created, err)
	}
	if _, err := svc.PublishInsight(context.Background(), 9, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetInsight(context.Background(), 7, 9, false); err != nil || repo.insight.Status != domain.InsightPublished {
		t.Fatalf("published insight could not be read: %+v err=%v", repo.insight, err)
	}
	repo.item.Description = "新说明"
	item, err := svc.GetInsight(context.Background(), 7, 9, false)
	if err != nil || item.Status != string(domain.InsightInvalidated) || repo.insight.Status != domain.InsightInvalidated {
		t.Fatalf("source change did not invalidate derived insight: dto=%+v err=%v", item, err)
	}
}

func TestRecommendOnlyReturnsApprovedVisibleItemsAndStoresFeedback(t *testing.T) {
	repo := &insightRepository{items: []*domain.KnowledgeItem{{ID: 7, Name: "Go 入门", Description: "Go 教程", Status: domain.ItemStatusApproved}, {ID: 8, Name: "待审资料", Description: "Go", Status: domain.ItemStatusPending}}, item: &domain.KnowledgeItem{ID: 7, Name: "Go 入门", Description: "Go 教程", Status: domain.ItemStatusApproved}}
	insight, _ := domain.NewInsight(7, 9, knowledgeSourceFingerprint(repo.item), "从基础开始", `["Go"]`, "model", "prompt", nil)
	insight.Status = domain.InsightPublished
	repo.insight = insight
	svc := NewKnowledgeService(repo, event.NewNoOpPublisher())
	items, err := svc.Recommend(context.Background(), 3, "Go")
	if err != nil || len(items) != 1 {
		t.Fatalf("recommendation query failed: items=%+v err=%v", items, err)
	}
	if items[0].SummaryMarkdown != "从基础开始" || items[0].Tags[0] != "Go" {
		t.Fatalf("published insight was not attached: %+v", items[0])
	}
	if items[0].MatchMode != "metadata_keyword" {
		t.Fatalf("metadata fallback was not identified: %+v", items[0])
	}
	if err := svc.SaveRecommendationFeedback(context.Background(), 3, 7, RecommendationFeedbackRequest{Helpful: true, Note: "有帮助"}); err != nil || repo.feedback == nil || !repo.feedback.Helpful {
		t.Fatalf("recommendation feedback was not stored: %+v err=%v", repo.feedback, err)
	}
}

func TestRecommendUsesSemanticSearchAndAttachesPublishedInsight(t *testing.T) {
	item := &domain.KnowledgeItem{
		ID: 17, Type: domain.ItemTypeFile, Name: "服务稳定性手册.pdf",
		Description: "后端工程手册", CategoryName: "工程实践",
		Status: domain.ItemStatusApproved, Approved: true,
	}
	repo := &insightRepository{item: item}
	insight, _ := domain.NewInsight(item.ID, 9, knowledgeSourceFingerprint(item), "适合处理请求超时问题", `["Go","稳定性"]`, "model", "prompt", nil)
	insight.Status = domain.InsightPublished
	repo.insight = insight
	searcher := &recommendationSearcher{items: []knowledgeindex.SearchResult{{
		ItemID: item.ID, Name: item.Name, SourceKind: "content",
		Excerpt: "通过 context 取消阻止请求永久阻塞。", Score: 0.82, MatchMode: "vector",
	}}}

	items, err := NewKnowledgeService(repo, event.NewNoOpPublisher(), searcher).
		Recommend(context.Background(), 3, "怎样避免请求一直卡住")
	if err != nil || len(items) != 1 {
		t.Fatalf("semantic recommendation failed: items=%+v err=%v", items, err)
	}
	got := items[0]
	if got.ItemID != item.ID || got.MatchMode != "vector" || got.SourceKind != "content" || got.Score != 0.82 {
		t.Fatalf("semantic metadata was not preserved: %+v", got)
	}
	if got.Excerpt == "" || got.DownloadURL != "/api/knowledge/download/17" || got.SummaryMarkdown == "" || len(got.Tags) != 2 {
		t.Fatalf("source projection or insight is incomplete: %+v", got)
	}
	if got.Reason != "正文语义与需求相关" || searcher.calls != 1 {
		t.Fatalf("unexpected reason/search count: %+v calls=%d", got, searcher.calls)
	}
}

func TestRecommendRechecksCurrentVisibilityForIndexMatches(t *testing.T) {
	for _, status := range []domain.ItemStatus{
		domain.ItemStatusPending,
		domain.ItemStatusQuarantined,
		domain.ItemStatusRejected,
	} {
		t.Run(string(status), func(t *testing.T) {
			item := &domain.KnowledgeItem{ID: 8, Name: "不可见资料", Status: status}
			searcher := &recommendationSearcher{items: []knowledgeindex.SearchResult{{
				ItemID: item.ID, Name: item.Name, Excerpt: "旧索引片段", MatchMode: "hybrid",
			}}}
			items, err := NewKnowledgeService(&insightRepository{item: item}, event.NewNoOpPublisher(), searcher).
				Recommend(context.Background(), 3, "资料")
			if err != nil || len(items) != 0 {
				t.Fatalf("status %s leaked from index: items=%+v err=%v", status, items, err)
			}
		})
	}
}

func TestRecommendReturnsEmptyWhenHybridSearchHasNoMatches(t *testing.T) {
	searcher := &recommendationSearcher{items: []knowledgeindex.SearchResult{}}
	items, err := NewKnowledgeService(&insightRepository{
		items: []*domain.KnowledgeItem{{ID: 1, Name: "元数据本可匹配", Status: domain.ItemStatusApproved}},
	}, event.NewNoOpPublisher(), searcher).Recommend(context.Background(), 3, "匹配")
	if err != nil || items == nil || len(items) != 0 {
		t.Fatalf("empty hybrid result must stay empty: items=%#v err=%v", items, err)
	}
}

func TestRecommendFallsBackWhenRetrievalFails(t *testing.T) {
	item := &domain.KnowledgeItem{ID: 7, Type: domain.ItemTypeLink, Name: "Go 入门", Description: "Go 教程", Status: domain.ItemStatusApproved}
	searcher := &recommendationSearcher{err: errors.New("retrieval unavailable")}
	items, err := NewKnowledgeService(&insightRepository{item: item, items: []*domain.KnowledgeItem{item}}, event.NewNoOpPublisher(), searcher).
		Recommend(context.Background(), 3, "Go")
	if err != nil || len(items) != 1 {
		t.Fatalf("metadata fallback failed: items=%+v err=%v", items, err)
	}
	if items[0].MatchMode != "metadata_keyword" || !strings.Contains(items[0].Reason, "降级") {
		t.Fatalf("fallback was not disclosed: %+v", items[0])
	}
}
