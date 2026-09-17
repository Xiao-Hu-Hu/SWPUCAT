package knowledge

import (
	"errors"
	"testing"
)

func TestKnowledgeItemVisibility(t *testing.T) {
	tests := []struct {
		name      string
		item      KnowledgeItem
		viewerID  int64
		canReview bool
		visible   bool
	}{
		{
			name:     "approved public item",
			item:     KnowledgeItem{Status: ItemStatusApproved, Approved: true, UploaderID: 1},
			viewerID: 2,
			visible:  true,
		},
		{
			name:     "pending owner",
			item:     KnowledgeItem{UploaderID: 1},
			viewerID: 1,
			visible:  true,
		},
		{
			name:     "pending other member",
			item:     KnowledgeItem{UploaderID: 1},
			viewerID: 2,
			visible:  false,
		},
		{
			name:      "pending reviewer",
			item:      KnowledgeItem{Status: ItemStatusPending, UploaderID: 1},
			viewerID:  2,
			canReview: true,
			visible:   true,
		},
		{
			name:     "conflicting approval state is private",
			item:     KnowledgeItem{Approved: true, Rejected: true, UploaderID: 1},
			viewerID: 2,
			visible:  false,
		},
		{
			name:     "deleted item is private even to owner",
			item:     KnowledgeItem{Status: ItemStatusDeleted, UploaderID: 1},
			viewerID: 1,
			visible:  false,
		},
		{
			name:     "quarantined item is private to owner",
			item:     KnowledgeItem{Status: ItemStatusQuarantined, UploaderID: 1},
			viewerID: 1,
			visible:  false,
		},
		{
			name:      "quarantined item remains visible to reviewer",
			item:      KnowledgeItem{Status: ItemStatusQuarantined, UploaderID: 1},
			viewerID:  2,
			canReview: true,
			visible:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.item.IsVisibleTo(tt.viewerID, tt.canReview); got != tt.visible {
				t.Fatalf("IsVisibleTo() = %v, want %v", got, tt.visible)
			}
		})
	}
}

func TestKnowledgeItemStatusSynchronizesLegacyFlags(t *testing.T) {
	item := KnowledgeItem{}
	item.Approve()
	if item.Status != ItemStatusApproved || !item.Approved || item.Rejected {
		t.Fatalf("approve produced inconsistent state: %+v", item)
	}
	item.Reject("outdated")
	if item.Status != ItemStatusRejected || item.Approved || !item.Rejected || item.RejectReason != "outdated" {
		t.Fatalf("reject produced inconsistent state: %+v", item)
	}
	item.MarkDeleted(9)
	if item.Status != ItemStatusDeleted || item.Approved || item.Rejected || item.DeletedBy != 9 || item.DeletedAt == nil {
		t.Fatalf("delete produced inconsistent state: %+v", item)
	}
}

func TestOfficialArticleIdentity(t *testing.T) {
	if !ItemTypeArticle.IsValid() {
		t.Fatal("article must be a valid knowledge item type")
	}
	item := KnowledgeItem{
		Type:         ItemTypeArticle,
		SourceOrigin: SourceOriginOfficialCatalog,
		SourceKey:    "official.checkin-policy",
		Revision:     1,
	}
	if !item.IsOfficial() {
		t.Fatalf("managed catalog identity was not recognized: %+v", item)
	}
	item.SourceKey = ""
	if item.IsOfficial() {
		t.Fatal("an official origin without a stable source key must not be managed")
	}
}

func TestQuarantineTransitionsAreReversible(t *testing.T) {
	item := KnowledgeItem{ID: 9, Status: ItemStatusApproved, Approved: true}
	if err := item.Quarantine(3, "license_risk", "许可证状态不明"); err != nil {
		t.Fatal(err)
	}
	if item.EffectiveStatus() != ItemStatusQuarantined || item.Approved || item.Rejected || item.QuarantinedAt == nil || item.QuarantinedBy != 3 {
		t.Fatalf("invalid quarantine state: %+v", item)
	}
	if err := item.RestoreFromQuarantine(3, "许可证已核验"); err != nil {
		t.Fatal(err)
	}
	if item.EffectiveStatus() != ItemStatusApproved || !item.Approved || item.QuarantinedAt != nil || item.QuarantineReason != "" {
		t.Fatalf("invalid restored state: %+v", item)
	}
	if err := item.RestoreFromQuarantine(3, "重复恢复"); !errors.Is(err, ErrInvalidItemState) {
		t.Fatalf("second restore error = %v", err)
	}
}
