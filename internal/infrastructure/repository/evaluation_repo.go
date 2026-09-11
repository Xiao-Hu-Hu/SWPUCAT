package repository

import (
	"SWPUCAT/internal/domain/evaluation"
	"SWPUCAT/internal/infrastructure/database"
	"context"
	"encoding/json"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type EvaluationRepo struct{ db *gorm.DB }

func NewEvaluationRepo(db *gorm.DB) *EvaluationRepo { return &EvaluationRepo{db: db} }

func (r *EvaluationRepo) FindSnapshotByID(ctx context.Context, id int64) (*evaluation.Snapshot, error) {
	var model database.EvaluationSnapshotModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, err
	}
	snapshot := &evaluation.Snapshot{ID: model.ID, PeriodID: model.PeriodID, PeriodKey: model.PeriodKey, StartsOn: model.StartsOn, EndsOn: model.EndsOn, Timezone: model.Timezone, CutoffAt: model.CutoffAt, RosterVersion: model.RosterVersion, RuleVersion: model.RuleVersion, SourceWatermark: model.SourceWatermark, SnapshotHash: model.SnapshotHash, Status: evaluation.SnapshotStatus(model.Status), CreatedBy: model.CreatedBy, CreatedAt: model.CreatedAt, Members: make([]evaluation.Member, 0)}
	var members []database.EvaluationSnapshotMemberModel
	if err := r.db.WithContext(ctx).Where("snapshot_id = ?", id).Order("user_id").Find(&members).Error; err != nil {
		return nil, err
	}
	for _, item := range members {
		var anomalies []string
		if err := json.Unmarshal([]byte(item.AnomaliesJSON), &anomalies); err != nil {
			anomalies = nil
		}
		var reportVersionID int64
		if item.ReportVersionID != nil {
			reportVersionID = *item.ReportVersionID
		}
		snapshot.Members = append(snapshot.Members, evaluation.Member{SnapshotID: item.SnapshotID, UserID: item.UserID, DisplayName: item.DisplayName, Required: item.Required, Exempt: item.Exempt, TotalMinutes: item.TotalMinutes, TotalHours: item.TotalHours, AttendanceDays: item.AttendanceDays, AchievedWeeks: item.AchievedWeeks, Anomalies: anomalies, ReportStatus: evaluation.ReportStatus(item.ReportStatus), ReportVersionID: reportVersionID, DataState: evaluation.DataState(item.DataState), UnknownReason: item.UnknownReason})
	}
	return snapshot, nil
}

func (r *EvaluationRepo) CreateDraft(ctx context.Context, draft *evaluation.Draft) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		model := &database.EvaluationDraftModel{ID: draft.ID, SnapshotID: draft.SnapshotID, PeriodID: draft.PeriodID, Status: string(draft.Status), Audience: string(draft.Audience), Title: draft.Title, SummaryMarkdown: draft.SummaryMarkdown, Model: draft.Model, PromptVersion: draft.PromptVersion, CreatedBy: draft.CreatedBy, CreatedAt: draft.CreatedAt, UpdatedAt: draft.UpdatedAt, PublishedAt: draft.PublishedAt}
		if err := tx.Create(model).Error; err != nil {
			return err
		}
		draft.ID, draft.CreatedAt, draft.UpdatedAt = model.ID, model.CreatedAt, model.UpdatedAt
		if len(draft.Members) == 0 {
			return nil
		}
		members := make([]database.EvaluationDraftMemberModel, 0, len(draft.Members))
		for i := range draft.Members {
			draft.Members[i].DraftID = model.ID
			draft.Members[i].UpdatedAt = model.UpdatedAt
			members = append(members, database.EvaluationDraftMemberModel{DraftID: model.ID, UserID: draft.Members[i].UserID, DisplayName: draft.Members[i].DisplayName, CommentMarkdown: draft.Members[i].CommentMarkdown, Recommendation: draft.Members[i].Recommendation, EvidenceJSON: draft.Members[i].EvidenceJSON, ManualOverride: draft.Members[i].ManualOverride, UpdatedAt: model.UpdatedAt})
		}
		return tx.Create(&members).Error
	})
}

func (r *EvaluationRepo) FindDraftByID(ctx context.Context, id int64) (*evaluation.Draft, error) {
	var model database.EvaluationDraftModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, err
	}
	draft := toDomainEvaluationDraft(&model)
	var members []database.EvaluationDraftMemberModel
	if err := r.db.WithContext(ctx).Where("draft_id = ?", id).Order("user_id").Find(&members).Error; err != nil {
		return nil, err
	}
	draft.Members = make([]evaluation.DraftMember, 0, len(members))
	for i := range members {
		draft.Members = append(draft.Members, toDomainEvaluationDraftMember(&members[i]))
	}
	return draft, nil
}

