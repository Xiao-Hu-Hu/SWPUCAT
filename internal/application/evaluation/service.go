package evaluationapp

import (
	domain "SWPUCAT/internal/domain/evaluation"
	"SWPUCAT/internal/domain/shared"
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"
)

const maxSummaryRunes = 120000

type CreateDraftRequest struct {
	SnapshotID      int64                `json:"snapshot_id"`
	Title           string               `json:"title"`
	SummaryMarkdown string               `json:"summary_markdown"`
	Audience        domain.DraftAudience `json:"audience"`
	Model           string               `json:"model,omitempty"`
	PromptVersion   string               `json:"prompt_version,omitempty"`
}

type UpdateDraftRequest struct {
	Title           string `json:"title"`
	SummaryMarkdown string `json:"summary_markdown"`
}

type MemberCommentRequest struct {
	CommentMarkdown string `json:"comment_markdown"`
	Recommendation  string `json:"recommendation,omitempty"`
}

type DraftDTO struct {
	ID              int64       `json:"id"`
	SnapshotID      int64       `json:"snapshot_id"`
	PeriodID        int64       `json:"period_id"`
	Status          string      `json:"status"`
	Audience        string      `json:"audience"`
	Title           string      `json:"title"`
	SummaryMarkdown string      `json:"summary_markdown"`
	Model           string      `json:"model,omitempty"`
	PromptVersion   string      `json:"prompt_version,omitempty"`
	CreatedBy       int64       `json:"created_by"`
	CreatedAt       string      `json:"created_at"`
	UpdatedAt       string      `json:"updated_at"`
	PublishedAt     string      `json:"published_at,omitempty"`
	Members         []MemberDTO `json:"members,omitempty"`
}

type MemberDTO struct {
	UserID          int64  `json:"user_id"`
	DisplayName     string `json:"display_name"`
	CommentMarkdown string `json:"comment_markdown"`
	Recommendation  string `json:"recommendation,omitempty"`
	EvidenceJSON    string `json:"evidence_json,omitempty"`
	ManualOverride  bool   `json:"manual_override"`
}

type Service struct{ repo domain.Repository }

func NewService(repo domain.Repository) *Service { return &Service{repo: repo} }

func (s *Service) CreateDraft(ctx context.Context, actorID int64, req CreateDraftRequest) (*DraftDTO, error) {
	snapshot, err := s.repo.FindSnapshotByID(ctx, req.SnapshotID)
	if err != nil {
		return nil, err
	}
	if snapshot.Status == domain.SnapshotInvalidated {
		return nil, shared.ErrConflict
	}
	if !utf8.ValidString(req.SummaryMarkdown) || utf8.RuneCountInString(req.SummaryMarkdown) > maxSummaryRunes {
		return nil, shared.ErrValidation
	}
	if req.Audience == "" {
		req.Audience = domain.DraftAudienceCaptain
	}
	draft, err := domain.NewDraft(snapshot.ID, snapshot.PeriodID, actorID, req.Audience, req.Title, req.SummaryMarkdown, req.Model, req.PromptVersion)
	if err != nil {
		return nil, err
	}
	for _, member := range snapshot.Members {
		evidence, marshalErr := json.Marshal(struct {
			TotalMinutes    float64             `json:"total_minutes"`
			TotalHours      float64             `json:"total_hours"`
			AttendanceDays  int                 `json:"attendance_days"`
			AchievedWeeks   int                 `json:"achieved_weeks"`
			Anomalies       []string            `json:"anomalies"`
			ReportStatus    domain.ReportStatus `json:"report_status"`
			ReportVersionID int64               `json:"report_version_id,omitempty"`
			DataState       domain.DataState    `json:"data_state"`
			UnknownReason   string              `json:"unknown_reason,omitempty"`
		}{member.TotalMinutes, member.TotalHours, member.AttendanceDays, member.AchievedWeeks, member.Anomalies, member.ReportStatus, member.ReportVersionID, member.DataState, member.UnknownReason})
		if marshalErr != nil {
			return nil, marshalErr
		}
		draft.Members = append(draft.Members, domain.DraftMember{UserID: member.UserID, DisplayName: member.DisplayName, EvidenceJSON: string(evidence)})
	}
	if err := s.repo.CreateDraft(ctx, draft); err != nil {
		return nil, err
	}
	return toDraftDTO(draft), nil
}

