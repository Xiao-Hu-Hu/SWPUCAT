package repository

import (
	"SWPUCAT/internal/domain/announcement"
	"SWPUCAT/internal/infrastructure/database"
	"context"

	"gorm.io/gorm"
)

type AnnouncementRepo struct {
	db *gorm.DB
}

func NewAnnouncementRepo(db *gorm.DB) *AnnouncementRepo {
	return &AnnouncementRepo{db: db}
}

func (r *AnnouncementRepo) Create(ctx context.Context, a *announcement.Announcement) error {
	model := &database.AnnouncementModel{
		ID:         a.ID,
		Title:      a.Title,
		Content:    a.Content,
		AuthorID:   a.AuthorID,
		AuthorName: a.AuthorName,
		Pinned:     a.Pinned,
	}
	return r.db.WithContext(ctx).Create(model).Error
}

func (r *AnnouncementRepo) FindByID(ctx context.Context, id int64) (*announcement.Announcement, error) {
	var model database.AnnouncementModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, err
	}
	return toDomainAnnouncement(&model), nil
}

func (r *AnnouncementRepo) FindAll(ctx context.Context) ([]*announcement.Announcement, error) {
	var models []database.AnnouncementModel
	if err := r.db.WithContext(ctx).Order("pinned DESC, created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	anns := make([]*announcement.Announcement, len(models))
	for i, m := range models {
		anns[i] = toDomainAnnouncement(&m)
	}
	return anns, nil
}

func (r *AnnouncementRepo) FindPinned(ctx context.Context) ([]*announcement.Announcement, error) {
	var models []database.AnnouncementModel
	if err := r.db.WithContext(ctx).Where("pinned = ?", true).Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	anns := make([]*announcement.Announcement, len(models))
	for i, m := range models {
		anns[i] = toDomainAnnouncement(&m)
	}
	return anns, nil
}

func (r *AnnouncementRepo) Update(ctx context.Context, a *announcement.Announcement) error {
	return r.db.WithContext(ctx).Model(&database.AnnouncementModel{}).Where("id = ?", a.ID).Updates(map[string]interface{}{
		"title":   a.Title,
		"content": a.Content,
		"pinned":  a.Pinned,
	}).Error
}

func (r *AnnouncementRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&database.AnnouncementModel{}, id).Error
}

func (r *AnnouncementRepo) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&database.AnnouncementModel{}).Count(&count).Error
	return count, err
}

func toDomainAnnouncement(m *database.AnnouncementModel) *announcement.Announcement {
	return &announcement.Announcement{
		ID:         m.ID,
		Title:      m.Title,
		Content:    m.Content,
		AuthorID:   m.AuthorID,
		AuthorName: m.AuthorName,
		Pinned:     m.Pinned,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
	}
}
