package reportapp

import (
	"SWPUCAT/internal/application/reportcheck"
	"SWPUCAT/internal/application/reportparse"
	domain "SWPUCAT/internal/domain/report"
	"SWPUCAT/internal/domain/shared"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"
)

const maxReportBodyRunes = reportparse.MaxBodyRunes

type CreatePeriodRequest struct {
	PeriodKey string     `json:"period_key"`
	StartsOn  string     `json:"starts_on"`
	EndsOn    string     `json:"ends_on"`
	Timezone  string     `json:"timezone"`
	DueAt     *time.Time `json:"due_at,omitempty"`
}

type AddMemberRequest struct {
	UserID          int64      `json:"user_id"`
	DisplayName     string     `json:"display_name"`
	Required        bool       `json:"required"`
	Exempt          bool       `json:"exempt"`
	ExemptionReason string     `json:"exemption_reason,omitempty"`
	EffectiveFrom   *time.Time `json:"effective_from,omitempty"`
	EffectiveUntil  *time.Time `json:"effective_until,omitempty"`
}

type CreateReportRequest struct {
	PeriodID   int64             `json:"period_id"`
	ReportType domain.ReportType `json:"report_type"`
}

type CreateVersionRequest struct {
	ExpectedVersion int    `json:"expected_version"`
	Title           string `json:"title"`
	BodyMarkdown    string `json:"body_markdown"`
}

type AttachAssetRequest struct {
	KnowledgeItemID int64  `json:"knowledge_item_id"`
	AssetRole       string `json:"asset_role"`
	Locator         string `json:"locator,omitempty"`
	Note            string `json:"note,omitempty"`
}

type PeriodDTO struct {
	ID            int64  `json:"id"`
	PeriodKey     string `json:"period_key"`
	StartsOn      string `json:"starts_on"`
	EndsOn        string `json:"ends_on"`
	Timezone      string `json:"timezone"`
	DueAt         string `json:"due_at,omitempty"`
	Status        string `json:"status"`
	RosterVersion int64  `json:"roster_version"`
	MemberCount   int    `json:"member_count"`
}

type MemberDTO struct {
	UserID          int64  `json:"user_id"`
	DisplayName     string `json:"display_name"`
	Required        bool   `json:"required"`
	Exempt          bool   `json:"exempt"`
	ExemptionReason string `json:"exemption_reason,omitempty"`
}

