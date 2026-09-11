package repository

import (
	"SWPUCAT/internal/domain/announcementdraft"
	"SWPUCAT/internal/infrastructure/database"
	"context"
	"encoding/json"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AnnouncementDraftRepo struct{ db *gorm.DB }

func NewAnnouncementDraftRepo(db *gorm.DB) *AnnouncementDraftRepo {
	return &AnnouncementDraftRepo{db: db}
}

func (r *AnnouncementDraftRepo) Create(ctx context.Context, draft *announcementdraft.Draft) error {
	if draft == nil {
		return announcementdraft.ErrInvalidDraft
	}
	model := &database.AnnouncementDraftModel{ID: draft.ID, Status: string(draft.Status), Title: draft.Title, Content: draft.Content, Pinned: draft.Pinned, SourceJSON: draft.SourceJSON, RecipientIDsJSON: draft.RecipientIDsJSON, EmailRequested: draft.EmailRequested, EmailResultJSON: draft.EmailResultJSON, CreatedBy: draft.CreatedBy, CreatedAt: draft.CreatedAt, UpdatedAt: draft.UpdatedAt, ConfirmedAt: draft.ConfirmedAt, PublishedAt: draft.PublishedAt}
	if draft.AnnouncementID != 0 {
		value := draft.AnnouncementID
		model.AnnouncementID = &value
	}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	draft.ID, draft.CreatedAt, draft.UpdatedAt = model.ID, model.CreatedAt, model.UpdatedAt
	return nil
}

func (r *AnnouncementDraftRepo) FindByID(ctx context.Context, id int64) (*announcementdraft.Draft, error) {
	var model database.AnnouncementDraftModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, err
	}
	return toDomainAnnouncementDraft(&model), nil
}

func (r *AnnouncementDraftRepo) List(ctx context.Context, status announcementdraft.Status) ([]*announcementdraft.Draft, error) {
	var models []database.AnnouncementDraftModel
	query := r.db.WithContext(ctx).Order("updated_at DESC, id DESC")
	if status != "" {
		query = query.Where("status = ?", string(status))
	}
	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]*announcementdraft.Draft, 0, len(models))
	for i := range models {
		result = append(result, toDomainAnnouncementDraft(&models[i]))
	}
	return result, nil
}

func (r *AnnouncementDraftRepo) Update(ctx context.Context, draft *announcementdraft.Draft, actorID int64, expectedStatus announcementdraft.Status) error {
	if draft == nil || draft.ID <= 0 || actorID <= 0 {
		return announcementdraft.ErrInvalidDraft
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var previous database.AnnouncementDraftModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&previous, draft.ID).Error; err != nil {
			return err
		}
		if announcementdraft.Status(previous.Status) != expectedStatus {
			return announcementdraft.ErrInvalidDraft
		}
		updates := map[string]interface{}{"status": string(draft.Status), "title": draft.Title, "content": draft.Content, "pinned": draft.Pinned, "source_json": draft.SourceJSON, "recipient_ids_json": draft.RecipientIDsJSON, "email_requested": draft.EmailRequested, "email_result_json": draft.EmailResultJSON, "announcement_id": nullableID(draft.AnnouncementID), "updated_at": draft.UpdatedAt, "confirmed_at": draft.ConfirmedAt, "published_at": draft.PublishedAt}
		if err := tx.Model(&database.AnnouncementDraftModel{}).Where("id = ? AND status = ?", draft.ID, string(expectedStatus)).Updates(updates).Error; err != nil {
			return err
		}
		detail, _ := json.Marshal(map[string]string{"from": previous.Status, "to": string(draft.Status)})
		return tx.Create(&database.AnnouncementDraftEventModel{DraftID: draft.ID, ActorID: actorID, EventType: "status_changed", DetailJSON: string(detail), CreatedAt: draft.UpdatedAt}).Error
	})
}

func nullableID(id int64) interface{} {
	if id <= 0 {
		return nil
	}
	return id
}

func toDomainAnnouncementDraft(model *database.AnnouncementDraftModel) *announcementdraft.Draft {
	var announcementID int64
	if model.AnnouncementID != nil {
		announcementID = *model.AnnouncementID
	}
	return &announcementdraft.Draft{ID: model.ID, Status: announcementdraft.Status(model.Status), Title: model.Title, Content: model.Content, Pinned: model.Pinned, SourceJSON: model.SourceJSON, RecipientIDsJSON: model.RecipientIDsJSON, EmailRequested: model.EmailRequested, EmailResultJSON: model.EmailResultJSON, AnnouncementID: announcementID, CreatedBy: model.CreatedBy, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt, ConfirmedAt: model.ConfirmedAt, PublishedAt: model.PublishedAt}
}

var _ announcementdraft.Repository = (*AnnouncementDraftRepo)(nil)
