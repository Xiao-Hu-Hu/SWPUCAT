package repository

import (
	"SWPUCAT/internal/infrastructure/database"
	"context"
	"time"

	"gorm.io/gorm"
)

type VerificationCodeRepo struct {
	db *gorm.DB
}

func NewVerificationCodeRepo(db *gorm.DB) *VerificationCodeRepo {
	return &VerificationCodeRepo{db: db}
}

func (r *VerificationCodeRepo) Create(ctx context.Context, email string, code string, expiresAt time.Time) error {
	model := &database.VerificationCodeModel{
		Email:     email,
		Code:      code,
		ExpiresAt: expiresAt,
	}
	return r.db.WithContext(ctx).Create(model).Error
}

func (r *VerificationCodeRepo) FindValidCode(ctx context.Context, email string, code string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&database.VerificationCodeModel{}).
		Where("email = ? AND code = ? AND used = false AND expires_at > ?", email, code, time.Now()).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	if count > 0 {
		// Mark code as used
		r.db.WithContext(ctx).Model(&database.VerificationCodeModel{}).
			Where("email = ? AND code = ? AND used = false", email, code).
			Update("used", true)
		return true, nil
	}
	return false, nil
}
