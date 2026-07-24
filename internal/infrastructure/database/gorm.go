package database

import (
	"SWPUCAT/internal/infrastructure/config"
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func NewGORM(cfg *config.DatabaseConfig) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)

	// Auto-migrate schema
	if err := db.AutoMigrate(
		&UserModel{},
		&AnnouncementModel{},
		&CheckinRecordModel{},
		&KnowledgeCategoryModel{},
		&KnowledgeItemModel{},
		&ApprovalModel{},
		&SettingModel{},
		&VerificationCodeModel{},
		&InvitationCodeModel{},
	); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate: %w", err)
	}

	return db, nil
}
