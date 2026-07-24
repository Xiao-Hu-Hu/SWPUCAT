package repository

import (
	"SWPUCAT/internal/infrastructure/database"
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SettingsRepo struct {
	db *gorm.DB
}

func NewSettingsRepo(db *gorm.DB) *SettingsRepo {
	return &SettingsRepo{db: db}
}

func (r *SettingsRepo) Get(ctx context.Context, key string) (string, error) {
	var model database.SettingModel
	if err := r.db.WithContext(ctx).Where("key = ?", key).First(&model).Error; err != nil {
		return "", err
	}
	return model.Value, nil
}

func (r *SettingsRepo) Set(ctx context.Context, key, value string) error {
	model := &database.SettingModel{Key: key, Value: value}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(model).Error
}
