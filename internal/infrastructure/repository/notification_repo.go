package repository

import (
	"SWPUCAT/internal/domain/notification"
	"SWPUCAT/internal/infrastructure/database"
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NotificationRepo struct{ db *gorm.DB }

func NewNotificationRepo(db *gorm.DB) *NotificationRepo { return &NotificationRepo{db: db} }

func (r *NotificationRepo) Create(ctx context.Context, item *notification.Notification) error {
	if item == nil {
		return notification.ErrInvalidNotification
	}
	model := &database.NotificationModel{ID: item.ID, RecipientID: item.RecipientID, NotificationType: string(item.Type), Title: item.Title, BodyMarkdown: item.BodyMarkdown, EntityType: item.EntityType, DedupeKey: item.DedupeKey, MetadataJSON: item.MetadataJSON, ReadAt: item.ReadAt, ExpiresAt: item.ExpiresAt, CreatedAt: item.CreatedAt}
	if item.EntityID != 0 {
		entityID := item.EntityID
		model.EntityID = &entityID
	}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "recipient_id"}, {Name: "dedupe_key"}}, DoNothing: true}).Create(model)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var existing database.NotificationModel
		if err := r.db.WithContext(ctx).Where("recipient_id = ? AND dedupe_key = ?", item.RecipientID, item.DedupeKey).First(&existing).Error; err != nil {
			return err
		}
		item.ID = existing.ID
		item.CreatedAt = existing.CreatedAt
		return nil
	}
	item.ID, item.CreatedAt = model.ID, model.CreatedAt
	return nil
}

func (r *NotificationRepo) ListForUser(ctx context.Context, userID int64, unreadOnly bool, page, pageSize int) ([]*notification.Notification, error) {
	if userID <= 0 {
		return nil, notification.ErrInvalidNotification
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	query := r.db.WithContext(ctx).Where("recipient_id = ? AND (expires_at IS NULL OR expires_at > ?)", userID, time.Now())
	if unreadOnly {
		query = query.Where("read_at IS NULL")
	}
	var models []database.NotificationModel
	if err := query.Order("created_at DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]*notification.Notification, 0, len(models))
	for i := range models {
		items = append(items, toDomainNotification(&models[i]))
	}
	return items, nil
}

func (r *NotificationRepo) CountUnread(ctx context.Context, userID int64) (int64, error) {
	if userID <= 0 {
		return 0, notification.ErrInvalidNotification
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&database.NotificationModel{}).Where("recipient_id = ? AND read_at IS NULL AND (expires_at IS NULL OR expires_at > ?)", userID, time.Now()).Count(&count).Error
	return count, err
}

func (r *NotificationRepo) MarkRead(ctx context.Context, userID, notificationID int64) error {
	if userID <= 0 || notificationID <= 0 {
		return notification.ErrInvalidNotification
	}
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&database.NotificationModel{}).Where("id = ? AND recipient_id = ? AND read_at IS NULL", notificationID, userID).Update("read_at", now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	var exists database.NotificationModel
	if err := r.db.WithContext(ctx).Where("id = ? AND recipient_id = ?", notificationID, userID).First(&exists).Error; err != nil {
		return err
	}
	return notification.ErrAlreadyRead
}

func (r *NotificationRepo) GetPreferences(ctx context.Context, userID int64) (*notification.Preferences, error) {
	if userID <= 0 {
		return nil, notification.ErrInvalidNotification
	}
	var model database.NotificationPreferenceModel
	err := r.db.WithContext(ctx).First(&model, userID).Error
	if err == gorm.ErrRecordNotFound {
		return &notification.Preferences{UserID: userID, CheckinAnomalyEnabled: true, ReportDueEnabled: true, EvaluationPublishedEnabled: true}, nil
	}
	if err != nil {
		return nil, err
	}
	return toDomainPreferences(&model), nil
}

func (r *NotificationRepo) SavePreferences(ctx context.Context, preferences *notification.Preferences) error {
	if preferences == nil || preferences.UserID <= 0 {
		return notification.ErrInvalidNotification
	}
	model := &database.NotificationPreferenceModel{UserID: preferences.UserID, CheckinAnomalyEnabled: preferences.CheckinAnomalyEnabled, ReportDueEnabled: preferences.ReportDueEnabled, EvaluationPublishedEnabled: preferences.EvaluationPublishedEnabled, UpdatedAt: time.Now()}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}}, DoUpdates: clause.AssignmentColumns([]string{"checkin_anomaly_enabled", "report_due_enabled", "evaluation_published_enabled", "updated_at"})}).Create(model).Error; err != nil {
		return err
	}
	preferences.UpdatedAt = model.UpdatedAt
	return nil
}

func toDomainNotification(model *database.NotificationModel) *notification.Notification {
	var entityID int64
	if model.EntityID != nil {
		entityID = *model.EntityID
	}
	return &notification.Notification{ID: model.ID, RecipientID: model.RecipientID, Type: notification.Type(model.NotificationType), Title: model.Title, BodyMarkdown: model.BodyMarkdown, EntityType: model.EntityType, EntityID: entityID, DedupeKey: model.DedupeKey, MetadataJSON: model.MetadataJSON, ReadAt: model.ReadAt, ExpiresAt: model.ExpiresAt, CreatedAt: model.CreatedAt}
}

func toDomainPreferences(model *database.NotificationPreferenceModel) *notification.Preferences {
	return &notification.Preferences{UserID: model.UserID, CheckinAnomalyEnabled: model.CheckinAnomalyEnabled, ReportDueEnabled: model.ReportDueEnabled, EvaluationPublishedEnabled: model.EvaluationPublishedEnabled, UpdatedAt: model.UpdatedAt}
}

var _ notification.Repository = (*NotificationRepo)(nil)
