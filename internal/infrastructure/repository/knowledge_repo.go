package repository

import (
	"SWPUCAT/internal/domain/knowledge"
	"SWPUCAT/internal/infrastructure/database"
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type KnowledgeRepo struct {
	db *gorm.DB
}

func NewKnowledgeRepo(db *gorm.DB) *KnowledgeRepo {
	return &KnowledgeRepo{db: db}
}

func (r *KnowledgeRepo) CreateItem(ctx context.Context, item *knowledge.KnowledgeItem) error {
	item.SetStatus(item.EffectiveStatus())
	if item.SourceOrigin == "" {
		item.SourceOrigin = knowledge.SourceOriginUser
	}
	if item.Revision < 1 {
		item.Revision = 1
	}
	model := &database.KnowledgeItemModel{
		Type:         string(item.Type),
		Name:         item.Name,
		Description:  item.Description,
		URL:          item.URL,
		FileSize:     item.FileSize,
		FileKey:      item.FileKey,
		CategoryID:   item.CategoryID,
		UploaderID:   item.UploaderID,
		UploaderName: item.UploaderName,
		Status:       string(item.Status),
		Approved:     item.Approved,
		Rejected:     item.Rejected,
		RejectReason: item.RejectReason,
		ReviewerID:   item.ReviewerID,
		ReviewerName: item.ReviewerName,
		SourceOrigin: item.SourceOrigin,
		SourceKey:    item.SourceKey,
		Revision:     item.Revision,
		DeletedAt:    item.DeletedAt,
		DeletedBy:    item.DeletedBy,
		CreatedAt:    item.CreatedAt,
	}
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(model).Error; err != nil {
			return err
		}
		if item.EffectiveStatus() == knowledge.ItemStatusApproved {
			return queueKnowledgeIndexJob(tx, model.ID)
		}
		return nil
	}); err != nil {
		return err
	}
	item.ID = model.ID
	item.CreatedAt = model.CreatedAt
	return nil
}

func (r *KnowledgeRepo) itemQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Model(&database.KnowledgeItemModel{}).
		Joins("LEFT JOIN knowledge_categories ON knowledge_categories.id = knowledge_items.category_id").
		Select("knowledge_items.*, knowledge_categories.name AS category_name").
		Where("knowledge_items.status <> ?", string(knowledge.ItemStatusDeleted)).
		Where("knowledge_items.deleted_at IS NULL")
}

func (r *KnowledgeRepo) FindItemByID(ctx context.Context, id int64) (*knowledge.KnowledgeItem, error) {
	var model database.KnowledgeItemModel
	if err := r.itemQuery(ctx).Where("knowledge_items.id = ?", id).First(&model).Error; err != nil {
		return nil, err
	}
	return toDomainKnowledgeItem(&model), nil
}

func (r *KnowledgeRepo) FindItems(ctx context.Context, filter knowledge.ItemFilter) ([]*knowledge.KnowledgeItem, error) {
	query := r.itemQuery(ctx)
	if filter.CategoryID != nil {
		query = query.Where("knowledge_items.category_id = ?", *filter.CategoryID)
	}
	if filter.Search != "" {
		pattern := "%" + filter.Search + "%"
		query = query.Where(
			"(knowledge_items.name ILIKE ? OR knowledge_items.description ILIKE ? OR knowledge_categories.name ILIKE ?)",
			pattern, pattern, pattern,
		)
	}
	if filter.Status != nil {
		query = query.Where("knowledge_items.status = ?", string(*filter.Status))
	} else if filter.Approved != nil {
		status := knowledge.ItemStatusPending
		if *filter.Approved {
			status = knowledge.ItemStatusApproved
		}
		query = query.Where("knowledge_items.status = ?", string(status))
	}
	if filter.Page > 0 && filter.PageSize > 0 {
		query = query.Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize)
	}

	var models []database.KnowledgeItemModel
	if err := query.Order("knowledge_items.created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]*knowledge.KnowledgeItem, 0, len(models))
	for i := range models {
		items = append(items, toDomainKnowledgeItem(&models[i]))
	}
	return items, nil
}

