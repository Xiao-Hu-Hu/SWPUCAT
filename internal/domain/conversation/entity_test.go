package conversation

import (
	"errors"
	"testing"
	"time"
)

func TestConversationTracksOwnerAndRollingContext(t *testing.T) {
	item, err := New(7, "session-1", "部署咨询")
	if err != nil {
		t.Fatal(err)
	}
	if !item.IsOwnedBy(7) || item.IsOwnedBy(8) {
		t.Fatal("conversation ownership is incorrect")
	}
	activeAt := time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC)
	if err := item.UpdateContext("已讨论部署", `[{"source_id":12}]`, activeAt); err != nil {
		t.Fatal(err)
	}
	if item.RollingSummary != "已讨论部署" || item.LastActiveAt != activeAt {
		t.Fatalf("context was not updated: %+v", item)
	}
}

func TestNewMessageValidatesRoleAndContent(t *testing.T) {
	if _, err := NewMessage(1, MessageRole("unknown"), "内容", "", "", "[]", 0); !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("invalid role error = %v", err)
	}
	if _, err := NewMessage(1, RoleUser, " ", "", "", "[]", 0); !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("empty content error = %v", err)
	}
}
