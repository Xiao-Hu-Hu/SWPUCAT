package repository

import (
	"context"
	"errors"
	"time"

	"SWPUCAT/internal/domain/invitation"
	"SWPUCAT/internal/domain/user"
	"SWPUCAT/internal/infrastructure/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RegistrationRepo owns the cross-aggregate transaction used during sign-up.
// Keeping it in infrastructure prevents the application service from trying to
// coordinate two independent repositories and accidentally leaving a partial
// registration behind.
type RegistrationRepo struct {
	db *gorm.DB
}

func NewRegistrationRepo(db *gorm.DB) *RegistrationRepo {
	return &RegistrationRepo{db: db}
}

func (r *RegistrationRepo) CreateUserAndUseInvitation(ctx context.Context, u *user.User, invitationID int64) error {
	if invitationID <= 0 {
		return invitation.ErrCodeNotFound
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var code database.InvitationCodeModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&code, invitationID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return invitation.ErrCodeNotFound
			}
			return err
		}
		if code.Used {
			return invitation.ErrCodeAlreadyUsed
		}
		if !time.Now().Before(code.ExpiresAt) {
			return invitation.ErrCodeExpired
		}

		model := newUserModel(u)
		if err := tx.Create(model).Error; err != nil {
			return err
		}
		assignPersistedUser(u, model)

		result := tx.Model(&database.InvitationCodeModel{}).
			Where("id = ? AND used = false AND expires_at > ?", invitationID, time.Now()).
			Updates(map[string]interface{}{"used": true, "used_by": u.ID})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return invitation.ErrCodeAlreadyUsed
		}
		return nil
	})
}
