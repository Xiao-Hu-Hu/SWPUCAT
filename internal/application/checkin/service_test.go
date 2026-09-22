package checkin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	checkindomain "SWPUCAT/internal/domain/checkin"
	userdomain "SWPUCAT/internal/domain/user"
)

type personalCheckinRepo struct {
	checkindomain.Repository
	records       []*checkindomain.CheckinRecord
	requestedUser int64
	requestedFrom string
	requestedTo   string
	err           error
}

func (r *personalCheckinRepo) FindByUserIDAndDateRange(_ context.Context, userID int64, from, to string) ([]*checkindomain.CheckinRecord, error) {
	r.requestedUser, r.requestedFrom, r.requestedTo = userID, from, to
	return r.records, r.err
}

type personalUserRepo struct {
	userdomain.Repository
	member      *userdomain.User
	requestedID int64
	err         error
}

type onlineCheckinRepo struct {
	checkindomain.Repository
	online map[int64]bool
	dates  []string
}

func (r *onlineCheckinRepo) GetLastByUserAndDate(_ context.Context, userID int64, date string) (*checkindomain.CheckinRecord, error) {
	r.dates = append(r.dates, date)
	if !r.online[userID] {
		return nil, nil
	}
	return &checkindomain.CheckinRecord{UserID: userID, Type: checkindomain.CheckinTypeIn, Date: date}, nil
}

type onlineUserRepo struct {
	userdomain.Repository
	members []*userdomain.User
}

func (r *onlineUserRepo) FindAll(context.Context) ([]*userdomain.User, error) {
	return r.members, nil
}

func (r *personalUserRepo) FindByID(_ context.Context, id int64) (*userdomain.User, error) {
	r.requestedID = id
	return r.member, r.err
}

type checkinSettingsStub struct {
	value string
	err   error
}

func (s checkinSettingsStub) Get(context.Context, string) (string, error) { return s.value, s.err }
func (s checkinSettingsStub) Set(context.Context, string, string) error   { return nil }

func TestGetPersonalWorkHoursCalculatesWeekMonthAndRequirement(t *testing.T) {
	fixedNow := time.Date(2026, time.September, 17, 12, 0, 0, 0, time.Local)
	repo := &personalCheckinRepo{records: []*checkindomain.CheckinRecord{
		checkinRecord(1, 7, checkindomain.CheckinTypeIn, "2026-09-02", "08:00:00"),
		checkinRecord(2, 7, checkindomain.CheckinTypeOut, "2026-09-02", "10:00:00"),
		checkinRecord(3, 7, checkindomain.CheckinTypeIn, "2026-09-15", "09:00:00"),
		checkinRecord(4, 7, checkindomain.CheckinTypeOut, "2026-09-15", "12:00:00"),
		checkinRecord(5, 7, checkindomain.CheckinTypeIn, "2026-09-16", "13:00:00"),
		checkinRecord(6, 7, checkindomain.CheckinTypeOut, "2026-09-16", "14:00:00"),
	}}
	users := &personalUserRepo{member: &userdomain.User{ID: 7, StudentID: userdomain.StudentID("202431060420"), Nickname: userdomain.Nickname("成员甲")}}
	service := NewCheckinService(repo, users, nil, checkinSettingsStub{})
	service.now = func() time.Time { return fixedNow }

	result, err := service.GetPersonalWorkHours(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetPersonalWorkHours() error = %v", err)
	}
	if repo.requestedUser != 7 || users.requestedID != 7 {
		t.Fatalf("service read a different member: records=%d user=%d", repo.requestedUser, users.requestedID)
	}
	if repo.requestedFrom != "2026-09-01" || repo.requestedTo != "2026-09-17" {
		t.Fatalf("repository range = %s..%s", repo.requestedFrom, repo.requestedTo)
	}
	if result.AsOfDate != "2026-09-17" || result.Week.StartsOn != "2026-09-14" || result.Week.EndsOn != "2026-09-17" {
		t.Fatalf("unexpected dates: %+v", result)
	}
	if result.Week.TotalMinutes != 240 || result.Week.TotalHours != 4 || result.Week.RecordDays != 2 {
		t.Fatalf("week summary = %+v", result.Week)
	}
	if result.Month.TotalMinutes != 360 || result.Month.TotalHours != 6 || result.Month.RecordDays != 3 {
		t.Fatalf("month summary = %+v", result.Month)
	}
	requirement := result.WeeklyRequirement
	if requirement.Grade != 3 || requirement.RequiredMinutes == nil || *requirement.RequiredMinutes != 360 {
		t.Fatalf("weekly requirement = %+v", requirement)
	}
	if requirement.IsMet == nil || *requirement.IsMet || requirement.ShortfallMinutes == nil || *requirement.ShortfallMinutes != 120 || requirement.Status != "below_target" {
		t.Fatalf("weekly progress = %+v", requirement)
	}
}

