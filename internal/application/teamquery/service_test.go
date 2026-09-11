package teamquery

import (
	checkinapp "SWPUCAT/internal/application/checkin"
	reportapp "SWPUCAT/internal/application/report"
	checkindomain "SWPUCAT/internal/domain/checkin"
	reportdomain "SWPUCAT/internal/domain/report"
	"SWPUCAT/internal/domain/user"
	"SWPUCAT/internal/infrastructure/event"
	"context"
	"testing"
	"time"
)

type fakeCheckinRepo struct {
	checkindomain.Repository
	records []*checkindomain.CheckinRecord
}

func (r fakeCheckinRepo) FindByDateRange(context.Context, string, string) ([]*checkindomain.CheckinRecord, error) {
	return r.records, nil
}

type fakeUserRepo struct {
	user.Repository
	users []*user.User
}

func (r fakeUserRepo) FindAll(context.Context) ([]*user.User, error) { return r.users, nil }

type fakeSettings struct{}

func (fakeSettings) Get(context.Context, string) (string, error) { return "", nil }
func (fakeSettings) Set(context.Context, string, string) error   { return nil }

type fakeReportRepo struct{ reportdomain.Repository }

func (fakeReportRepo) ListReports(context.Context, int64, bool, *int64, reportdomain.ReportStatus, int, int) ([]*reportdomain.Report, error) {
	return []*reportdomain.Report{{ID: 10, PeriodID: 5, AuthorID: 1, Status: reportdomain.ReportStatusSubmitted}}, nil
}

func TestQueryAppliesDeterministicHourAndReportFilters(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	checkinSvc := checkinapp.NewCheckinService(fakeCheckinRepo{records: []*checkindomain.CheckinRecord{{ID: 1, UserID: 1, Type: checkindomain.CheckinTypeIn, Date: today, Time: "09:00:00"}, {ID: 2, UserID: 1, Type: checkindomain.CheckinTypeOut, Date: today, Time: "10:00:00"}}}, fakeUserRepo{users: []*user.User{{ID: 1, Nickname: "提交者", Status: user.StatusActive}, {ID: 2, Nickname: "未提交", Status: user.StatusActive}}}, event.NewNoOpPublisher(), fakeSettings{})
	reportSvc := reportapp.NewService(fakeReportRepo{})
	minHours := 2.0
	result, err := NewService(checkinSvc, reportSvc).Query(context.Background(), Request{Period: "today", PeriodID: 5, MinHours: &minHours, ReportStatus: "missing", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Members) != 1 || result.Members[0].UserID != 2 || result.Members[0].ReportState != "missing" {
		t.Fatalf("unexpected filtered team query: %+v", result)
	}
	if len(result.Members[0].Reasons) != 2 {
		t.Fatalf("query reasons did not explain both filters: %+v", result.Members[0].Reasons)
	}
}
