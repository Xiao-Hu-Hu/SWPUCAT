package repository

import (
	"SWPUCAT/internal/domain/knowledge"
	"SWPUCAT/internal/infrastructure/database"
	"context"

	"gorm.io/gorm"
)

type KnowledgeRepo struct {
	db *gorm.DB
}

func NewKnowledgeRepo(db *gorm.DB) *KnowledgeRepo {
	return &KnowledgeRepo{db: db}
}

func (r *KnowledgeRepo) CreateItem(ctx context.Context, item *knowledge.KnowledgeItem) error {
	model := &database.KnowledgeItemModel{
		ID:           item.ID,
		Type:         string(item.Type),
		Name:         item.Name,
		URL:          item.URL,
		FileSize:     item.FileSize,
		FileKey:      item.FileKey,
		CategoryID:   item.CategoryID,
		UploaderID:   item.UploaderID,
		UploaderName: item.UploaderName,
		Approved:     item.Approved,
	}
	return r.db.WithContext(ctx).Create(model).Error
}

func (r *KnowledgeRepo) FindItemByID(ctx context.Context, id int64) (*knowledge.KnowledgeItem, error) {
	var model database.KnowledgeItemModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, err
	}
	return toDomainKnowledgeItem(&model), nil
}

func (r *KnowledgeRepo) FindItems(ctx context.Context, filter knowledge.ItemFilter) ([]*knowledge.KnowledgeItem, error) {
	query := r.db.WithContext(ctx).Model(&database.KnowledgeItemModel{})
	if filter.CategoryID != nil {
		query = query.Where("category_id = ?", *filter.CategoryID)
	}
	if filter.Search != "" {
		query = query.Where("name LIKE ?", "%"+filter.Search+"%")
	}
	if filter.Approved != nil {
		query = query.Where("approved = ?", *filter.Approved)
	}

	var models []database.KnowledgeItemModel
	if err := query.Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]*knowledge.KnowledgeItem, len(models))
	for i, m := range models {
		items[i] = toDomainKnowledgeItem(&m)
	}
	return items, nil
}

func (r *KnowledgeRepo) UpdateItem(ctx context.Context, item *knowledge.KnowledgeItem) error {
	return r.db.WithContext(ctx).Model(&database.KnowledgeItemModel{}).Where("id = ?", item.ID).Updates(map[string]interface{}{
		"name":        item.Name,
		"url":         item.URL,
		"file_size":   item.FileSize,
		"category_id": item.CategoryID,
		"approved":    item.Approved,
	}).Error
}

func (r *KnowledgeRepo) DeleteItem(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&database.KnowledgeItemModel{}, id).Error
}

func (r *KnowledgeRepo) CountItems(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&database.KnowledgeItemModel{}).Where("approved = ?", true).Count(&count).Error
	return count, err
}

func (r *KnowledgeRepo) CountItemsByCategory(ctx context.Context, categoryID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&database.KnowledgeItemModel{}).Where("category_id = ?", categoryID).Count(&count).Error
	return count, err
}

func (r *KnowledgeRepo) CreateCategory(ctx context.Context, cat *knowledge.Category) error {
	model := &database.KnowledgeCategoryModel{
		ID:       cat.ID,
		Name:     cat.Name,
		IsSystem: cat.IsSystem,
	}
	return r.db.WithContext(ctx).Create(model).Error
}

func (r *KnowledgeRepo) FindCategoryByID(ctx context.Context, id int64) (*knowledge.Category, error) {
	var model database.KnowledgeCategoryModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, err
	}
	return toDomainCategory(&model), nil
}

func (r *KnowledgeRepo) FindAllCategories(ctx context.Context) ([]*knowledge.Category, error) {
	var models []database.KnowledgeCategoryModel
	if err := r.db.WithContext(ctx).Order("id").Find(&models).Error; err != nil {
		return nil, err
	}
	cats := make([]*knowledge.Category, len(models))
	for i, m := range models {
		cats[i] = toDomainCategory(&m)
	}
	return cats, nil
}

func (r *KnowledgeRepo) DeleteCategory(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&database.KnowledgeCategoryModel{}, id).Error
}

func toDomainKnowledgeItem(m *database.KnowledgeItemModel) *knowledge.KnowledgeItem {
	return &knowledge.KnowledgeItem{
		ID:           m.ID,
		Type:         knowledge.ItemType(m.Type),
		Name:         m.Name,
		URL:          m.URL,
		FileSize:     m.FileSize,
		FileKey:      m.FileKey,
		CategoryID:   m.CategoryID,
		UploaderID:   m.UploaderID,
		UploaderName: m.UploaderName,
		Approved:     m.Approved,
		CreatedAt:    m.CreatedAt,
	}
}

func toDomainCategory(m *database.KnowledgeCategoryModel) *knowledge.Category {
	return &knowledge.Category{
		ID:        m.ID,
		Name:      m.Name,
		IsSystem:  m.IsSystem,
		CreatedAt: m.CreatedAt,
	}
}
