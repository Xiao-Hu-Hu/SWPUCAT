package evaluation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

type SnapshotStatus string

const (
	SnapshotDraft       SnapshotStatus = "draft"
	SnapshotPublished   SnapshotStatus = "published"
	SnapshotInvalidated SnapshotStatus = "invalidated"
)

type ReportStatus string

const (
	ReportSubmitted ReportStatus = "submitted"
	ReportReviewed  ReportStatus = "reviewed"
	ReportMissing   ReportStatus = "missing"
	ReportUnparsed  ReportStatus = "unparsed"
	ReportUnknown   ReportStatus = "unknown"
)

type DataState string

const (
	DataComplete DataState = "complete"
	DataPartial  DataState = "partial"
	DataUnknown  DataState = "unknown"
)

var (
	ErrInvalidSnapshot = errors.New("invalid evaluation snapshot")
	ErrSnapshotChanged = errors.New("evaluation snapshot is immutable")
)

type Snapshot struct {
	ID              int64
	PeriodID        int64
	PeriodKey       string
	StartsOn        time.Time
	EndsOn          time.Time
	Timezone        string
	CutoffAt        time.Time
	RosterVersion   int64
	RuleVersion     string
	SourceWatermark string
	SnapshotHash    string
	Status          SnapshotStatus
	CreatedBy       int64
	CreatedAt       time.Time
	Members         []Member
}

type Member struct {
	SnapshotID      int64
	UserID          int64
	DisplayName     string
	Required        bool
	Exempt          bool
	TotalMinutes    float64
	TotalHours      float64
	AttendanceDays  int
	AchievedWeeks   int
	Anomalies       []string
	ReportStatus    ReportStatus
	ReportVersionID int64
	DataState       DataState
	UnknownReason   string
}

type AttendanceInput struct {
	UserID         int64
	DisplayName    string
	Required       bool
	Exempt         bool
	TotalMinutes   float64
	AttendanceDays int
	AchievedWeeks  int
	Anomalies      []string
}

type ReportInput struct {
	UserID        int64
	Status        ReportStatus
	VersionID     int64
	DataState     DataState
	UnknownReason string
}

func NewSnapshot(periodID int64, periodKey string, startsOn, endsOn time.Time, timezone string, cutoffAt time.Time, rosterVersion int64, ruleVersion, sourceWatermark string, createdBy int64, attendance []AttendanceInput, reports []ReportInput) (*Snapshot, error) {
	if periodID <= 0 || strings.TrimSpace(periodKey) == "" || startsOn.IsZero() || endsOn.IsZero() || startsOn.After(endsOn) || strings.TrimSpace(timezone) == "" || cutoffAt.IsZero() || rosterVersion <= 0 || strings.TrimSpace(ruleVersion) == "" || createdBy <= 0 {
		return nil, ErrInvalidSnapshot
	}
	reportMap := make(map[int64]ReportInput, len(reports))
	for _, item := range reports {
		reportMap[item.UserID] = item
	}
	members := make([]Member, 0, len(attendance))
	for _, item := range attendance {
		if item.UserID <= 0 || item.TotalMinutes < 0 || item.AttendanceDays < 0 || item.AchievedWeeks < 0 {
			return nil, ErrInvalidSnapshot
		}
		anomalies := append([]string(nil), item.Anomalies...)
		sort.Strings(anomalies)
		member := Member{UserID: item.UserID, DisplayName: strings.TrimSpace(item.DisplayName), Required: item.Required, Exempt: item.Exempt, TotalMinutes: item.TotalMinutes, TotalHours: item.TotalMinutes / 60, AttendanceDays: item.AttendanceDays, AchievedWeeks: item.AchievedWeeks, Anomalies: anomalies, ReportStatus: ReportUnknown, DataState: DataComplete}
		if report, ok := reportMap[item.UserID]; ok {
			member.ReportStatus, member.ReportVersionID, member.DataState, member.UnknownReason = report.Status, report.VersionID, report.DataState, strings.TrimSpace(report.UnknownReason)
		}
		members = append(members, member)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].UserID < members[j].UserID })
	snapshot := &Snapshot{PeriodID: periodID, PeriodKey: periodKey, StartsOn: startsOn, EndsOn: endsOn, Timezone: timezone, CutoffAt: cutoffAt, RosterVersion: rosterVersion, RuleVersion: ruleVersion, SourceWatermark: sourceWatermark, Status: SnapshotDraft, CreatedBy: createdBy, CreatedAt: time.Now(), Members: members}
	snapshot.SnapshotHash = snapshotHash(snapshot)
	return snapshot, nil
}

func (s *Snapshot) Invalidate() error {
	if s == nil || s.Status == SnapshotInvalidated {
		return ErrSnapshotChanged
	}
	s.Status = SnapshotInvalidated
	return nil
}

func snapshotHash(snapshot *Snapshot) string {
	payload := struct {
		PeriodID, RosterVersion, CreatedBy                                            int64
		PeriodKey, StartsOn, EndsOn, Timezone, CutoffAt, RuleVersion, SourceWatermark string
		Members                                                                       []Member
	}{snapshot.PeriodID, snapshot.RosterVersion, snapshot.CreatedBy, snapshot.PeriodKey, snapshot.StartsOn.Format(time.RFC3339), snapshot.EndsOn.Format(time.RFC3339), snapshot.Timezone, snapshot.CutoffAt.Format(time.RFC3339), snapshot.RuleVersion, snapshot.SourceWatermark, snapshot.Members}
	data, _ := json.Marshal(payload)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