func (r *KnowledgeRepo) FindItemsByUploader(ctx context.Context, uploaderID int64) ([]*knowledge.KnowledgeItem, error) {
	var models []database.KnowledgeItemModel
	if err := r.itemQuery(ctx).
		Where("knowledge_items.uploader_id = ?", uploaderID).
		Order("knowledge_items.created_at DESC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]*knowledge.KnowledgeItem, 0, len(models))
	for i := range models {
		items = append(items, toDomainKnowledgeItem(&models[i]))
	}
	return items, nil
}

func (r *KnowledgeRepo) UpdateItem(ctx context.Context, item *knowledge.KnowledgeItem, expectedStatus knowledge.ItemStatus) error {
	item.SetStatus(item.EffectiveStatus())
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&database.KnowledgeItemModel{}).
			Where("id = ? AND status = ? AND deleted_at IS NULL", item.ID, string(expectedStatus)).
			Updates(map[string]interface{}{
				"name":          item.Name,
				"description":   item.Description,
				"url":           item.URL,
				"file_size":     item.FileSize,
				"file_key":      item.FileKey,
				"category_id":   item.CategoryID,
				"status":        string(item.Status),
				"approved":      item.Approved,
				"rejected":      item.Rejected,
				"reject_reason": item.RejectReason,
				"reviewer_id":   item.ReviewerID,
				"reviewer_name": item.ReviewerName,
				"deleted_at":    item.DeletedAt,
				"deleted_by":    item.DeletedBy,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return knowledge.ErrInvalidItemState
		}
		if item.EffectiveStatus() == knowledge.ItemStatusApproved {
			return queueKnowledgeIndexJob(tx, item.ID)
		}
		if err := tx.Exec("DELETE FROM knowledge_chunks WHERE knowledge_item_id = ?", item.ID).Error; err != nil {
			return err
		}
		return tx.Exec("DELETE FROM knowledge_index_jobs WHERE knowledge_item_id = ?", item.ID).Error
	})
}

func (r *KnowledgeRepo) DeleteItem(ctx context.Context, id, deletedBy int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&database.KnowledgeItemModel{}).
			Where("id = ? AND status <> ?", id, string(knowledge.ItemStatusDeleted)).
			Updates(map[string]interface{}{
				"status":     string(knowledge.ItemStatusDeleted),
				"approved":   false,
				"rejected":   false,
				"deleted_at": time.Now(),
				"deleted_by": deletedBy,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		if err := tx.Exec("DELETE FROM knowledge_chunks WHERE knowledge_item_id = ?", id).Error; err != nil {
			return err
		}
		return tx.Exec("DELETE FROM knowledge_index_jobs WHERE knowledge_item_id = ?", id).Error
	})
}

func (r *KnowledgeRepo) CountItems(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&database.KnowledgeItemModel{}).
		Where("status = ?", string(knowledge.ItemStatusApproved)).
		Where("deleted_at IS NULL").
		Count(&count).Error
	return count, err
}

func (r *KnowledgeRepo) CreateCategory(ctx context.Context, cat *knowledge.Category) error {
	model := &database.KnowledgeCategoryModel{
		Name:      cat.Name,
		IsSystem:  cat.IsSystem,
		CreatedAt: cat.CreatedAt,
	}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	cat.ID = model.ID
	cat.CreatedAt = model.CreatedAt
	return nil
}

func (r *KnowledgeRepo) FindAllCategories(ctx context.Context) ([]*knowledge.Category, error) {
	var models []database.KnowledgeCategoryModel
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	categories := make([]*knowledge.Category, 0, len(models))
	for i := range models {
		categories = append(categories, &knowledge.Category{
			ID:        models[i].ID,
			Name:      models[i].Name,
			IsSystem:  models[i].IsSystem,
			CreatedAt: models[i].CreatedAt,
		})
	}
	return categories, nil
}

func (r *KnowledgeRepo) FindCategoryByID(ctx context.Context, id int64) (*knowledge.Category, error) {
	var model database.KnowledgeCategoryModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, err
	}
	return &knowledge.Category{
		ID:        model.ID,
		Name:      model.Name,
		IsSystem:  model.IsSystem,
		CreatedAt: model.CreatedAt,
	}, nil
}

func (r *KnowledgeRepo) DeleteCategory(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&database.KnowledgeCategoryModel{}, id).Error
}

func (r *KnowledgeRepo) CountItemsByCategory(ctx context.Context, categoryID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&database.KnowledgeItemModel{}).
		Where("category_id = ?", categoryID).
		Where("status <> ?", string(knowledge.ItemStatusDeleted)).
		Where("deleted_at IS NULL").
		Count(&count).Error
	return count, err
}

func (r *KnowledgeRepo) CreateInsight(ctx context.Context, insight *knowledge.Insight) error {
	if insight == nil {
		return knowledge.ErrInvalidInsight
	}
	model := &database.KnowledgeItemInsightModel{ID: insight.ID, KnowledgeItemID: insight.KnowledgeItemID, SourceFingerprint: insight.SourceFingerprint, Status: string(insight.Status), SummaryMarkdown: insight.SummaryMarkdown, TagsJSON: insight.TagsJSON, Model: insight.Model, PromptVersion: insight.PromptVersion, ReviewDueAt: insight.ReviewDueAt, CreatedBy: insight.CreatedBy, CreatedAt: insight.CreatedAt, UpdatedAt: insight.UpdatedAt}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	insight.ID, insight.CreatedAt, insight.UpdatedAt = model.ID, model.CreatedAt, model.UpdatedAt
	return nil
}

