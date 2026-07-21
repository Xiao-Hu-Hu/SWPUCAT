package repository

import (
	"SWPUCAT/internal/domain/invitation"
	"SWPUCAT/internal/infrastructure/database"
	"context"
	"time"

	"gorm.io/gorm"
)

type InvitationRepo struct {
	db *gorm.DB
}

func NewInvitationRepo(db *gorm.DB) *InvitationRepo {
	return &InvitationRepo{db: db}
}

func (r *InvitationRepo) Create(ctx context.Context, code *invitation.InvitationCode) error {
	model := &database.InvitationCodeModel{
		Code:      code.Code,
		Type:      string(code.Type),
		CreatorID: code.CreatorID,
		UsedBy:    code.UsedBy,
		Used:      code.Used,
		ExpiresAt: code.ExpiresAt,
	}
	return r.db.WithContext(ctx).Create(model).Error
}

func (r *InvitationRepo) FindByCode(ctx context.Context, code string) (*invitation.InvitationCode, error) {
	var model database.InvitationCodeModel
	if err := r.db.WithContext(ctx).Where("code = ?", code).First(&model).Error; err != nil {
		return nil, err
	}
	return toDomainInvitation(&model), nil
}

func (r *InvitationRepo) FindByCreatorID(ctx context.Context, creatorID int64) ([]*invitation.InvitationCode, error) {
	var models []database.InvitationCodeModel
	if err := r.db.WithContext(ctx).Where("creator_id = ?", creatorID).Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	codes := make([]*invitation.InvitationCode, len(models))
	for i, m := range models {
		codes[i] = toDomainInvitation(&m)
	}
	return codes, nil
}

func (r *InvitationRepo) Update(ctx context.Context, code *invitation.InvitationCode) error {
	return r.db.WithContext(ctx).Model(&database.InvitationCodeModel{}).Where("id = ?", code.ID).Updates(map[string]interface{}{
		"used":     code.Used,
		"used_by":  code.UsedBy,
	}).Error
}

func (r *InvitationRepo) DeleteExpired(ctx context.Context) error {
	return r.db.WithContext(ctx).Where("expires_at < ? AND used = false", time.Now()).Delete(&database.InvitationCodeModel{}).Error
}

func toDomainInvitation(m *database.InvitationCodeModel) *invitation.InvitationCode {
	return &invitation.InvitationCode{
		ID:        m.ID,
		Code:      m.Code,
		Type:      invitation.InvitationType(m.Type),
		CreatorID: m.CreatorID,
		UsedBy:    m.UsedBy,
		Used:      m.Used,
		ExpiresAt: m.ExpiresAt,
		CreatedAt: m.CreatedAt,
	}
}