type ReportDTO struct {
	ID             int64  `json:"id"`
	PeriodID       int64  `json:"period_id"`
	ReportType     string `json:"report_type"`
	AuthorID       int64  `json:"author_id"`
	AuthorName     string `json:"author_name"`
	Status         string `json:"status"`
	Visibility     string `json:"visibility"`
	CurrentVersion int    `json:"current_version"`
	LateSubmission bool   `json:"late_submission"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type VersionDTO struct {
	ID               int64       `json:"id"`
	ReportID         int64       `json:"report_id"`
	Version          int         `json:"version"`
	Status           string      `json:"status"`
	Title            string      `json:"title"`
	BodyMarkdown     string      `json:"body_markdown"`
	ContentHash      string      `json:"content_hash"`
	ParseStatus      string      `json:"parse_status"`
	ParseError       string      `json:"parse_error,omitempty"`
	ParserVersion    string      `json:"parser_version,omitempty"`
	ParsedJSON       interface{} `json:"parsed_json,omitempty"`
	QualityStatus    string      `json:"quality_status,omitempty"`
	QualityJSON      interface{} `json:"quality_json,omitempty"`
	QualityCheckedAt string      `json:"quality_checked_at,omitempty"`
	SubmittedAt      string      `json:"submitted_at,omitempty"`
	CreatedAt        string      `json:"created_at"`
	Assets           []AssetDTO  `json:"assets,omitempty"`
}

type AssetDTO struct {
	ReportVersionID int64  `json:"report_version_id"`
	KnowledgeItemID int64  `json:"knowledge_item_id"`
	AssetRole       string `json:"asset_role"`
	Locator         string `json:"locator,omitempty"`
	Note            string `json:"note,omitempty"`
}

type RosterEntryDTO struct {
	UserID          int64  `json:"user_id"`
	DisplayName     string `json:"display_name"`
	Required        bool   `json:"required"`
	Exempt          bool   `json:"exempt"`
	ExemptionReason string `json:"exemption_reason,omitempty"`
	ReportID        int64  `json:"report_id,omitempty"`
	ReportStatus    string `json:"report_status,omitempty"`
	SubmittedAt     string `json:"submitted_at,omitempty"`
}

type Service struct {
	repo domain.Repository
}

func NewService(repo domain.Repository) *Service { return &Service{repo: repo} }

func (s *Service) CreatePeriod(ctx context.Context, actorID int64, actorName string, req CreatePeriodRequest) (*PeriodDTO, error) {
	if strings.TrimSpace(req.Timezone) == "" {
		req.Timezone = "Asia/Shanghai"
	}
	start, err := time.Parse("2006-01-02", strings.TrimSpace(req.StartsOn))
	if err != nil {
		return nil, domain.ErrInvalidPeriod
	}
	end, err := time.Parse("2006-01-02", strings.TrimSpace(req.EndsOn))
	if err != nil {
		return nil, domain.ErrInvalidPeriod
	}
	// A submission deadline is mandatory so every period hard-closes on time;
	// otherwise the "已截止" state can never be derived or enforced.
	if req.DueAt == nil {
		return nil, domain.ErrInvalidPeriod
	}
	if req.DueAt.Before(end) {
		return nil, domain.ErrInvalidPeriod
	}
	period, err := domain.NewReportPeriod(req.PeriodKey, start, end, req.Timezone, req.DueAt, actorID, actorName)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreatePeriod(ctx, period); err != nil {
		return nil, err
	}
	return toPeriodDTO(period), nil
}

func (s *Service) ListPeriods(ctx context.Context) ([]PeriodDTO, error) {
	periods, err := s.repo.ListPeriods(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]PeriodDTO, 0, len(periods))
	for _, period := range periods {
		result = append(result, *toPeriodDTO(period))
	}
	return result, nil
}

func (s *Service) ListRoster(ctx context.Context, periodID int64) ([]RosterEntryDTO, error) {
	entries, err := s.repo.ListRoster(ctx, periodID)
	if err != nil {
		return nil, err
	}
	result := make([]RosterEntryDTO, 0, len(entries))
	for _, entry := range entries {
		dto := RosterEntryDTO{
			UserID: entry.UserID, DisplayName: entry.DisplayName, Required: entry.Required,
			Exempt: entry.Exempt, ExemptionReason: entry.ExemptionReason,
			ReportID: entry.ReportID, ReportStatus: string(entry.ReportStatus),
		}
		if entry.SubmittedAt != nil {
			dto.SubmittedAt = entry.SubmittedAt.Format(time.RFC3339)
		}
		result = append(result, dto)
	}
	return result, nil
}

func (s *Service) AddPeriodMember(ctx context.Context, periodID int64, req AddMemberRequest) error {
	period, err := s.repo.FindPeriodByID(ctx, periodID)
	if err != nil {
		return err
	}
	member := domain.PeriodMember{PeriodID: period.ID, UserID: req.UserID, DisplayName: strings.TrimSpace(req.DisplayName), Required: req.Required, Exempt: req.Exempt, ExemptionReason: strings.TrimSpace(req.ExemptionReason), EffectiveFrom: req.EffectiveFrom, EffectiveUntil: req.EffectiveUntil}
	if err := period.AddMember(member); err != nil {
		return err
	}
	if err := s.repo.AddPeriodMember(ctx, &member); err != nil {
		return err
	}
	return nil
}

// IsPeriodMember reports whether the user is on the period's submission roster
// (required and not exempt), which lets the UI gate the "create report" action
// ahead of the backend check.
func (s *Service) IsPeriodMember(ctx context.Context, periodID, userID int64) (bool, error) {
	period, err := s.repo.FindPeriodByID(ctx, periodID)
	if err != nil {
		return false, err
	}
	return isSubmissionRosterMember(period, userID), nil
}

func (s *Service) CreateReport(ctx context.Context, authorID int64, authorName string, req CreateReportRequest) (*ReportDTO, error) {
	period, err := s.repo.FindPeriodByID(ctx, req.PeriodID)
	if err != nil {
		return nil, err
	}
	if req.ReportType == domain.ReportTypeMonthly {
		if !period.AcceptsSubmissions(time.Now()) {
			return nil, domain.ErrPeriodClosed
		}
		if hasSubmissionRoster(period) && !isSubmissionRosterMember(period, authorID) {
			return nil, domain.ErrNotInRoster
		}
	}
	item, err := domain.NewReport(req.PeriodID, authorID, req.ReportType, authorName)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateReport(ctx, item); err != nil {
		return nil, err
	}
	return toReportDTO(item), nil
}

func (s *Service) ListReports(ctx context.Context, viewerID int64, manager bool, periodID *int64, status domain.ReportStatus, page, pageSize int) ([]ReportDTO, error) {
	reports, err := s.repo.ListReports(ctx, viewerID, manager, periodID, status, page, pageSize)
	if err != nil {
		return nil, err
	}
	result := make([]ReportDTO, 0, len(reports))
	for _, item := range reports {
		result = append(result, *toReportDTO(item))
	}
	return result, nil
}

func (s *Service) GetReport(ctx context.Context, id, viewerID int64, manager bool) (*ReportDTO, error) {
	item, err := s.repo.FindReportByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !canAccess(item, viewerID, manager) {
		return nil, shared.ErrForbidden
	}
	return toReportDTO(item), nil
}

func (s *Service) CreateVersion(ctx context.Context, reportID, actorID int64, manager bool, req CreateVersionRequest) (*VersionDTO, error) {
	parent, err := s.repo.FindReportByID(ctx, reportID)
	if err != nil {
		return nil, err
	}
	if !canAccess(parent, actorID, manager) {
		return nil, shared.ErrForbidden
	}
	period, err := s.repo.FindPeriodByID(ctx, parent.PeriodID)
	if err != nil {
		return nil, err
	}
	if !period.AcceptsSubmissions(time.Now()) {
		return nil, domain.ErrPeriodClosed
	}
	if req.ExpectedVersion < 0 || req.ExpectedVersion != parent.CurrentVersion || !utf8.ValidString(req.BodyMarkdown) || utf8.RuneCountInString(req.BodyMarkdown) > maxReportBodyRunes {
		return nil, shared.ErrConflict
	}
	version, err := parent.CreateVersion(parent.CurrentVersion+1, req.Title, req.BodyMarkdown, actorID)
	if err != nil {
		return nil, err
	}
	parsed := reportparse.Parse(req.BodyMarkdown)
	version.ParserVersion = reportparse.ParserVersion
	version.ParsedAt = timePtr(time.Now())
	if parsed.Status == reportparse.ParseReady {
		version.ParseStatus = domain.ParseStatusReady
		data, marshalErr := json.Marshal(parsed.Report)
		if marshalErr != nil {
			return nil, marshalErr
		}
		version.ParsedJSON = string(data)
	} else {
		version.ParseStatus = domain.ParseStatusFailed
		version.ParseError = parsed.Error
		version.ParsedJSON = "{}"
	}
	digest := sha256.Sum256([]byte(req.BodyMarkdown))
	version.ContentHash = hex.EncodeToString(digest[:])
	if err := s.repo.CreateVersion(ctx, version, parent.CurrentVersion); err != nil {
		return nil, err
	}
	return toVersionDTO(version), nil
}

func (s *Service) GetVersion(ctx context.Context, reportID int64, version int, viewerID int64, manager bool) (*VersionDTO, error) {
	parent, err := s.repo.FindReportByID(ctx, reportID)
	if err != nil {
		return nil, err
	}
	if !canAccess(parent, viewerID, manager) {
		return nil, shared.ErrForbidden
	}
	item, err := s.repo.FindVersion(ctx, reportID, version)
	if err != nil {
		return nil, err
	}
	return toVersionDTO(item), nil
}

func (s *Service) GetSubmittedVersion(ctx context.Context, reportID int64, viewerID int64, manager bool) (*VersionDTO, error) {
	parent, err := s.repo.FindReportByID(ctx, reportID)
	if err != nil {
		return nil, err
	}
	if !canAccess(parent, viewerID, manager) {
		return nil, shared.ErrForbidden
	}
	item, err := s.repo.FindSubmittedVersion(ctx, reportID)
	if err != nil {
		return nil, err
	}
	return toVersionDTO(item), nil
}

// WithdrawVersion lets the author cancel their own submission, returning the
// report to draft so it can be edited and resubmitted.
func (s *Service) WithdrawVersion(ctx context.Context, reportID int64, _ int, actorID int64) (*VersionDTO, error) {
	parent, err := s.repo.FindReportByID(ctx, reportID)
	if err != nil {
		return nil, err
	}
	if parent.AuthorID != actorID {
		return nil, shared.ErrForbidden
	}
	if parent.Status != domain.ReportStatusSubmitted {
		return nil, domain.ErrInvalidVersionState
	}
	version, err := s.repo.FindSubmittedVersion(ctx, reportID)
	if err != nil {
		return nil, err
	}
	if err := version.Withdraw(); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateVersion(ctx, version); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateReportStatus(ctx, reportID, domain.ReportStatusDraft); err != nil {
		return nil, err
	}
	return toVersionDTO(version), nil
}

func (s *Service) SubmitVersion(ctx context.Context, reportID int64, versionNumber int, actorID int64, manager bool) (*VersionDTO, error) {
	parent, err := s.repo.FindReportByID(ctx, reportID)
	if err != nil {
		return nil, err
	}
	if !canAccess(parent, actorID, manager) {
		return nil, shared.ErrForbidden
	}
	period, err := s.repo.FindPeriodByID(ctx, parent.PeriodID)
	if err != nil {
		return nil, err
	}
	if !period.AcceptsSubmissions(time.Now()) {
		return nil, domain.ErrPeriodClosed
	}
	version, err := s.repo.FindVersion(ctx, reportID, versionNumber)
	if err != nil {
		return nil, err
	}
	if err := version.Submit(actorID); err != nil {
		return nil, err
	}
	if version.SubmittedAt != nil && version.SubmittedAt.After(period.EndsOn) {
		parent.LateSubmission = true
		if err := s.repo.UpdateReportLateSubmission(ctx, reportID, true); err != nil {
			return nil, err
		}
	}
	if err := s.repo.UpdateVersion(ctx, version); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateReportStatus(ctx, reportID, domain.ReportStatusSubmitted); err != nil {
		return nil, err
	}
	return toVersionDTO(version), nil
}

func (s *Service) ReviewVersion(ctx context.Context, reportID int64, _ int, approved bool, manager bool) (*VersionDTO, error) {
	if !manager {
		return nil, shared.ErrForbidden
	}
	parent, err := s.repo.FindReportByID(ctx, reportID)
	if err != nil {
		return nil, err
	}
	if parent.Status != domain.ReportStatusSubmitted {
		return nil, domain.ErrInvalidVersionState
	}
	// Review always targets the submitted version, not whatever current_version
	// points at, so a stray post-submit draft can never cause a state conflict.
	version, err := s.repo.FindSubmittedVersion(ctx, reportID)
	if err != nil {
		return nil, err
	}
	if err := version.Review(approved); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateVersion(ctx, version); err != nil {
		return nil, err
	}
	status := domain.ReportStatusReviewed
	if !approved {
		status = domain.ReportStatusDraft
	}
	if err := s.repo.UpdateReportStatus(ctx, reportID, status); err != nil {
		return nil, err
	}
	return toVersionDTO(version), nil
}

func (s *Service) AttachAsset(ctx context.Context, reportID int64, versionNumber int, actorID int64, manager bool, req AttachAssetRequest) (*AssetDTO, error) {
	parent, err := s.repo.FindReportByID(ctx, reportID)
	if err != nil {
		return nil, err
	}
	if !canAccess(parent, actorID, manager) {
		return nil, shared.ErrForbidden
	}
	version, err := s.repo.FindVersion(ctx, reportID, versionNumber)
	if err != nil {
		return nil, err
	}
	asset := &domain.ReportAsset{ReportVersionID: version.ID, KnowledgeItemID: req.KnowledgeItemID, AssetRole: strings.TrimSpace(req.AssetRole), Locator: strings.TrimSpace(req.Locator), Note: strings.TrimSpace(req.Note)}
	if err := version.AttachAsset(*asset); err != nil {
		return nil, err
	}
	if err := s.repo.AttachAsset(ctx, asset); err != nil {
		return nil, err
	}
	return toAssetDTO(asset), nil
}

func (s *Service) CheckQuality(ctx context.Context, reportID int64, versionNumber int, viewerID int64, manager bool) (*VersionDTO, error) {
	parent, err := s.repo.FindReportByID(ctx, reportID)
	if err != nil {
		return nil, err
	}
	if !canAccess(parent, viewerID, manager) {
		return nil, shared.ErrForbidden
	}
	version, err := s.repo.FindVersion(ctx, reportID, versionNumber)
	if err != nil {
		return nil, err
	}
	parsed := reportparse.Parse(version.BodyMarkdown)
	quality := reportcheck.Check(parsed.Report)
	data, err := json.Marshal(quality)
	if err != nil {
		return nil, err
	}
	status := "ready"
	if parsed.Status == reportparse.ParseFailed {
		status = "failed"
	}
	now := time.Now()
	if err := s.repo.UpdateVersionQuality(ctx, version.ID, status, string(data), &now); err != nil {
		return nil, err
	}
	version.QualityStatus, version.QualityJSON, version.QualityCheckedAt = status, string(data), &now
	return toVersionDTO(version), nil
}

func (s *Service) DecideQuality(ctx context.Context, reportID int64, versionNumber, findingIndex int, viewerID int64, manager, accepted bool, note string) error {
	parent, err := s.repo.FindReportByID(ctx, reportID)
	if err != nil {
		return err
	}
	if !canAccess(parent, viewerID, manager) {
		return shared.ErrForbidden
	}
	version, err := s.repo.FindVersion(ctx, reportID, versionNumber)
	if err != nil {
		return err
	}
	if findingIndex < 0 || findingIndex >= 1000 {
		return shared.ErrValidation
	}
	decision := "rejected"
	if accepted {
		decision = "accepted"
	}
	return s.repo.SaveQualityDecision(ctx, version.ID, findingIndex, decision, strings.TrimSpace(note), viewerID)
}

func canAccess(item *domain.Report, viewerID int64, manager bool) bool {
	return item != nil && (manager || item.AuthorID == viewerID)
}

func hasSubmissionRoster(period *domain.ReportPeriod) bool {
	if period == nil {
		return false
	}
	for _, member := range period.Members {
		if member.Required && !member.Exempt {
			return true
		}
	}
	return false
}

func isSubmissionRosterMember(period *domain.ReportPeriod, userID int64) bool {
	if period == nil {
		return false
	}
	for _, member := range period.Members {
		if member.UserID == userID && member.Required && !member.Exempt {
			return true
		}
	}
	return false
}

func toPeriodDTO(item *domain.ReportPeriod) *PeriodDTO {
	if item == nil {
		return nil
	}
	dto := &PeriodDTO{ID: item.ID, PeriodKey: item.PeriodKey, StartsOn: item.StartsOn.Format("2006-01-02"), EndsOn: item.EndsOn.Format("2006-01-02"), Timezone: item.Timezone, Status: string(item.Status), RosterVersion: item.RosterVersion, MemberCount: len(item.Members)}
	if item.DueAt != nil {
		dto.DueAt = item.DueAt.Format(time.RFC3339)
	}
	return dto
}

func toReportDTO(item *domain.Report) *ReportDTO {
	return &ReportDTO{ID: item.ID, PeriodID: item.PeriodID, ReportType: string(item.Type), AuthorID: item.AuthorID, AuthorName: item.AuthorName, Status: string(item.Status), Visibility: item.Visibility, CurrentVersion: item.CurrentVersion, LateSubmission: item.LateSubmission, CreatedAt: item.CreatedAt.Format(time.RFC3339), UpdatedAt: item.UpdatedAt.Format(time.RFC3339)}
}

func toVersionDTO(item *domain.ReportVersion) *VersionDTO {
	dto := &VersionDTO{ID: item.ID, ReportID: item.ReportID, Version: item.Version, Status: string(item.Status), Title: item.Title, BodyMarkdown: item.BodyMarkdown, ContentHash: item.ContentHash, ParseStatus: string(item.ParseStatus), ParseError: item.ParseError, ParserVersion: item.ParserVersion, QualityStatus: item.QualityStatus, CreatedAt: item.CreatedAt.Format(time.RFC3339), Assets: make([]AssetDTO, 0, len(item.Assets))}
	if item.ParsedJSON != "" {
		var parsed interface{}
		if json.Unmarshal([]byte(item.ParsedJSON), &parsed) == nil {
			dto.ParsedJSON = parsed
		}
	}
	if item.SubmittedAt != nil {
		dto.SubmittedAt = item.SubmittedAt.Format(time.RFC3339)
	}
	if item.QualityJSON != "" {
		var quality interface{}
		if json.Unmarshal([]byte(item.QualityJSON), &quality) == nil {
			dto.QualityJSON = quality
		}
	}
	if item.QualityCheckedAt != nil {
		dto.QualityCheckedAt = item.QualityCheckedAt.Format(time.RFC3339)
	}
	for _, asset := range item.Assets {
		dto.Assets = append(dto.Assets, *toAssetDTO(&asset))
	}
	return dto
}

func toAssetDTO(item *domain.ReportAsset) *AssetDTO {
	return &AssetDTO{ReportVersionID: item.ReportVersionID, KnowledgeItemID: item.KnowledgeItemID, AssetRole: item.AssetRole, Locator: item.Locator, Note: item.Note}
}

func timePtr(value time.Time) *time.Time { return &value }
