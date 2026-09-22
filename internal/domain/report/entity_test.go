package report

import (
	"errors"
	"testing"
	"time"
)

func TestReportPeriodRosterVersionAndExemption(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	period, err := NewReportPeriod("2026-08", start, end, "Asia/Shanghai", nil, 9, "队长")
	if err != nil {
		t.Fatal(err)
	}
	if err := period.AddMember(PeriodMember{UserID: 3, DisplayName: "成员", Required: true}); err != nil {
		t.Fatal(err)
	}
	if period.RosterVersion != 2 || len(period.Members) != 1 {
		t.Fatalf("unexpected roster snapshot: version=%d members=%d", period.RosterVersion, len(period.Members))
	}
	if err := period.AddMember(PeriodMember{UserID: 3, DisplayName: "成员", Required: true}); !errors.Is(err, ErrDuplicateMember) {
		t.Fatalf("duplicate member error = %v", err)
	}
	if err := period.AddMember(PeriodMember{UserID: 4, Exempt: true}); !errors.Is(err, ErrInvalidPeriod) {
		t.Fatalf("missing exemption reason error = %v", err)
	}
}

func TestReportVersionIsAppendOnlyAndRequiresBodyOnSubmit(t *testing.T) {
	report, err := NewReport(1, 3, ReportTypeMonthly, "成员")
	if err != nil {
		t.Fatal(err)
	}
	version, err := report.CreateVersion(1, "8 月月报", "", 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := report.MarkVersionCreated(version.Version); err != nil {
		t.Fatal(err)
	}
	if err := version.Submit(3); !errors.Is(err, ErrInvalidVersionState) {
		t.Fatalf("empty version was submitted: %v", err)
	}
	version.BodyMarkdown = "## 成果\n完成了接口治理"
	if err := version.Submit(3); err != nil {
		t.Fatal(err)
	}
	if err := version.Review(true); err != nil || version.Status != VersionStatusReviewed {
		t.Fatalf("review transition failed: err=%v status=%s", err, version.Status)
	}
	if _, err := report.CreateVersion(1, "重复版本", "body", 3); !errors.Is(err, ErrInvalidVersion) {
		t.Fatalf("version overwrite was allowed: %v", err)
	}
}

func TestReportVersionAssetIsUniqueByRole(t *testing.T) {
	version := &ReportVersion{ID: 5, Version: 1, Status: VersionStatusDraft}
	asset := ReportAsset{KnowledgeItemID: 8, AssetRole: "evidence"}
	if err := version.AttachAsset(asset); err != nil {
		t.Fatal(err)
	}
	if err := version.AttachAsset(asset); !errors.Is(err, ErrDuplicateAsset) {
		t.Fatalf("duplicate asset was allowed: %v", err)
	}
}

func TestReportPeriodDeadlineFallsBackToEndsOn(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)

	withDueAt, err := NewReportPeriod("2026-08", start, end, "Asia/Shanghai", &end, 9, "队长")
	if err != nil {
		t.Fatal(err)
	}
	if !withDueAt.Deadline().Equal(end) || withDueAt.Deadline().Equal(start) {
		t.Fatalf("Deadline() should return due_at when set: %v", withDueAt.Deadline())
	}

	withoutDueAt, err := NewReportPeriod("2026-07", start, end, "Asia/Shanghai", nil, 9, "队长")
	if err != nil {
		t.Fatal(err)
	}
	if !withoutDueAt.Deadline().Equal(end) {
		t.Fatalf("Deadline() should fall back to ends_on when due_at is nil: %v", withoutDueAt.Deadline())
	}
}

func TestReportPeriodAcceptanceWindows(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	due := end.Add(24 * time.Hour)
	period, err := NewReportPeriod("2026-08", start, end, "Asia/Shanghai", &due, 9, "队长")
	if err != nil {
		t.Fatal(err)
	}

	// now <= ends_on: active, submittable, not late, not locked.
	active := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	if !period.AcceptsSubmissions(active) || period.IsLocked(active) {
		t.Fatalf("active window should accept submissions: accepts=%v locked=%v", period.AcceptsSubmissions(active), period.IsLocked(active))
	}

	// ends_on < now <= due_at: late window, still submittable but flagged late.
	late := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	if !period.AcceptsSubmissions(late) || period.IsLocked(late) {
		t.Fatalf("late window should still accept submissions: accepts=%v locked=%v", period.AcceptsSubmissions(late), period.IsLocked(late))
	}

	// now > due_at: locked, no submissions allowed.
	locked := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	if period.AcceptsSubmissions(locked) || !period.IsLocked(locked) {
		t.Fatalf("past-deadline window should be locked: accepts=%v locked=%v", period.AcceptsSubmissions(locked), period.IsLocked(locked))
	}

	// A closed period never accepts submissions, even inside the active window.
	closed := *period
	closed.Status = PeriodStatusClosed
	if closed.AcceptsSubmissions(active) {
		t.Fatalf("closed period should reject submissions: accepts=%v", closed.AcceptsSubmissions(active))
	}
}
