package reportapp

import (
	domain "SWPUCAT/internal/domain/report"
	"context"
	"testing"
	"time"
)

type fakeReportRepo struct {
	domain.Repository
	period         *domain.ReportPeriod
	report         *domain.Report
	versions       map[int]*domain.ReportVersion
	lateSubmission bool
}

func (r *fakeReportRepo) FindPeriodByID(context.Context, int64) (*domain.ReportPeriod, error) {
	return r.period, nil
}

func (r *fakeReportRepo) CreateReport(_ context.Context, item *domain.Report) error {
	item.ID = 11
	r.report = item
	return nil
}

func (r *fakeReportRepo) FindReportByID(context.Context, int64) (*domain.Report, error) {
	return r.report, nil
}

func (r *fakeReportRepo) CreateVersion(_ context.Context, item *domain.ReportVersion, _ int) error {
	item.ID = 21
	if r.versions == nil {
		r.versions = make(map[int]*domain.ReportVersion)
	}
	r.versions[item.Version] = item
	r.report.CurrentVersion = item.Version
	return nil
}

func (r *fakeReportRepo) FindVersion(_ context.Context, _ int64, version int) (*domain.ReportVersion, error) {
	return r.versions[version], nil
}

func (r *fakeReportRepo) UpdateVersion(context.Context, *domain.ReportVersion) error { return nil }
func (r *fakeReportRepo) UpdateReportStatus(context.Context, int64, domain.ReportStatus) error {
	return nil
}
func (r *fakeReportRepo) UpdateReportLateSubmission(_ context.Context, _ int64, late bool) error {
	r.lateSubmission = late
	return nil
}

// openPeriod returns an open period whose submission deadline is still in the
// future (the reporter may submit), so roster/lock tests don't trip the other.
func openPeriod(endsOffset, dueOffset time.Duration) *domain.ReportPeriod {
	now := time.Now()
	return &domain.ReportPeriod{ID: 1, Status: domain.PeriodStatusOpen, StartsOn: now.Add(-24 * time.Hour), EndsOn: now.Add(endsOffset), DueAt: timePtr(now.Add(dueOffset))}
}

func TestCreateVersionStoresParserMetadataAndHash(t *testing.T) {
	repo := &fakeReportRepo{
		report:   &domain.Report{ID: 11, PeriodID: 1, AuthorID: 3, CurrentVersion: 0, Status: domain.ReportStatusDraft},
		versions: make(map[int]*domain.ReportVersion),
		period:   openPeriod(24*time.Hour, 48*time.Hour),
	}
	svc := NewService(repo)
	version, err := svc.CreateVersion(context.Background(), 11, 3, false, CreateVersionRequest{ExpectedVersion: 0, Title: "8 月月报", BodyMarkdown: "## 本月成果\n完成接口治理"})
	if err != nil {
		t.Fatal(err)
	}
	if version.ParseStatus != string(domain.ParseStatusReady) || version.ParserVersion == "" || version.ContentHash == "" || version.ParsedJSON == nil {
		t.Fatalf("parser metadata missing: %+v", version)
	}
}

func TestCreateReportRequiresConfiguredRosterMember(t *testing.T) {
	repo := &fakeReportRepo{period: &domain.ReportPeriod{ID: 1, Status: domain.PeriodStatusOpen, EndsOn: time.Now().Add(24 * time.Hour), DueAt: timePtr(time.Now().Add(48 * time.Hour)), Members: []domain.PeriodMember{{UserID: 3, Required: true}}}}
	svc := NewService(repo)
	_, err := svc.CreateReport(context.Background(), 4, "其他成员", CreateReportRequest{PeriodID: 1, ReportType: domain.ReportTypeMonthly})
	if err != domain.ErrNotInRoster {
		t.Fatalf("unlisted member was allowed: %v", err)
	}
}

func TestCreateReportAllowsWhenNoSubmissionRoster(t *testing.T) {
	repo := &fakeReportRepo{period: openPeriod(24*time.Hour, 48*time.Hour)}
	svc := NewService(repo)
	if _, err := svc.CreateReport(context.Background(), 4, "其他成员", CreateReportRequest{PeriodID: 1, ReportType: domain.ReportTypeMonthly}); err != nil {
		t.Fatalf("empty roster should allow anyone: %v", err)
	}
}

func TestCreateReportRejectsLockedPeriod(t *testing.T) {
	repo := &fakeReportRepo{period: &domain.ReportPeriod{ID: 1, Status: domain.PeriodStatusOpen, EndsOn: time.Now().Add(-48 * time.Hour), DueAt: timePtr(time.Now().Add(-24 * time.Hour))}}
	svc := NewService(repo)
	_, err := svc.CreateReport(context.Background(), 3, "成员", CreateReportRequest{PeriodID: 1, ReportType: domain.ReportTypeMonthly})
	if err != domain.ErrPeriodClosed {
		t.Fatalf("locked period should reject create: %v", err)
	}
}

func TestSubmitVersionRejectsLockedPeriod(t *testing.T) {
	repo := &fakeReportRepo{
		report: &domain.Report{ID: 11, PeriodID: 1, AuthorID: 3, Status: domain.ReportStatusDraft},
		period: &domain.ReportPeriod{ID: 1, Status: domain.PeriodStatusOpen, EndsOn: time.Now().Add(-48 * time.Hour), DueAt: timePtr(time.Now().Add(-24 * time.Hour))},
	}
	svc := NewService(repo)
	_, err := svc.SubmitVersion(context.Background(), 11, 1, 3, false)
	if err != domain.ErrPeriodClosed {
		t.Fatalf("locked period should reject submit: %v", err)
	}
}

func TestSubmitVersionMarksLate(t *testing.T) {
	repo := &fakeReportRepo{
		report:   &domain.Report{ID: 11, PeriodID: 1, AuthorID: 3, Status: domain.ReportStatusDraft},
		versions: map[int]*domain.ReportVersion{1: {ID: 21, ReportID: 11, Version: 1, Status: domain.VersionStatusDraft, BodyMarkdown: "正文"}},
		period:   &domain.ReportPeriod{ID: 1, Status: domain.PeriodStatusOpen, EndsOn: time.Now().Add(-2 * time.Hour), DueAt: timePtr(time.Now().Add(24 * time.Hour))},
	}
	svc := NewService(repo)
	if _, err := svc.SubmitVersion(context.Background(), 11, 1, 3, false); err != nil {
		t.Fatalf("late-but-allowed submit should succeed: %v", err)
	}
	if !repo.report.LateSubmission || !repo.lateSubmission {
		t.Fatalf("late submission not marked: report=%+v late=%v", repo.report, repo.lateSubmission)
	}
}

func TestIsPeriodMember(t *testing.T) {
	repo := &fakeReportRepo{period: &domain.ReportPeriod{ID: 1, Status: domain.PeriodStatusOpen, Members: []domain.PeriodMember{{UserID: 3, Required: true}, {UserID: 7, Required: true, Exempt: true}}}}
	svc := NewService(repo)

	member, err := svc.IsPeriodMember(context.Background(), 1, 3)
	if err != nil || !member {
		t.Fatalf("roster member not recognized: member=%v err=%v", member, err)
	}

	exemptMember, err := svc.IsPeriodMember(context.Background(), 1, 7)
	if err != nil || exemptMember {
		t.Fatalf("exempt member recognized as submitter: member=%v err=%v", exemptMember, err)
	}

	nonMember, err := svc.IsPeriodMember(context.Background(), 1, 99)
	if err != nil || nonMember {
		t.Fatalf("non-roster user recognized: member=%v err=%v", nonMember, err)
	}
}
