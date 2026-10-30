package evaluation

import (
	"testing"
	"time"
)

func TestSnapshotContainsEveryAttendanceMemberAndReportState(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	snapshot, err := NewSnapshot(4, "2026-08", start, end, "Asia/Shanghai", end.Add(24*time.Hour), 3, "checkin-v1", "watermark-7", 9, []AttendanceInput{
		{UserID: 8, DisplayName: "无记录", Required: true},
		{UserID: 3, DisplayName: "成员", Required: true, TotalMinutes: 120, Anomalies: []string{"missing_clock_out", "duplicate_clock_in"}},
	}, []ReportInput{{UserID: 3, Status: ReportReviewed, VersionID: 12, DataState: DataComplete}})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Members) != 2 || snapshot.Members[0].UserID != 3 || snapshot.Members[1].UserID != 8 {
		t.Fatalf("members were not stable and complete: %+v", snapshot.Members)
	}
	if snapshot.Members[0].ReportStatus != ReportReviewed || snapshot.Members[1].ReportStatus != ReportUnknown {
		t.Fatalf("report status was not preserved: %+v", snapshot.Members)
	}
	if snapshot.Members[0].TotalHours != 2 || snapshot.SnapshotHash == "" {
		t.Fatalf("snapshot facts/hash invalid: %+v", snapshot)
	}
}

func TestSnapshotInvalidationIsExplicit(t *testing.T) {
	snapshot := &Snapshot{Status: SnapshotDraft}
	if err := snapshot.Invalidate(); err != nil || snapshot.Status != SnapshotInvalidated {
		t.Fatalf("snapshot was not invalidated: err=%v status=%s", err, snapshot.Status)
	}
	if err := snapshot.Invalidate(); err == nil {
		t.Fatal("invalidated snapshot was changed again")
	}
}

func TestDraftRequiresReviewBeforePublishAndSupportsReturnForRevision(t *testing.T) {
	draft, err := NewDraft(7, 4, 9, DraftAudienceCaptain, "八月考评", "总结", "model-x", "prompt-v1")
	if err != nil {
		t.Fatal(err)
	}
	if err := draft.Publish(); err != ErrSnapshotChanged {
		t.Fatalf("published an unreviewed draft: %v", err)
	}
	if err := draft.Review(true); err != nil || draft.Status != DraftStatusReviewed {
		t.Fatalf("review transition failed: err=%v status=%s", err, draft.Status)
	}
	if err := draft.Review(false); err != nil || draft.Status != DraftStatusDraft {
		t.Fatalf("rejection did not return draft for revision: err=%v status=%s", err, draft.Status)
	}
	if err := draft.Review(true); err != nil {
		t.Fatalf("second review transition failed: %v", err)
	}
	if err := draft.Publish(); err != nil || draft.Status != DraftStatusPublished || draft.PublishedAt == nil {
		t.Fatalf("publish transition failed: err=%v status=%s", err, draft.Status)
	}
	if err := draft.Invalidate(); err != ErrSnapshotChanged {
		t.Fatalf("published draft was invalidated: %v", err)
	}
}
