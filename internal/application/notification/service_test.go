package notificationapp

import (
	"SWPUCAT/internal/domain/evaluation"
	"SWPUCAT/internal/domain/notification"
	"SWPUCAT/internal/domain/report"
	"SWPUCAT/internal/domain/user"
	"context"
	"strings"
	"testing"
	"time"
)

type fakeNotificationRepo struct {
	notification.Repository
	items map[string]*notification.Notification
	prefs map[int64]*notification.Preferences
}

func (r *fakeNotificationRepo) Create(_ context.Context, item *notification.Notification) error {
	if r.items == nil {
		r.items = map[string]*notification.Notification{}
	}
	key := strings.Join([]string{string(item.Type), item.DedupeKey, string(rune(item.RecipientID))}, ":")
	if existing, ok := r.items[key]; ok {
		item.ID = existing.ID
		return nil
	}
	item.ID = int64(len(r.items) + 1)
	r.items[key] = item
	return nil
}

func (r *fakeNotificationRepo) GetPreferences(_ context.Context, userID int64) (*notification.Preferences, error) {
	if item, ok := r.prefs[userID]; ok {
		return item, nil
	}
	return &notification.Preferences{UserID: userID, CheckinAnomalyEnabled: true, ReportDueEnabled: true, EvaluationPublishedEnabled: true}, nil
}

func (r *fakeNotificationRepo) SavePreferences(_ context.Context, item *notification.Preferences) error {
	if r.prefs == nil {
		r.prefs = map[int64]*notification.Preferences{}
	}
	r.prefs[item.UserID] = item
	return nil
}

type fakeUserRepo struct {
	user.Repository
	users []*user.User
}

func (r fakeUserRepo) FindAll(context.Context) ([]*user.User, error) { return r.users, nil }

type fakePeriodRepo struct {
	periods []*report.ReportPeriod
	ids     []int64
}

func (r fakePeriodRepo) ListDuePeriods(context.Context, time.Time, time.Time) ([]*report.ReportPeriod, error) {
	return r.periods, nil
}

func (r fakePeriodRepo) ListPendingReportMemberIDs(context.Context, int64) ([]int64, error) {
	return r.ids, nil
}

func TestNotifyCheckinAnomalyIsDeduplicatedAndRespectsPreference(t *testing.T) {
	repo := &fakeNotificationRepo{}
	svc := NewService(repo, nil)
	if err := svc.NotifyCheckinAnomaly(context.Background(), 3, "2026-09-11", []string{"missing_clock_out", "duplicate_clock_in", "missing_clock_out"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.NotifyCheckinAnomaly(context.Background(), 3, "2026-09-11", []string{"duplicate_clock_in"}); err != nil {
		t.Fatal(err)
	}
	if len(repo.items) != 1 {
		t.Fatalf("duplicate anomaly notification was created: %d", len(repo.items))
	}
	for _, item := range repo.items {
		if !strings.Contains(item.BodyMarkdown, "duplicate_clock_in、missing_clock_out") {
			t.Fatalf("anomalies were not normalized: %s", item.BodyMarkdown)
		}
	}
	repo.prefs = map[int64]*notification.Preferences{3: {UserID: 3}}
	if err := svc.NotifyCheckinAnomaly(context.Background(), 3, "2026-09-12", []string{"missing_clock_out"}); err != nil {
		t.Fatal(err)
	}
	if len(repo.items) != 1 {
		t.Fatalf("disabled anomaly preference was ignored")
	}
}

func TestRunReportDueRemindersUsesPendingMembers(t *testing.T) {
	repo := &fakeNotificationRepo{}
	svc := NewService(repo, nil, fakePeriodRepo{periods: []*report.ReportPeriod{{ID: 8, PeriodKey: "2026-09", DueAt: timePtr(time.Now().Add(time.Hour))}}, ids: []int64{3, 4}})
	if err := svc.RunReportDueReminders(context.Background(), time.Now(), 48*time.Hour); err != nil {
		t.Fatal(err)
	}
	if len(repo.items) != 2 {
		t.Fatalf("pending member reminders missing: %d", len(repo.items))
	}
}

func TestNotifyEvaluationPublishedTargetsActiveAudience(t *testing.T) {
	repo := &fakeNotificationRepo{}
	users := []*user.User{{ID: 1, Role: user.RoleCaptain, Status: user.StatusActive}, {ID: 2, Role: user.RoleMember, Status: user.StatusActive}, {ID: 3, Role: user.RoleMember, Status: user.StatusDisabled}}
	svc := NewService(repo, fakeUserRepo{users: users})
	if err := svc.NotifyEvaluationPublished(context.Background(), &evaluation.Draft{ID: 9, CreatedBy: 1, Audience: evaluation.DraftAudienceTeam, Title: "九月考评"}); err != nil {
		t.Fatal(err)
	}
	if len(repo.items) != 2 {
		t.Fatalf("inactive user received notification or active user missed it: %d", len(repo.items))
	}
}

func timePtr(value time.Time) *time.Time { return &value }