func (r *EvaluationRepo) UpdateDraft(ctx context.Context, draft *evaluation.Draft, editorID int64, expectedStatus evaluation.DraftStatus) error {
	if draft == nil || draft.ID <= 0 || editorID <= 0 {
		return evaluation.ErrInvalidSnapshot
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var previous database.EvaluationDraftModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&previous, draft.ID).Error; err != nil {
			return err
		}
		if evaluation.DraftStatus(previous.Status) != expectedStatus {
			return evaluation.ErrSnapshotChanged
		}
		updates := map[string]interface{}{"status": string(draft.Status), "title": draft.Title, "summary_markdown": draft.SummaryMarkdown, "updated_at": draft.UpdatedAt, "published_at": draft.PublishedAt}
		if err := tx.Model(&database.EvaluationDraftModel{}).Where("id = ? AND status = ?", draft.ID, string(expectedStatus)).Updates(updates).Error; err != nil {
			return err
		}
		edits := make([]database.EvaluationDraftEditModel, 0, 3)
		addEdit := func(field, oldValue, newValue string) {
			if oldValue != newValue {
				edits = append(edits, database.EvaluationDraftEditModel{DraftID: draft.ID, EditorID: editorID, Field: field, OldValue: oldValue, NewValue: newValue, CreatedAt: draft.UpdatedAt})
			}
		}
		addEdit("status", previous.Status, string(draft.Status))
		addEdit("title", previous.Title, draft.Title)
		addEdit("summary_markdown", previous.SummaryMarkdown, draft.SummaryMarkdown)
		if len(edits) > 0 {
			return tx.Create(&edits).Error
		}
		return nil
	})
}

func (r *EvaluationRepo) SaveMemberComment(ctx context.Context, member *evaluation.DraftMember, editorID int64) error {
	if member == nil || member.DraftID <= 0 || member.UserID <= 0 || editorID <= 0 {
		return evaluation.ErrInvalidSnapshot
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var draft database.EvaluationDraftModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&draft, member.DraftID).Error; err != nil {
			return err
		}
		if evaluation.DraftStatus(draft.Status) != evaluation.DraftStatusDraft {
			return evaluation.ErrSnapshotChanged
		}
		var previous database.EvaluationDraftMemberModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("draft_id = ? AND user_id = ?", member.DraftID, member.UserID).First(&previous).Error; err != nil {
			return err
		}
		now := time.Now()
		updates := map[string]interface{}{"comment_markdown": member.CommentMarkdown, "recommendation": member.Recommendation, "manual_override": true, "updated_at": now}
		if err := tx.Model(&database.EvaluationDraftMemberModel{}).Where("draft_id = ? AND user_id = ?", member.DraftID, member.UserID).Updates(updates).Error; err != nil {
			return err
		}
		edits := make([]database.EvaluationDraftEditModel, 0, 2)
		if previous.CommentMarkdown != member.CommentMarkdown {
			edits = append(edits, database.EvaluationDraftEditModel{DraftID: member.DraftID, EditorID: editorID, Field: "member.comment_markdown:" + strconv.FormatInt(member.UserID, 10), OldValue: previous.CommentMarkdown, NewValue: member.CommentMarkdown, CreatedAt: now})
		}
		if previous.Recommendation != member.Recommendation {
			edits = append(edits, database.EvaluationDraftEditModel{DraftID: member.DraftID, EditorID: editorID, Field: "member.recommendation:" + strconv.FormatInt(member.UserID, 10), OldValue: previous.Recommendation, NewValue: member.Recommendation, CreatedAt: now})
		}
		if len(edits) > 0 {
			return tx.Create(&edits).Error
		}
		return nil
	})
}

func toDomainEvaluationDraft(model *database.EvaluationDraftModel) *evaluation.Draft {
	return &evaluation.Draft{ID: model.ID, SnapshotID: model.SnapshotID, PeriodID: model.PeriodID, Status: evaluation.DraftStatus(model.Status), Audience: evaluation.DraftAudience(model.Audience), Title: model.Title, SummaryMarkdown: model.SummaryMarkdown, Model: model.Model, PromptVersion: model.PromptVersion, CreatedBy: model.CreatedBy, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt, PublishedAt: model.PublishedAt, Members: make([]evaluation.DraftMember, 0)}
}

func toDomainEvaluationDraftMember(model *database.EvaluationDraftMemberModel) evaluation.DraftMember {
	return evaluation.DraftMember{DraftID: model.DraftID, UserID: model.UserID, DisplayName: model.DisplayName, CommentMarkdown: model.CommentMarkdown, Recommendation: model.Recommendation, EvidenceJSON: model.EvidenceJSON, ManualOverride: model.ManualOverride, UpdatedAt: model.UpdatedAt}
}

var _ evaluation.Repository = (*EvaluationRepo)(nil)
