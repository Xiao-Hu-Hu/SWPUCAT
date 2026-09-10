package knowledge

import "testing"

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
