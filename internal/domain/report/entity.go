package report

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type PeriodStatus string

const (
	PeriodStatusOpen     PeriodStatus = "open"
	PeriodStatusClosed   PeriodStatus = "closed"
	PeriodStatusArchived PeriodStatus = "archived"
)

type ReportType string

const (
	ReportTypeMonthly ReportType = "monthly"
	ReportTypeTeam    ReportType = "team"
)

type ReportStatus string

const (
	ReportStatusDraft     ReportStatus = "draft"
	ReportStatusSubmitted ReportStatus = "submitted"
	ReportStatusReviewed  ReportStatus = "reviewed"
	ReportStatusArchived  ReportStatus = "archived"
)

type VersionStatus string

const (
	VersionStatusDraft       VersionStatus = "draft"
	VersionStatusSubmitted   VersionStatus = "submitted"
	VersionStatusReviewed    VersionStatus = "reviewed"
	VersionStatusRejected    VersionStatus = "rejected"
	VersionStatusInvalidated VersionStatus = "invalidated"
)

type ParseStatus string

const (
	ParseStatusPending    ParseStatus = "pending"
	ParseStatusProcessing ParseStatus = "processing"
	ParseStatusReady      ParseStatus = "ready"
	ParseStatusFailed     ParseStatus = "failed"
)

var (
	ErrInvalidPeriod       = errors.New("invalid report period")
	ErrInvalidReport       = errors.New("invalid report")
	ErrInvalidVersion      = errors.New("invalid report version")
	ErrInvalidVersionState = errors.New("invalid report version state")
	ErrDuplicateMember     = errors.New("report period member already exists")
	ErrDuplicateAsset      = errors.New("report version asset already exists")
)