func (r *KnowledgeRepo) FindInsightByItemID(ctx context.Context, itemID int64) (*knowledge.Insight, error) {
	var model database.KnowledgeItemInsightModel
	if err := r.db.WithContext(ctx).Where("knowledge_item_id = ?", itemID).Order("created_at DESC, id DESC").First(&model).Error; err != nil {
		return nil, err
	}
	return toDomainKnowledgeInsight(&model), nil
}

func (r *KnowledgeRepo) FindInsightByID(ctx context.Context, insightID int64) (*knowledge.Insight, error) {
	var model database.KnowledgeItemInsightModel
	if err := r.db.WithContext(ctx).First(&model, insightID).Error; err != nil {
		return nil, err
	}
	return toDomainKnowledgeInsight(&model), nil
}

func (r *KnowledgeRepo) UpdateInsight(ctx context.Context, insight *knowledge.Insight, expectedStatus knowledge.InsightStatus) error {
	if insight == nil || insight.ID <= 0 {
		return knowledge.ErrInvalidInsight
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var previous database.KnowledgeItemInsightModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&previous, insight.ID).Error; err != nil {
			return err
		}
		if knowledge.InsightStatus(previous.Status) != expectedStatus {
			return knowledge.ErrInvalidInsight
		}
		result := tx.Model(&database.KnowledgeItemInsightModel{}).Where("id = ? AND status = ?", insight.ID, string(expectedStatus)).Updates(map[string]interface{}{
			"status": string(insight.Status), "summary_markdown": insight.SummaryMarkdown, "tags_json": insight.TagsJSON, "reviewed_by": insight.ReviewedBy, "reviewed_at": insight.ReviewedAt, "updated_at": insight.UpdatedAt,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return knowledge.ErrInvalidInsight
		}
		return nil
	})
}

func (r *KnowledgeRepo) ListDueInsights(ctx context.Context, now time.Time, page, pageSize int) ([]*knowledge.Insight, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var models []database.KnowledgeItemInsightModel
	if err := r.db.WithContext(ctx).Where("status = ? AND review_due_at IS NOT NULL AND review_due_at <= ?", string(knowledge.InsightPublished), now).Order("review_due_at ASC, id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]*knowledge.Insight, 0, len(models))
	for i := range models {
		items = append(items, toDomainKnowledgeInsight(&models[i]))
	}
	return items, nil
}

func (r *KnowledgeRepo) SaveRecommendationFeedback(ctx context.Context, feedback *knowledge.RecommendationFeedback) error {
	if feedback == nil || feedback.UserID <= 0 || feedback.KnowledgeItemID <= 0 {
		return knowledge.ErrInvalidInsight
	}
	model := &database.KnowledgeRecommendationFeedbackModel{UserID: feedback.UserID, KnowledgeItemID: feedback.KnowledgeItemID, Helpful: feedback.Helpful, Note: feedback.Note, CreatedAt: feedback.CreatedAt}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "knowledge_item_id"}}, DoUpdates: clause.AssignmentColumns([]string{"helpful", "note", "created_at"})}).Create(model).Error
}

func toDomainKnowledgeItem(model *database.KnowledgeItemModel) *knowledge.KnowledgeItem {
	item := &knowledge.KnowledgeItem{
		ID:           model.ID,
		Type:         knowledge.ItemType(model.Type),
		Name:         model.Name,
		Description:  model.Description,
		URL:          model.URL,
		FileSize:     model.FileSize,
		FileKey:      model.FileKey,
		CategoryID:   model.CategoryID,
		CategoryName: model.CategoryName,
		UploaderID:   model.UploaderID,
		UploaderName: model.UploaderName,
		Status:       knowledge.ItemStatus(model.Status),
		Approved:     model.Approved,
		Rejected:     model.Rejected,
		RejectReason: model.RejectReason,
		ReviewerID:   model.ReviewerID,
		ReviewerName: model.ReviewerName,
		SourceOrigin: model.SourceOrigin,
		SourceKey:    model.SourceKey,
		Revision:     model.Revision,
		DeletedAt:    model.DeletedAt,
		DeletedBy:    model.DeletedBy,
		CreatedAt:    model.CreatedAt,
	}
	item.SetStatus(item.EffectiveStatus())
	return item
}

func toDomainKnowledgeInsight(model *database.KnowledgeItemInsightModel) *knowledge.Insight {
	var reviewedBy int64
	if model.ReviewedBy != nil {
		reviewedBy = *model.ReviewedBy
	}
	return &knowledge.Insight{ID: model.ID, KnowledgeItemID: model.KnowledgeItemID, SourceFingerprint: model.SourceFingerprint, Status: knowledge.InsightStatus(model.Status), SummaryMarkdown: model.SummaryMarkdown, TagsJSON: model.TagsJSON, Model: model.Model, PromptVersion: model.PromptVersion, ReviewDueAt: model.ReviewDueAt, CreatedBy: model.CreatedBy, ReviewedBy: reviewedBy, ReviewedAt: model.ReviewedAt, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt}
}