func TestGetPersonalWorkHoursMarksExactConfiguredRequirementMet(t *testing.T) {
	fixedNow := time.Date(2026, time.September, 17, 12, 0, 0, 0, time.Local)
	repo := &personalCheckinRepo{records: []*checkindomain.CheckinRecord{
		checkinRecord(1, 8, checkindomain.CheckinTypeIn, "2026-09-16", "08:00:00"),
		checkinRecord(2, 8, checkindomain.CheckinTypeOut, "2026-09-16", "12:00:00"),
	}}
	users := &personalUserRepo{member: &userdomain.User{ID: 8, StudentID: userdomain.StudentID("202431060421"), Nickname: userdomain.Nickname("成员乙")}}
	configured, err := json.Marshal(RequirementsDTO{Requirements: []CheckinRequirement{
		{Grade: 3, Minutes: 600},
		{Grade: 3, Minutes: 240},
	}})
	if err != nil {
		t.Fatal(err)
	}
	service := NewCheckinService(repo, users, nil, checkinSettingsStub{value: string(configured)})
	service.now = func() time.Time { return fixedNow }

	result, err := service.GetPersonalWorkHours(context.Background(), 8)
	if err != nil {
		t.Fatal(err)
	}
	progress := result.WeeklyRequirement
	if progress.IsMet == nil || !*progress.IsMet || progress.ShortfallMinutes == nil || *progress.ShortfallMinutes != 0 || progress.Status != "met" {
		t.Fatalf("weekly progress = %+v", progress)
	}
}

func TestGetPersonalWorkHoursIncludesPreviousMonthWhenWeekCrossesBoundary(t *testing.T) {
	fixedNow := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.Local)
	repo := &personalCheckinRepo{records: []*checkindomain.CheckinRecord{
		checkinRecord(1, 8, checkindomain.CheckinTypeIn, "2026-08-31", "08:00:00"),
		checkinRecord(2, 8, checkindomain.CheckinTypeOut, "2026-08-31", "09:00:00"),
		checkinRecord(3, 8, checkindomain.CheckinTypeIn, "2026-09-01", "08:00:00"),
		checkinRecord(4, 8, checkindomain.CheckinTypeOut, "2026-09-01", "10:00:00"),
	}}
	users := &personalUserRepo{member: &userdomain.User{ID: 8, StudentID: userdomain.StudentID("202631060421"), Nickname: userdomain.Nickname("成员乙")}}
	service := NewCheckinService(repo, users, nil, checkinSettingsStub{})
	service.now = func() time.Time { return fixedNow }

	result, err := service.GetPersonalWorkHours(context.Background(), 8)
	if err != nil {
		t.Fatal(err)
	}
	if repo.requestedFrom != "2026-08-31" || repo.requestedTo != "2026-09-01" {
		t.Fatalf("repository range = %s..%s", repo.requestedFrom, repo.requestedTo)
	}
	if result.Week.TotalMinutes != 180 || result.Month.TotalMinutes != 120 {
		t.Fatalf("cross-boundary summaries: week=%+v month=%+v", result.Week, result.Month)
	}
}

