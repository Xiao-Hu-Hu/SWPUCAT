package report

import "context"

type Repository interface {
	CreatePeriod(ctx context.Context, period *ReportPeriod) error
	FindPeriodByID(ctx context.Context, id int64) (*ReportPeriod, error)
	ListPeriods(ctx context.Context) ([]*ReportPeriod, error)
	AddPeriodMember(ctx context.Context, member *PeriodMember) error

	CreateReport(ctx context.Context, item *Report) error
	FindReportByID(ctx context.Context, id int64) (*Report, error)
	ListReports(ctx context.Context, authorID int64, manager bool, periodID *int64, status ReportStatus, page, pageSize int) ([]*Report, error)
	CreateVersion(ctx context.Context, version *ReportVersion, expectedCurrent int) error
	FindVersion(ctx context.Context, reportID int64, version int) (*ReportVersion, error)
	UpdateVersion(ctx context.Context, version *ReportVersion) error
	UpdateReportStatus(ctx context.Context, reportID int64, status ReportStatus) error
	AttachAsset(ctx context.Context, asset *ReportAsset) error
}
