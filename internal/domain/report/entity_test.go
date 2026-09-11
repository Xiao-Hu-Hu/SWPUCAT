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
