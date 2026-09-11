package announcementdraft

import "testing"

func TestDraftTransitionsRequireHumanApproval(t *testing.T) {
	draft, err := New(7, "通知", "正文", "{}", "[]", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := draft.MarkPublished(9, "{}"); err != ErrInvalidDraft {
		t.Fatalf("published unconfirmed draft: %v", err)
	}
	if err := draft.Confirm(); err != nil || draft.Status != StatusConfirmed {
		t.Fatalf("confirm failed: err=%v status=%s", err, draft.Status)
	}
	if err := draft.MarkPublished(9, `{"status":"not_requested"}`); err != nil || draft.Status != StatusPublished {
		t.Fatalf("publish transition failed: err=%v status=%s", err, draft.Status)
	}
	if err := draft.Discard(); err != ErrInvalidDraft {
		t.Fatalf("discarded published draft: %v", err)
	}
}
