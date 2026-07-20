package repository

import (
	"SWPUCAT/internal/domain/user"
	"SWPUCAT/internal/infrastructure/database"
	"context"

	"gorm.io/gorm"
)

type UserRepo struct {
	db *gorm.DB
}

func NewUserRepo(db *gorm.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) Create(ctx context.Context, u *user.User) error {
	model := &database.UserModel{
		ID:           u.ID,
		Username:     string(u.Username),
		PasswordHash: string(u.PasswordHash),
		Nickname:     string(u.Nickname),
		Role:         string(u.Role),
		CheckinCount: u.CheckinCount,
	}
	return r.db.WithContext(ctx).Create(model).Error
}

func (r *UserRepo) FindByID(ctx context.Context, id int64) (*user.User, error) {
	var model database.UserModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, err
	}
	return toDomainUser(&model), nil
}

func (r *UserRepo) FindByUsername(ctx context.Context, username string) (*user.User, error) {
	var model database.UserModel
	if err := r.db.WithContext(ctx).Where("username = ?", username).First(&model).Error; err != nil {
		return nil, err
	}
	return toDomainUser(&model), nil
}

func (r *UserRepo) FindAll(ctx context.Context) ([]*user.User, error) {
	var models []database.UserModel
	if err := r.db.WithContext(ctx).Order("id").Find(&models).Error; err != nil {
		return nil, err
	}
	users := make([]*user.User, len(models))
	for i, m := range models {
		users[i] = toDomainUser(&m)
	}
	return users, nil
}

func (r *UserRepo) FindByIDs(ctx context.Context, ids []int64) ([]*user.User, error) {
	var models []database.UserModel
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&models).Error; err != nil {
		return nil, err
	}
	users := make([]*user.User, len(models))
	for i, m := range models {
		users[i] = toDomainUser(&m)
	}
	return users, nil
}

func (r *UserRepo) Update(ctx context.Context, u *user.User) error {
	return r.db.WithContext(ctx).Model(&database.UserModel{}).Where("id = ?", u.ID).Updates(map[string]interface{}{
		"nickname":      string(u.Nickname),
		"role":          string(u.Role),
		"checkin_count": u.CheckinCount,
	}).Error
}

func (r *UserRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&database.UserModel{}, id).Error
}

func (r *UserRepo) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&database.UserModel{}).Count(&count).Error
	return count, err
}

func (r *UserRepo) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&database.UserModel{}).Where("username = ?", username).Count(&count).Error
	return count > 0, err
}

func toDomainUser(m *database.UserModel) *user.User {
	return &user.User{
		ID:           m.ID,
		Username:     user.Username(m.Username),
		PasswordHash: user.PasswordHash(m.PasswordHash),
		Nickname:     user.Nickname(m.Nickname),
		Role:         user.Role(m.Role),
		CheckinCount: m.CheckinCount,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}
