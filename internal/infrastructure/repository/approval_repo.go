package repository

import (
	"SWPUCAT/internal/domain/approval"
	"SWPUCAT/internal/infrastructure/database"
	"context"

	"gorm.io/gorm"
)

type ApprovalRepo struct {
	db *gorm.DB
}

func NewApprovalRepo(db *gorm.DB) *ApprovalRepo {
	return &ApprovalRepo{db: db}
}

func (r *ApprovalRepo) Create(ctx context.Context, a *approval.Approval) error {
	model := &database.ApprovalModel{
		ID:           a.ID,
		FileName:     a.FileName,
		FileSize:     a.FileSize,
		FileKey:      a.FileKey,
		CategoryID:   a.CategoryID,
		UploaderID:   a.UploaderID,
		UploaderName: a.UploaderName,
		Status:       string(a.Status),
	}
	return r.db.WithContext(ctx).Create(model).Error
}

func (r *ApprovalRepo) FindByID(ctx context.Context, id int64) (*approval.Approval, error) {
	var model database.ApprovalModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, err
	}
	return toDomainApproval(&model), nil
}

func (r *ApprovalRepo) FindPending(ctx context.Context) ([]*approval.Approval, error) {
	var models []database.ApprovalModel
	if err := r.db.WithContext(ctx).Where("status = ?", "pending").Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	approvals := make([]*approval.Approval, len(models))
	for i, m := range models {
		approvals[i] = toDomainApproval(&m)
	}
	return approvals, nil
}

func (r *ApprovalRepo) FindByUploaderID(ctx context.Context, uploaderID int64) ([]*approval.Approval, error) {
	var models []database.ApprovalModel
	if err := r.db.WithContext(ctx).Where("uploader_id = ?", uploaderID).Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	approvals := make([]*approval.Approval, len(models))
	for i, m := range models {
		approvals[i] = toDomainApproval(&m)
	}
	return approvals, nil
}

func (r *ApprovalRepo) Update(ctx context.Context, a *approval.Approval) error {
	return r.db.WithContext(ctx).Model(&database.ApprovalModel{}).Where("id = ?", a.ID).Updates(map[string]interface{}{
		"reviewer_id": *a.ReviewerID,
		"status":      string(a.Status),
	}).Error
}

func (r *ApprovalRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&database.ApprovalModel{}, id).Error
}

func (r *ApprovalRepo) CountPending(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&database.ApprovalModel{}).Where("status = ?", "pending").Count(&count).Error
	return count, err
}

func toDomainApproval(m *database.ApprovalModel) *approval.Approval {
	a := &approval.Approval{
		ID:           m.ID,
		FileName:     m.FileName,
		FileSize:     m.FileSize,
		FileKey:      m.FileKey,
		CategoryID:   m.CategoryID,
		UploaderID:   m.UploaderID,
		UploaderName: m.UploaderName,
		Status:       approval.ApprovalStatus(m.Status),
		CreatedAt:    m.CreatedAt,
	}
	if m.ReviewerID != 0 {
		a.ReviewerID = &m.ReviewerID
	}
	return a
}
