package knowledge

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "SWPUCAT/internal/domain/knowledge"
	"SWPUCAT/internal/infrastructure/event"
)

type stateRepository struct {
	domain.Repository
	item           *domain.KnowledgeItem
	expectedStatus domain.ItemStatus
	updateCalls    int
}

type insightRepository struct {
	domain.Repository
	item     *domain.KnowledgeItem
	items    []*domain.KnowledgeItem
	insight  *domain.Insight
	feedback *domain.RecommendationFeedback
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

func (r *stateRepository) UpdateItem(_ context.Context, item *domain.KnowledgeItem, expected domain.ItemStatus) error {
	r.item = item
	r.expectedStatus = expected
	r.updateCalls++
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
	if err := svc.SaveRecommendationFeedback(context.Background(), 3, 7, RecommendationFeedbackRequest{Helpful: true, Note: "有帮助"}); err != nil || repo.feedback == nil || !repo.feedback.Helpful {
		t.Fatalf("recommendation feedback was not stored: %+v err=%v", repo.feedback, err)
	}
}
