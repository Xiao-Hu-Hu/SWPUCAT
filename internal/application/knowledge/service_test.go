package knowledge

import (
	"context"
	"errors"
	"testing"

	domain "SWPUCAT/internal/domain/knowledge"
	"SWPUCAT/internal/infrastructure/event"
)

type stateRepository struct {
	domain.Repository
	item           *domain.KnowledgeItem
	expectedStatus domain.ItemStatus
	updateCalls    int
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