func (s *Service) GetDraft(ctx context.Context, id int64) (*DraftDTO, error) {
	draft, err := s.repo.FindDraftByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return toDraftDTO(draft), nil
}

func (s *Service) UpdateDraft(ctx context.Context, id, actorID int64, req UpdateDraftRequest) (*DraftDTO, error) {
	draft, err := s.repo.FindDraftByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.ensureDraftSnapshotActive(ctx, draft, actorID); err != nil {
		return nil, err
	}
	if draft.Status != domain.DraftStatusDraft {
		return nil, shared.ErrConflict
	}
	if !utf8.ValidString(req.SummaryMarkdown) || utf8.RuneCountInString(req.SummaryMarkdown) > maxSummaryRunes {
		return nil, shared.ErrValidation
	}
	expectedStatus := draft.Status
	draft.Title, draft.SummaryMarkdown, draft.UpdatedAt = strings.TrimSpace(req.Title), req.SummaryMarkdown, time.Now()
	if err := s.repo.UpdateDraft(ctx, draft, actorID, expectedStatus); err != nil {
		return nil, err
	}
	return toDraftDTO(draft), nil
}

func (s *Service) ReviewDraft(ctx context.Context, id, actorID int64, approved bool) (*DraftDTO, error) {
	draft, err := s.repo.FindDraftByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.ensureDraftSnapshotActive(ctx, draft, actorID); err != nil {
		return nil, err
	}
	expectedStatus := draft.Status
	if err := draft.Review(approved); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateDraft(ctx, draft, actorID, expectedStatus); err != nil {
		return nil, err
	}
	return toDraftDTO(draft), nil
}

func (s *Service) PublishDraft(ctx context.Context, id, actorID int64) (*DraftDTO, error) {
	draft, err := s.repo.FindDraftByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.ensureDraftSnapshotActive(ctx, draft, actorID); err != nil {
		return nil, err
	}
	expectedStatus := draft.Status
	if err := draft.Publish(); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateDraft(ctx, draft, actorID, expectedStatus); err != nil {
		return nil, err
	}
	return toDraftDTO(draft), nil
}

func (s *Service) SaveMemberComment(ctx context.Context, draftID, actorID, userID int64, req MemberCommentRequest) error {
	draft, err := s.repo.FindDraftByID(ctx, draftID)
	if err != nil {
		return err
	}
	if err := s.ensureDraftSnapshotActive(ctx, draft, actorID); err != nil {
		return err
	}
	if draft.Status != domain.DraftStatusDraft {
		return shared.ErrConflict
	}
	return s.repo.SaveMemberComment(ctx, &domain.DraftMember{DraftID: draftID, UserID: userID, CommentMarkdown: req.CommentMarkdown, Recommendation: req.Recommendation}, actorID)
}

func (s *Service) ensureDraftSnapshotActive(ctx context.Context, draft *domain.Draft, actorID int64) error {
	snapshot, err := s.repo.FindSnapshotByID(ctx, draft.SnapshotID)
	if err != nil {
		return err
	}
	if snapshot.Status != domain.SnapshotInvalidated {
		return nil
	}
	expectedStatus := draft.Status
	if err := draft.Invalidate(); err != nil {
		return shared.ErrConflict
	}
	if err := s.repo.UpdateDraft(ctx, draft, actorID, expectedStatus); err != nil {
		return err
	}
	return shared.ErrConflict
}

func toDraftDTO(item *domain.Draft) *DraftDTO {
	dto := &DraftDTO{ID: item.ID, SnapshotID: item.SnapshotID, PeriodID: item.PeriodID, Status: string(item.Status), Audience: string(item.Audience), Title: item.Title, SummaryMarkdown: item.SummaryMarkdown, Model: item.Model, PromptVersion: item.PromptVersion, CreatedBy: item.CreatedBy, CreatedAt: item.CreatedAt.Format(time.RFC3339), UpdatedAt: item.UpdatedAt.Format(time.RFC3339), Members: make([]MemberDTO, 0, len(item.Members))}
	if item.PublishedAt != nil {
		dto.PublishedAt = item.PublishedAt.Format(time.RFC3339)
	}
	for _, member := range item.Members {
		dto.Members = append(dto.Members, MemberDTO{UserID: member.UserID, DisplayName: member.DisplayName, CommentMarkdown: member.CommentMarkdown, Recommendation: member.Recommendation, EvidenceJSON: member.EvidenceJSON, ManualOverride: member.ManualOverride})
	}
	return dto
}