func TestGetPersonalWorkHoursKeepsUnknownRequirementUnknown(t *testing.T) {
	fixedNow := time.Date(2026, time.September, 17, 12, 0, 0, 0, time.Local)
	repo := &personalCheckinRepo{}
	users := &personalUserRepo{member: &userdomain.User{ID: 9, Nickname: userdomain.Nickname("无学号成员")}}
	service := NewCheckinService(repo, users, nil, checkinSettingsStub{})
	service.now = func() time.Time { return fixedNow }

	result, err := service.GetPersonalWorkHours(context.Background(), 9)
	if err != nil {
		t.Fatal(err)
	}
	if result.WeeklyRequirement.Status != "unknown" || result.WeeklyRequirement.IsMet != nil || result.WeeklyRequirement.RequiredMinutes != nil {
		t.Fatalf("unknown requirement was treated as configured: %+v", result.WeeklyRequirement)
	}
	if result.Week.AttendanceState != "absent" || len(result.Week.Anomalies) != 1 || result.Week.Anomalies[0] != "no_checkin" {
		t.Fatalf("zero-record week = %+v", result.Week)
	}
}

func TestGetPersonalWorkHoursPropagatesScopedRepositoryError(t *testing.T) {
	want := errors.New("query failed")
	repo := &personalCheckinRepo{err: want}
	users := &personalUserRepo{member: &userdomain.User{ID: 10, Nickname: userdomain.Nickname("成员")}}
	service := NewCheckinService(repo, users, nil, checkinSettingsStub{})
	service.now = func() time.Time { return time.Date(2026, time.September, 17, 12, 0, 0, 0, time.Local) }

	_, err := service.GetPersonalWorkHours(context.Background(), 10)
	if !errors.Is(err, want) {
		t.Fatalf("GetPersonalWorkHours() error = %v, want %v", err, want)
	}
}

func TestSummarizeCheckinRecordsReportsIncompleteTransitions(t *testing.T) {
	records := []*checkindomain.CheckinRecord{
		checkinRecord(1, 7, checkindomain.CheckinTypeOut, "2026-09-17", "08:00:00"),
		checkinRecord(2, 7, checkindomain.CheckinTypeIn, "2026-09-17", "09:00:00"),
	}
	result := summarizeCheckinRecords(7, "成员", records)
	if result.TotalMinutes != 0 || result.Status != "attention" {
		t.Fatalf("summary = %+v", result)
	}
	if len(result.Anomalies) != 2 || result.Anomalies[0] != "missing_clock_out" || result.Anomalies[1] != "orphan_clock_out" {
		t.Fatalf("anomalies = %#v", result.Anomalies)
	}
}

func TestGetOnlineSummaryMinimizesMemberResponse(t *testing.T) {
	fixedNow := time.Date(2026, time.September, 18, 12, 0, 0, 0, time.Local)
	users := &onlineUserRepo{members: []*userdomain.User{
		{ID: 7, Nickname: userdomain.Nickname("当前成员")},
		{ID: 8, Nickname: userdomain.Nickname("其他成员")},
	}}
	repo := &onlineCheckinRepo{online: map[int64]bool{7: true, 8: true}}
	service := NewCheckinService(repo, users, nil, checkinSettingsStub{})
	service.now = func() time.Time { return fixedNow }

	result, err := service.GetOnlineSummary(context.Background(), 7, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.AsOfDate != "2026-09-18" || result.TotalMembers != 2 || result.OnlineCount != 2 || result.SelfOnline == nil || !*result.SelfOnline {
		t.Fatalf("summary = %+v", result)
	}
	if result.OnlineMembers != nil || result.Truncated {
		t.Fatalf("member response exposed names: %+v", result)
	}
	for _, date := range repo.dates {
		if date != "2026-09-18" {
			t.Fatalf("repository date = %q", date)
		}
	}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"user_id", "token", "ip", "device", "activity", "其他成员"} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("privacy-minimized payload leaked %q: %s", forbidden, payload)
		}
	}
}

