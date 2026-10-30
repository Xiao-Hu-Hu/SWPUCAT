package notificationapp

import (
	evaluation "SWPUCAT/internal/domain/evaluation"
	"SWPUCAT/internal/domain/notification"
	reportdomain "SWPUCAT/internal/domain/report"
	"SWPUCAT/internal/domain/shared"
	"SWPUCAT/internal/domain/user"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type CreateRequest struct {
	RecipientID  int64                  `json:"recipient_id"`
	Type         notification.Type      `json:"type"`
	Title        string                 `json:"title"`
	BodyMarkdown string                 `json:"body_markdown"`
	EntityType   string                 `json:"entity_type,omitempty"`
	EntityID     int64                  `json:"entity_id,omitempty"`
	DedupeKey    string                 `json:"dedupe_key"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	ExpiresAt    *time.Time             `json:"expires_at,omitempty"`
}

type NotificationDTO struct {
	ID           int64  `json:"id"`
	Type         string `json:"type"`
	Title        string `json:"title"`
	BodyMarkdown string `json:"body_markdown"`
	EntityType   string `json:"entity_type,omitempty"`
	EntityID     int64  `json:"entity_id,omitempty"`
	MetadataJSON string `json:"metadata_json,omitempty"`
	ReadAt       string `json:"read_at,omitempty"`
	CreatedAt    string `json:"created_at"`
	ExpiresAt    string `json:"expires_at,omitempty"`
}

type ListResult struct {
	Items       []NotificationDTO `json:"items"`
	Page        int               `json:"page"`
	PageSize    int               `json:"page_size"`
	UnreadCount int64             `json:"unread_count"`
}

type UpdatePreferencesRequest struct {
	CheckinAnomalyEnabled      *bool `json:"checkin_anomaly_enabled,omitempty"`
	ReportDueEnabled           *bool `json:"report_due_enabled,omitempty"`
	EvaluationPublishedEnabled *bool `json:"evaluation_published_enabled,omitempty"`
}

type PreferencesDTO struct {
	UserID                     int64  `json:"user_id"`
	CheckinAnomalyEnabled      bool   `json:"checkin_anomaly_enabled"`
	ReportDueEnabled           bool   `json:"report_due_enabled"`
	EvaluationPublishedEnabled bool   `json:"evaluation_published_enabled"`
	UpdatedAt                  string `json:"updated_at,omitempty"`
}

type Service struct {
	repo       notification.Repository
	userRepo   user.Repository
	periodRepo ReportPeriodReader
}

type ReportPeriodReader interface {
	ListDuePeriods(ctx context.Context, from, until time.Time) ([]*reportdomain.ReportPeriod, error)
	ListPendingReportMemberIDs(ctx context.Context, periodID int64) ([]int64, error)
}

func NewService(repo notification.Repository, userRepo user.Repository, periodRepos ...ReportPeriodReader) *Service {
	service := &Service{repo: repo, userRepo: userRepo}
	if len(periodRepos) > 0 {
		service.periodRepo = periodRepos[0]
	}
	return service
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (*NotificationDTO, error) {
	metadata := req.Metadata
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, shared.ErrValidation
	}
	item, err := notification.New(req.RecipientID, req.Type, req.Title, req.BodyMarkdown, req.EntityType, req.EntityID, req.DedupeKey, string(metadataJSON), req.ExpiresAt)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, item); err != nil {
		return nil, err
	}
	return toDTO(item), nil
}

func (s *Service) List(ctx context.Context, userID int64, unreadOnly bool, page, pageSize int) (*ListResult, error) {
	items, err := s.repo.ListForUser(ctx, userID, unreadOnly, page, pageSize)
	if err != nil {
		return nil, err
	}
	count, err := s.repo.CountUnread(ctx, userID)
	if err != nil {
		return nil, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	result := &ListResult{Items: make([]NotificationDTO, 0, len(items)), Page: page, PageSize: pageSize, UnreadCount: count}
	for _, item := range items {
		result.Items = append(result.Items, *toDTO(item))
	}
	return result, nil
}

func (s *Service) MarkRead(ctx context.Context, userID, notificationID int64) error {
	return s.repo.MarkRead(ctx, userID, notificationID)
}

func (s *Service) GetPreferences(ctx context.Context, userID int64) (*PreferencesDTO, error) {
	item, err := s.repo.GetPreferences(ctx, userID)
	if err != nil {
		return nil, err
	}
	return preferencesDTO(item), nil
}

func (s *Service) UpdatePreferences(ctx context.Context, userID int64, req UpdatePreferencesRequest) (*PreferencesDTO, error) {
	item, err := s.repo.GetPreferences(ctx, userID)
	if err != nil {
		return nil, err
	}
	item.UserID = userID
	if req.CheckinAnomalyEnabled != nil {
		item.CheckinAnomalyEnabled = *req.CheckinAnomalyEnabled
	}
	if req.ReportDueEnabled != nil {
		item.ReportDueEnabled = *req.ReportDueEnabled
	}
	if req.EvaluationPublishedEnabled != nil {
		item.EvaluationPublishedEnabled = *req.EvaluationPublishedEnabled
	}
	if err := s.repo.SavePreferences(ctx, item); err != nil {
		return nil, err
	}
	return preferencesDTO(item), nil
}

// NotifyCheckinAnomaly is an idempotent hook for the check-in statistics job.
// scopeKey should identify the exact day or period being evaluated.
func (s *Service) NotifyCheckinAnomaly(ctx context.Context, recipientID int64, scopeKey string, anomalies []string) error {
	prefs, err := s.repo.GetPreferences(ctx, recipientID)
	if err != nil {
		return err
	}
	if !prefs.CheckinAnomalyEnabled || len(anomalies) == 0 {
		return nil
	}
	anomalies = uniqueSorted(anomalies)
	return s.createInternal(ctx, CreateRequest{RecipientID: recipientID, Type: notification.TypeCheckinAnomaly, Title: "打卡异常提醒", BodyMarkdown: fmt.Sprintf("在 **%s** 检测到打卡异常：%s。请核对打卡记录或联系队长。", strings.TrimSpace(scopeKey), strings.Join(anomalies, "、")), EntityType: "checkin", DedupeKey: fmt.Sprintf("checkin-anomaly:%d:%s", recipientID, strings.TrimSpace(scopeKey)), Metadata: map[string]interface{}{"scope": strings.TrimSpace(scopeKey), "anomalies": anomalies}})
}

// NotifyReportDue creates one deduplicated reminder per recipient.
func (s *Service) NotifyReportDue(ctx context.Context, recipientIDs []int64, periodID int64, periodKey string, dueAt *time.Time) error {
	for _, recipientID := range uniqueIDs(recipientIDs) {
		prefs, err := s.repo.GetPreferences(ctx, recipientID)
		if err != nil {
			return err
		}
		if !prefs.ReportDueEnabled {
			continue
		}
		if err := s.createInternal(ctx, CreateRequest{RecipientID: recipientID, Type: notification.TypeReportDue, Title: "月报截止提醒", BodyMarkdown: fmt.Sprintf("%s 月报即将截止，请提交或更新你的报告。", strings.TrimSpace(periodKey)), EntityType: "report_period", EntityID: periodID, DedupeKey: fmt.Sprintf("report-due:%d:%d", periodID, recipientID), Metadata: map[string]interface{}{"period_key": strings.TrimSpace(periodKey), "due_at": dueAt}}); err != nil {
			return err
		}
	}
	return nil
}

// RunReportDueReminders is safe to run repeatedly. The repository's unique
// recipient/dedupe key makes each period produce at most one reminder per
// member, while the due window allows the process to catch up after restart.
func (s *Service) RunReportDueReminders(ctx context.Context, now time.Time, lead time.Duration) error {
	if s.periodRepo == nil {
		return nil
	}
	if lead <= 0 {
		lead = 48 * time.Hour
	}
	periods, err := s.periodRepo.ListDuePeriods(ctx, now, now.Add(lead))
	if err != nil {
		return err
	}
	for _, period := range periods {
		ids, listErr := s.periodRepo.ListPendingReportMemberIDs(ctx, period.ID)
		if listErr != nil {
			return listErr
		}
		if err := s.NotifyReportDue(ctx, ids, period.ID, period.PeriodKey, period.DueAt); err != nil {
			return err
		}
	}
	return nil
}

// NotifyEvaluationPublished is used by the evaluation service after a draft
// is published. Team drafts notify active users; captain/private drafts notify
// the author and active managers.
func (s *Service) NotifyEvaluationPublished(ctx context.Context, draft *evaluation.Draft) error {
	if draft == nil || s.userRepo == nil {
		return shared.ErrInternal
	}
	users, err := s.userRepo.FindAll(ctx)
	if err != nil {
		return err
	}
	recipients := make([]int64, 0, len(users))
	for _, item := range users {
		if !item.IsActive() {
			continue
		}
		if draft.Audience == evaluation.DraftAudienceTeam || item.ID == draft.CreatedBy || item.IsCaptain() {
			recipients = append(recipients, item.ID)
		}
	}
	prefsEnabled := make([]int64, 0, len(recipients))
	for _, recipientID := range uniqueIDs(recipients) {
		prefs, prefErr := s.repo.GetPreferences(ctx, recipientID)
		if prefErr != nil {
			return prefErr
		}
		if prefs.EvaluationPublishedEnabled {
			prefsEnabled = append(prefsEnabled, recipientID)
		}
	}
	for _, recipientID := range prefsEnabled {
		if err := s.createInternal(ctx, CreateRequest{RecipientID: recipientID, Type: notification.TypeEvaluationPublished, Title: "考评已发布", BodyMarkdown: fmt.Sprintf("考评草稿《%s》已由队长确认发布。", draft.Title), EntityType: "evaluation_draft", EntityID: draft.ID, DedupeKey: fmt.Sprintf("evaluation-published:%d:%d", draft.ID, recipientID), Metadata: map[string]interface{}{"audience": draft.Audience, "snapshot_id": draft.SnapshotID}}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) createInternal(ctx context.Context, req CreateRequest) error {
	_, err := s.Create(ctx, req)
	return err
}

func toDTO(item *notification.Notification) *NotificationDTO {
	dto := &NotificationDTO{ID: item.ID, Type: string(item.Type), Title: item.Title, BodyMarkdown: item.BodyMarkdown, EntityType: item.EntityType, EntityID: item.EntityID, MetadataJSON: item.MetadataJSON, CreatedAt: item.CreatedAt.Format(time.RFC3339)}
	if item.ReadAt != nil {
		dto.ReadAt = item.ReadAt.Format(time.RFC3339)
	}
	if item.ExpiresAt != nil {
		dto.ExpiresAt = item.ExpiresAt.Format(time.RFC3339)
	}
	return dto
}

func preferencesDTO(item *notification.Preferences) *PreferencesDTO {
	return &PreferencesDTO{UserID: item.UserID, CheckinAnomalyEnabled: item.CheckinAnomalyEnabled, ReportDueEnabled: item.ReportDueEnabled, EvaluationPublishedEnabled: item.EvaluationPublishedEnabled, UpdatedAt: item.UpdatedAt.Format(time.RFC3339)}
}

func uniqueIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id > 0 {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				result = append(result, id)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			if _, ok := seen[value]; !ok {
				seen[value] = struct{}{}
				result = append(result, value)
			}
		}
	}
	sort.Strings(result)
	return result
}
