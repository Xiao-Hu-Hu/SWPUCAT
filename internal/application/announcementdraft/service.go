package announcementdraftapp

import (
	announcementapp "SWPUCAT/internal/application/announcement"
	"SWPUCAT/internal/domain/announcementdraft"
	"SWPUCAT/internal/domain/shared"
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

type CreateRequest struct {
	Title          string                 `json:"title"`
	Content        string                 `json:"content"`
	Pinned         bool                   `json:"pinned"`
	Source         map[string]interface{} `json:"source,omitempty"`
	RecipientIDs   []int64                `json:"recipient_ids,omitempty"`
	EmailRequested bool                   `json:"email_requested"`
}

type UpdateRequest struct {
	Title          string  `json:"title"`
	Content        string  `json:"content"`
	Pinned         *bool   `json:"pinned,omitempty"`
	RecipientIDs   []int64 `json:"recipient_ids,omitempty"`
	EmailRequested *bool   `json:"email_requested,omitempty"`
}

type DraftDTO struct {
	ID              int64   `json:"id"`
	Status          string  `json:"status"`
	Title           string  `json:"title"`
	Content         string  `json:"content"`
	Pinned          bool    `json:"pinned"`
	SourceJSON      string  `json:"source_json,omitempty"`
	RecipientIDs    []int64 `json:"recipient_ids"`
	EmailRequested  bool    `json:"email_requested"`
	EmailResultJSON string  `json:"email_result_json,omitempty"`
	AnnouncementID  int64   `json:"announcement_id,omitempty"`
	CreatedBy       int64   `json:"created_by"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
	ConfirmedAt     string  `json:"confirmed_at,omitempty"`
	PublishedAt     string  `json:"published_at,omitempty"`
}

type Publisher interface {
	Create(ctx context.Context, operatorID int64, operatorName string, req announcementapp.CreateRequest) (*announcementapp.AnnouncementDTO, error)
	NotifyMembers(ctx context.Context, operatorID, annID int64, userIDs []int64) (*announcementapp.NotifyResult, error)
}

type Service struct {
	repo      announcementdraft.Repository
	publisher Publisher
}

func NewService(repo announcementdraft.Repository, publisher Publisher) *Service {
	return &Service{repo: repo, publisher: publisher}
}

func (s *Service) Create(ctx context.Context, actorID int64, req CreateRequest) (*DraftDTO, error) {
	source := req.Source
	if source == nil {
		source = map[string]interface{}{}
	}
	sourceJSON, err := json.Marshal(source)
	if err != nil {
		return nil, shared.ErrValidation
	}
	recipients, err := normalizeRecipients(req.RecipientIDs, req.EmailRequested)
	if err != nil {
		return nil, err
	}
	recipientJSON, err := json.Marshal(recipients)
	if err != nil {
		return nil, shared.ErrValidation
	}
	draft, err := announcementdraft.New(actorID, req.Title, req.Content, string(sourceJSON), string(recipientJSON), req.Pinned, req.EmailRequested)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, draft); err != nil {
		return nil, err
	}
	return toDTO(draft), nil
}

func (s *Service) Get(ctx context.Context, id int64) (*DraftDTO, error) {
	draft, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return toDTO(draft), nil
}

func (s *Service) List(ctx context.Context, status string) ([]DraftDTO, error) {
	status = strings.TrimSpace(status)
	if status != "" && status != string(announcementdraft.StatusDraft) && status != string(announcementdraft.StatusConfirmed) && status != string(announcementdraft.StatusPublished) && status != string(announcementdraft.StatusDiscarded) {
		return nil, shared.ErrValidation
	}
	drafts, err := s.repo.List(ctx, announcementdraft.Status(status))
	if err != nil {
		return nil, err
	}
	result := make([]DraftDTO, 0, len(drafts))
	for _, draft := range drafts {
		result = append(result, *toDTO(draft))
	}
	return result, nil
}

func (s *Service) Update(ctx context.Context, actorID, id int64, req UpdateRequest) (*DraftDTO, error) {
	draft, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if draft.Status != announcementdraft.StatusDraft {
		return nil, shared.ErrConflict
	}
	if strings.TrimSpace(req.Title) == "" || len([]rune(req.Title)) > 256 || strings.TrimSpace(req.Content) == "" || len([]rune(req.Content)) > 120000 {
		return nil, shared.ErrValidation
	}
	recipients := []int64{}
	if req.RecipientIDs != nil {
		recipients, err = normalizeRecipients(req.RecipientIDs, req.EmailRequestedValue(draft.EmailRequested))
		if err != nil {
			return nil, err
		}
	} else if req.EmailRequested != nil && *req.EmailRequested {
		recipients, err = parseRecipients(draft.RecipientIDsJSON)
		if err != nil || len(recipients) == 0 {
			return nil, shared.ErrValidation
		}
	}
	if req.RecipientIDs != nil {
		data, _ := json.Marshal(recipients)
		draft.RecipientIDsJSON = string(data)
	}
	if req.EmailRequested != nil {
		draft.EmailRequested = *req.EmailRequested
	}
	draft.Title, draft.Content, draft.UpdatedAt = strings.TrimSpace(req.Title), req.Content, time.Now()
	if req.Pinned != nil {
		draft.Pinned = *req.Pinned
	}
	if err := s.repo.Update(ctx, draft, actorID, announcementdraft.StatusDraft); err != nil {
		return nil, err
	}
	return toDTO(draft), nil
}

func (s *Service) Confirm(ctx context.Context, actorID, id int64) (*DraftDTO, error) {
	draft, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := draft.Confirm(); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, draft, actorID, announcementdraft.StatusDraft); err != nil {
		return nil, err
	}
	return toDTO(draft), nil
}

func (s *Service) Discard(ctx context.Context, actorID, id int64) error {
	draft, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	expected := draft.Status
	if err := draft.Discard(); err != nil {
		return err
	}
	return s.repo.Update(ctx, draft, actorID, expected)
}

func (s *Service) Publish(ctx context.Context, actorID int64, actorName string, id int64) (*DraftDTO, error) {
	draft, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if draft.Status != announcementdraft.StatusConfirmed || s.publisher == nil {
		return nil, shared.ErrConflict
	}
	recipients, err := parseRecipients(draft.RecipientIDsJSON)
	if err != nil {
		return nil, shared.ErrValidation
	}
	announcement, err := s.publisher.Create(ctx, actorID, actorName, announcementapp.CreateRequest{Title: draft.Title, Content: draft.Content, Pinned: draft.Pinned})
	if err != nil {
		return nil, err
	}
	emailResult := map[string]interface{}{"requested": draft.EmailRequested, "status": "not_requested"}
	if draft.EmailRequested {
		result, notifyErr := s.publisher.NotifyMembers(ctx, actorID, announcement.ID, recipients)
		if notifyErr != nil {
			emailResult["status"] = "failed"
			emailResult["error"] = notifyErr.Error()
		} else {
			if result != nil && len(result.Failed) > 0 {
				emailResult["status"] = "partial"
			} else {
				emailResult["status"] = "completed"
			}
			emailResult["result"] = result
		}
	}
	emailResultJSON, _ := json.Marshal(emailResult)
	if err := draft.MarkPublished(announcement.ID, string(emailResultJSON)); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, draft, actorID, announcementdraft.StatusConfirmed); err != nil {
		return nil, err
	}
	return toDTO(draft), nil
}

func normalizeRecipients(ids []int64, emailRequested bool) ([]int64, error) {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, shared.ErrValidation
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			result = append(result, id)
		}
	}
	if len(result) > 500 || emailRequested && len(result) == 0 {
		return nil, shared.ErrValidation
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

func parseRecipients(value string) ([]int64, error) {
	var ids []int64
	if err := json.Unmarshal([]byte(value), &ids); err != nil {
		return nil, err
	}
	return normalizeRecipients(ids, false)
}

func (r UpdateRequest) EmailRequestedValue(current bool) bool {
	if r.EmailRequested != nil {
		return *r.EmailRequested
	}
	return current
}

func toDTO(draft *announcementdraft.Draft) *DraftDTO {
	recipients, _ := parseRecipients(draft.RecipientIDsJSON)
	dto := &DraftDTO{ID: draft.ID, Status: string(draft.Status), Title: draft.Title, Content: draft.Content, Pinned: draft.Pinned, SourceJSON: draft.SourceJSON, RecipientIDs: recipients, EmailRequested: draft.EmailRequested, EmailResultJSON: draft.EmailResultJSON, AnnouncementID: draft.AnnouncementID, CreatedBy: draft.CreatedBy, CreatedAt: draft.CreatedAt.Format(time.RFC3339), UpdatedAt: draft.UpdatedAt.Format(time.RFC3339)}
	if draft.ConfirmedAt != nil {
		dto.ConfirmedAt = draft.ConfirmedAt.Format(time.RFC3339)
	}
	if draft.PublishedAt != nil {
		dto.PublishedAt = draft.PublishedAt.Format(time.RFC3339)
	}
	return dto
}
