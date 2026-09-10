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
			item:     KnowledgeItem{Approved: true, UploaderID: 1},
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
			item:      KnowledgeItem{UploaderID: 1},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.item.IsVisibleTo(tt.viewerID, tt.canReview); got != tt.visible {
				t.Fatalf("IsVisibleTo() = %v, want %v", got, tt.visible)
			}
		})
	}
}