func TestGetOnlineSummaryManagerNamesAreSortedAndStablyTruncated(t *testing.T) {
	fixedNow := time.Date(2026, time.September, 18, 12, 0, 0, 0, time.Local)
	members := make([]*userdomain.User, 0, 53)
	online := make(map[int64]bool, 53)
	for index := 52; index >= 0; index-- {
		id := int64(index + 1)
		members = append(members, &userdomain.User{ID: id, Nickname: userdomain.Nickname(fmt.Sprintf("成员%02d", index))})
		online[id] = true
	}
	service := NewCheckinService(&onlineCheckinRepo{online: online}, &onlineUserRepo{members: members}, nil, checkinSettingsStub{})
	service.now = func() time.Time { return fixedNow }

	result, err := service.GetOnlineSummary(context.Background(), 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.OnlineCount != 53 || len(result.OnlineMembers) != 50 || !result.Truncated {
		t.Fatalf("bounded summary = %+v", result)
	}
	want := make([]string, 50)
	for index := range want {
		want[index] = fmt.Sprintf("成员%02d", index)
	}
	if !reflect.DeepEqual(result.OnlineMembers, want) {
		t.Fatalf("online names = %#v, want %#v", result.OnlineMembers, want)
	}
}

func checkinRecord(id, userID int64, kind checkindomain.CheckinType, date, clock string) *checkindomain.CheckinRecord {
	parsed, _ := time.Parse("2006-01-02 15:04:05", date+" "+clock)
	return &checkindomain.CheckinRecord{ID: id, UserID: userID, Type: kind, Date: date, Time: clock, CreatedAt: parsed}
}

type makeupCheckinRepo struct {
	checkindomain.Repository
	record *checkindomain.CheckinRecord
	err    error
}

func (r *makeupCheckinRepo) CreateMakeup(_ context.Context, record *checkindomain.CheckinRecord) error {
	r.record = record
	return r.err
}

func TestMakeupStoresSingleMakeupRecordWithMinutes(t *testing.T) {
	repo := &makeupCheckinRepo{}
	users := &personalUserRepo{member: &userdomain.User{ID: 5, Status: userdomain.StatusActive}}
	service := NewCheckinService(repo, users, nil, checkinSettingsStub{})

	err := service.Makeup(context.Background(), MakeupRequest{UserID: 5, Date: "2026-09-15", Minutes: 90})
	if err != nil {
		t.Fatalf("Makeup() error = %v", err)
	}
	if repo.record == nil || repo.record.Type != checkindomain.CheckinTypeMakeup || repo.record.Minutes != 90 ||
		repo.record.Date != "2026-09-15" || repo.record.Time != "" {
		t.Fatalf("unexpected makeup record: %+v", repo.record)
	}
}

func TestMakeupRejectsInvalidMinutesAndDate(t *testing.T) {
	repo := &makeupCheckinRepo{}
	users := &personalUserRepo{member: &userdomain.User{ID: 5, Status: userdomain.StatusActive}}
	service := NewCheckinService(repo, users, nil, checkinSettingsStub{})

	if err := service.Makeup(context.Background(), MakeupRequest{UserID: 5, Date: "2026-09-15", Minutes: 0}); err == nil {
		t.Fatal("zero minutes should be rejected")
	}
	if err := service.Makeup(context.Background(), MakeupRequest{UserID: 5, Date: "2026-09-15", Minutes: 60}); err != nil {
		t.Fatalf("valid makeup should succeed: %v", err)
	}
	if repo.record == nil || repo.record.Minutes != 60 {
		t.Fatalf("makeup record = %+v", repo.record)
	}
}

func TestSummarizeCheckinRecordsAddsMakeupMinutes(t *testing.T) {
	records := []*checkindomain.CheckinRecord{
		checkinRecord(1, 7, checkindomain.CheckinTypeMakeup, "2026-09-15", ""),
		checkinRecord(2, 7, checkindomain.CheckinTypeIn, "2026-09-15", "08:00:00"),
		checkinRecord(3, 7, checkindomain.CheckinTypeOut, "2026-09-15", "10:00:00"),
	}
	records[0].Minutes = 90
	result := summarizeCheckinRecords(7, "成员", records)
	if result.TotalMinutes != 210 {
		t.Fatalf("summary = %+v, want total 210 (90 makeup + 120 in/out)", result)
	}
	if result.Status != "normal" || len(result.Anomalies) != 0 {
		t.Fatalf("makeup should not be an anomaly: %+v", result)
	}
}
