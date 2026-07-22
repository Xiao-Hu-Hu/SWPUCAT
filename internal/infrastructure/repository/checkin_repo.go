package repository

import (
	"SWPUCAT/internal/domain/checkin"
	"SWPUCAT/internal/infrastructure/database"
	"context"

	"gorm.io/gorm"
)

type CheckinRepo struct {
	db *gorm.DB
}

func NewCheckinRepo(db *gorm.DB) *CheckinRepo {
	return &CheckinRepo{db: db}
}

func (r *CheckinRepo) Create(ctx context.Context, record *checkin.CheckinRecord) error {
	model := &database.CheckinRecordModel{
		ID:     record.ID,
		UserID: record.UserID,
		Type:   string(record.Type),
		Date:   record.Date,
		Time:   record.Time,
	}
	return r.db.WithContext(ctx).Create(model).Error
}

func (r *CheckinRepo) FindByUserID(ctx context.Context, userID int64) ([]*checkin.CheckinRecord, error) {
	var models []database.CheckinRecordModel
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	records := make([]*checkin.CheckinRecord, len(models))
	for i, m := range models {
		records[i] = toDomainCheckinRecord(&m)
	}
	return records, nil
}

func (r *CheckinRepo) FindByUserIDAndDate(ctx context.Context, userID int64, date string) ([]*checkin.CheckinRecord, error) {
	var models []database.CheckinRecordModel
	if err := r.db.WithContext(ctx).Where("user_id = ? AND date = ?", userID, date).Order("created_at").Find(&models).Error; err != nil {
		return nil, err
	}
	records := make([]*checkin.CheckinRecord, len(models))
	for i, m := range models {
		records[i] = toDomainCheckinRecord(&m)
	}
	return records, nil
}

func (r *CheckinRepo) FindByDate(ctx context.Context, date string) ([]*checkin.CheckinRecord, error) {
	var models []database.CheckinRecordModel
	if err := r.db.WithContext(ctx).Where("date = ?", date).Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	records := make([]*checkin.CheckinRecord, len(models))
	for i, m := range models {
		records[i] = toDomainCheckinRecord(&m)
	}
	return records, nil
}

func (r *CheckinRepo) CountByDate(ctx context.Context, date string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&database.CheckinRecordModel{}).Where("date = ?", date).Count(&count).Error
	return count, err
}

func (r *CheckinRepo) FindByUserIDAndDateRange(ctx context.Context, userID int64, startDate, endDate string) ([]*checkin.CheckinRecord, error) {
	var models []database.CheckinRecordModel
	if err := r.db.WithContext(ctx).Where("user_id = ? AND date >= ? AND date <= ?", userID, startDate, endDate).Order("created_at").Find(&models).Error; err != nil {
		return nil, err
	}
	records := make([]*checkin.CheckinRecord, len(models))
	for i, m := range models {
		records[i] = toDomainCheckinRecord(&m)
	}
	return records, nil
}

func (r *CheckinRepo) FindByDateRange(ctx context.Context, startDate, endDate string) ([]*checkin.CheckinRecord, error) {
	var models []database.CheckinRecordModel
	if err := r.db.WithContext(ctx).Where("date >= ? AND date <= ?", startDate, endDate).Order("user_id, created_at").Find(&models).Error; err != nil {
		return nil, err
	}
	records := make([]*checkin.CheckinRecord, len(models))
	for i, m := range models {
		records[i] = toDomainCheckinRecord(&m)
	}
	return records, nil
}

func (r *CheckinRepo) GetLastByUserAndDate(ctx context.Context, userID int64, date string) (*checkin.CheckinRecord, error) {
	var model database.CheckinRecordModel
	if err := r.db.WithContext(ctx).Where("user_id = ? AND date = ?", userID, date).Order("created_at DESC").First(&model).Error; err != nil {
		return nil, err
	}
	return toDomainCheckinRecord(&model), nil
}

func toDomainCheckinRecord(m *database.CheckinRecordModel) *checkin.CheckinRecord {
	return &checkin.CheckinRecord{
		ID:        m.ID,
		UserID:    m.UserID,
		Type:      checkin.CheckinType(m.Type),
		Date:      m.Date,
		Time:      m.Time,
		CreatedAt: m.CreatedAt,
	}
}