type ReportPeriod struct {
	ID            int64
	PeriodKey     string
	StartsOn      time.Time
	EndsOn        time.Time
	Timezone      string
	DueAt         *time.Time
	Status        PeriodStatus
	RosterVersion int64
	CreatedBy     int64
	CreatedByName string
	Members       []PeriodMember
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type PeriodMember struct {
	PeriodID        int64
	UserID          int64
	DisplayName     string
	Required        bool
	Exempt          bool
	ExemptionReason string
	EffectiveFrom   *time.Time
	EffectiveUntil  *time.Time
}

type Report struct {
	ID             int64
	PeriodID       int64
	Type           ReportType
	AuthorID       int64
	AuthorName     string
	Status         ReportStatus
	Visibility     string
	CurrentVersion int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ReportVersion struct {
	ID            int64
	ReportID      int64
	Version       int
	Status        VersionStatus
	Title         string
	BodyMarkdown  string
	ContentHash   string
	ParseStatus   ParseStatus
	ParseError    string
	ParserVersion string
	ParsedJSON    string
	ParsedAt      *time.Time
	CreatedBy     int64
	SubmittedBy   int64
	SubmittedAt   *time.Time
	CreatedAt     time.Time
	Assets        []ReportAsset
}

type ReportAsset struct {
	ReportVersionID int64
	KnowledgeItemID int64
	AssetRole       string
	Locator         string
	Note            string
}

var periodKeyPattern = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

func NewReportPeriod(periodKey string, startsOn, endsOn time.Time, timezone string, dueAt *time.Time, createdBy int64, createdByName string) (*ReportPeriod, error) {
	periodKey = strings.TrimSpace(periodKey)
	timezone = strings.TrimSpace(timezone)
	if !periodKeyPattern.MatchString(periodKey) || startsOn.IsZero() || endsOn.IsZero() || startsOn.After(endsOn) || timezone == "" || createdBy <= 0 {
		return nil, ErrInvalidPeriod
	}
	return &ReportPeriod{
		PeriodKey: periodKey, StartsOn: startsOn, EndsOn: endsOn, Timezone: timezone,
		DueAt: dueAt, Status: PeriodStatusOpen, RosterVersion: 1,
		CreatedBy: createdBy, CreatedByName: strings.TrimSpace(createdByName),
		CreatedAt: time.Now(), UpdatedAt: time.Now(), Members: make([]PeriodMember, 0),
	}, nil
}

func (p *ReportPeriod) AddMember(member PeriodMember) error {
	if p == nil || p.Status != PeriodStatusOpen || member.UserID <= 0 || member.Exempt && strings.TrimSpace(member.ExemptionReason) == "" || member.EffectiveFrom != nil && member.EffectiveUntil != nil && member.EffectiveFrom.After(*member.EffectiveUntil) {
		return ErrInvalidPeriod
	}
	for _, existing := range p.Members {
		if existing.UserID == member.UserID {
			return ErrDuplicateMember
		}
	}
	member.PeriodID = p.ID
	p.Members = append(p.Members, member)
	p.RosterVersion++
	p.UpdatedAt = time.Now()
	return nil
}

func (p *ReportPeriod) Close() error {
	if p == nil || p.Status != PeriodStatusOpen {
		return ErrInvalidPeriod
	}
	p.Status = PeriodStatusClosed
	p.UpdatedAt = time.Now()
	return nil
}

func NewReport(periodID, authorID int64, reportType ReportType, authorName string) (*Report, error) {
	if periodID <= 0 || authorID <= 0 || (reportType != ReportTypeMonthly && reportType != ReportTypeTeam) {
		return nil, ErrInvalidReport
	}
	now := time.Now()
	return &Report{PeriodID: periodID, Type: reportType, AuthorID: authorID, AuthorName: strings.TrimSpace(authorName), Status: ReportStatusDraft, Visibility: "private", CreatedAt: now, UpdatedAt: now}, nil
}

func (r *Report) CreateVersion(version int, title, body string, createdBy int64) (*ReportVersion, error) {
	if r == nil || version <= 0 || createdBy <= 0 || version != r.CurrentVersion+1 || r.Status == ReportStatusArchived {
		return nil, ErrInvalidVersion
	}
	now := time.Now()
	return &ReportVersion{ReportID: r.ID, Version: version, Status: VersionStatusDraft, Title: strings.TrimSpace(title), BodyMarkdown: body, ParseStatus: ParseStatusPending, CreatedBy: createdBy, CreatedAt: now}, nil
}

func (r *Report) MarkVersionCreated(version int) error {
	if r == nil || version != r.CurrentVersion+1 {
		return ErrInvalidVersion
	}
	r.CurrentVersion = version
	r.Status = ReportStatusDraft
	r.UpdatedAt = time.Now()
	return nil
}

func (v *ReportVersion) Submit(submittedBy int64) error {
	if v == nil || v.Status != VersionStatusDraft || submittedBy <= 0 || strings.TrimSpace(v.BodyMarkdown) == "" {
		return ErrInvalidVersionState
	}
	now := time.Now()
	v.Status = VersionStatusSubmitted
	v.SubmittedBy = submittedBy
	v.SubmittedAt = &now
	return nil
}

func (v *ReportVersion) Review(approved bool) error {
	if v == nil || v.Status != VersionStatusSubmitted {
		return ErrInvalidVersionState
	}
	if approved {
		v.Status = VersionStatusReviewed
	} else {
		v.Status = VersionStatusRejected
	}
	return nil
}

func (v *ReportVersion) Invalidate(reason string) error {
	if v == nil || v.Status == VersionStatusInvalidated || strings.TrimSpace(reason) == "" {
		return ErrInvalidVersionState
	}
	v.Status = VersionStatusInvalidated
	v.ParseStatus = ParseStatusFailed
	v.ParseError = strings.TrimSpace(reason)
	return nil
}

func (v *ReportVersion) AttachAsset(asset ReportAsset) error {
	if v == nil || v.Status != VersionStatusDraft || asset.KnowledgeItemID <= 0 || strings.TrimSpace(asset.AssetRole) == "" {
		return ErrInvalidVersion
	}
	for _, existing := range v.Assets {
		if existing.KnowledgeItemID == asset.KnowledgeItemID && existing.AssetRole == asset.AssetRole {
			return ErrDuplicateAsset
		}
	}
	asset.ReportVersionID = v.ID
	v.Assets = append(v.Assets, asset)
	return nil
}

func (p PeriodStatus) IsValid() bool {
	return p == PeriodStatusOpen || p == PeriodStatusClosed || p == PeriodStatusArchived
}

func (r ReportStatus) IsValid() bool {
	return r == ReportStatusDraft || r == ReportStatusSubmitted || r == ReportStatusReviewed || r == ReportStatusArchived
}

func (v VersionStatus) IsValid() bool {
	return v == VersionStatusDraft || v == VersionStatusSubmitted || v == VersionStatusReviewed || v == VersionStatusRejected || v == VersionStatusInvalidated
}

func (p *ReportPeriod) String() string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%s (%s)", p.PeriodKey, p.Status)
}
