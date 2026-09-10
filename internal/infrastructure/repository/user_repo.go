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
	model := newUserModel(u)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	assignPersistedUser(u, model)
	return nil
}

func newUserModel(u *user.User) *database.UserModel {
	return &database.UserModel{
		ID:            u.ID,
		Username:      string(u.Username),
		StudentID:     string(u.StudentID),
		Email:         u.Email,
		PasswordHash:  string(u.PasswordHash),
		Nickname:      string(u.Nickname),
		Avatar:        u.Avatar,
		TechDirection: u.TechDirection,
		Role:          string(u.Role),
		Status:        string(u.Status),
		AuthVersion:   u.AuthVersion,
		DisabledAt:    u.DisabledAt,
		CheckinCount:  u.CheckinCount,
	}
}

func assignPersistedUser(u *user.User, model *database.UserModel) {
	u.ID = model.ID
	u.Status = user.Status(model.Status)
	u.AuthVersion = model.AuthVersion
	u.DisabledAt = model.DisabledAt
	u.CreatedAt = model.CreatedAt
	u.UpdatedAt = model.UpdatedAt
	if u.JoinedAt.IsZero() {
		u.JoinedAt = model.CreatedAt
	}
}

func (r *UserRepo) FindByID(ctx context.Context, id int64) (*user.User, error) {
	var model database.UserModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, err
	}
	return toDomainUser(&model), nil
}

func (r *UserRepo) FindByUsername(ctx context.Context, username user.Username) (*user.User, error) {
	var model database.UserModel
	if err := r.db.WithContext(ctx).Where("username = ?", string(username)).First(&model).Error; err != nil {
		return nil, err
	}
	return toDomainUser(&model), nil
}

func (r *UserRepo) FindByStudentID(ctx context.Context, studentID user.StudentID) (*user.User, error) {
	var model database.UserModel
	if err := r.db.WithContext(ctx).Where("student_id = ?", string(studentID)).First(&model).Error; err != nil {
		return nil, err
	}
	return toDomainUser(&model), nil
}

func (r *UserRepo) FindAll(ctx context.Context) ([]*user.User, error) {
	var models []database.UserModel
	if err := r.db.WithContext(ctx).Where("status = ?", string(user.StatusActive)).Order("id").Find(&models).Error; err != nil {
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
	if err := r.db.WithContext(ctx).Where("id IN ? AND status = ?", ids, string(user.StatusActive)).Find(&models).Error; err != nil {
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
		"nickname":       string(u.Nickname),
		"avatar":         u.Avatar,
		"tech_direction": u.TechDirection,
		"password_hash":  string(u.PasswordHash),
		"role":           string(u.Role),
		"status":         string(u.Status),
		"auth_version":   u.AuthVersion,
		"disabled_at":    u.DisabledAt,
		"checkin_count":  u.CheckinCount,
	}).Error
}

func (r *UserRepo) Delete(ctx context.Context, id int64) error {
	result := r.db.WithContext(ctx).Model(&database.UserModel{}).
		Where("id = ? AND status = ?", id, string(user.StatusActive)).
		Updates(map[string]interface{}{
			"status":       string(user.StatusDisabled),
			"disabled_at":  gorm.Expr("CURRENT_TIMESTAMP"),
			"auth_version": gorm.Expr("auth_version + 1"),
			"updated_at":   gorm.Expr("CURRENT_TIMESTAMP"),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *UserRepo) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&database.UserModel{}).Where("status = ?", string(user.StatusActive)).Count(&count).Error
	return count, err
}

func (r *UserRepo) ExistsByUsername(ctx context.Context, username user.Username) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&database.UserModel{}).Where("username = ?", string(username)).Count(&count).Error
	return count > 0, err
}

func (r *UserRepo) ExistsByStudentID(ctx context.Context, studentID user.StudentID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&database.UserModel{}).Where("student_id = ?", string(studentID)).Count(&count).Error
	return count > 0, err
}

func toDomainUser(m *database.UserModel) *user.User {
	return &user.User{
		ID:            m.ID,
		Username:      user.Username(m.Username),
		StudentID:     user.StudentID(m.StudentID),
		Email:         m.Email,
		PasswordHash:  user.PasswordHash(m.PasswordHash),
		Nickname:      user.Nickname(m.Nickname),
		Avatar:        m.Avatar,
		TechDirection: m.TechDirection,
		Role:          user.Role(m.Role),
		Status:        user.Status(m.Status),
		AuthVersion:   m.AuthVersion,
		DisabledAt:    m.DisabledAt,
		CheckinCount:  m.CheckinCount,
		JoinedAt:      m.CreatedAt,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}
