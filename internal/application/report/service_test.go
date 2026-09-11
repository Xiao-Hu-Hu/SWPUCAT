package reportapp

import (
	domain "SWPUCAT/internal/domain/report"
	"SWPUCAT/internal/domain/shared"
	"context"
	"testing"
)

type fakeReportRepo struct {
	domain.Repository
	period   *domain.ReportPeriod
	report   *domain.Report
	versions map[int]*domain.ReportVersion
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

func TestCreateVersionStoresParserMetadataAndHash(t *testing.T) {
	repo := &fakeReportRepo{report: &domain.Report{ID: 11, AuthorID: 3, CurrentVersion: 0, Status: domain.ReportStatusDraft}, versions: make(map[int]*domain.ReportVersion)}
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
	repo := &fakeReportRepo{period: &domain.ReportPeriod{ID: 1, Status: domain.PeriodStatusOpen, Members: []domain.PeriodMember{{UserID: 3}}}}
	svc := NewService(repo)
	_, err := svc.CreateReport(context.Background(), 4, "其他成员", CreateReportRequest{PeriodID: 1, ReportType: domain.ReportTypeMonthly})
	if err != shared.ErrForbidden {
		t.Fatalf("unlisted member was allowed: %v", err)
	}
}
