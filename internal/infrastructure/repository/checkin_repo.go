package repository

import (
	"SWPUCAT/internal/domain/checkin"
	"SWPUCAT/internal/domain/shared"
	"SWPUCAT/internal/domain/user"
	"SWPUCAT/internal/infrastructure/database"
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	record.ID = model.ID
	record.CreatedAt = model.CreatedAt
	return nil
}

// CreateTransition performs the attendance state check and insert in one
// transaction. Locking the user row gives every request for one user a stable
// serialization point, so two simultaneous clock-in/out requests cannot both
// observe the same previous state and create duplicate transitions.
func (r *CheckinRepo) CreateTransition(ctx context.Context, record *checkin.CheckinRecord, idempotencyKey string) (bool, error) {
	if record == nil || !record.Type.IsValid() {
		return false, checkin.ErrInvalidCheckinType
	}

	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var account database.UserModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, record.UserID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return user.ErrUserNotFound
			}
			return err
		}
		if account.Status != "" && account.Status != string(user.StatusActive) {
			return shared.ErrForbidden
		}

		if idempotencyKey != "" {
			var previous database.CheckinIdempotencyModel
			lookupErr := tx.Where("user_id = ? AND action = ? AND idempotency_key = ?", record.UserID, string(record.Type), idempotencyKey).
				First(&previous).Error
			if lookupErr == nil {
				record.ID = previous.RecordID
				record.Date = previous.RecordDate
				record.Time = previous.RecordTime
				record.CreatedAt = previous.CreatedAt
				return nil
			}
			if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				return lookupErr
			}
		}

		var latest database.CheckinRecordModel
		latestErr := tx.Where("user_id = ? AND date = ?", record.UserID, record.Date).
			Order("created_at DESC, id DESC").First(&latest).Error
		hasLatest := latestErr == nil
		if latestErr != nil && !errors.Is(latestErr, gorm.ErrRecordNotFound) {
			return latestErr
		}

		switch record.Type {
		case checkin.CheckinTypeIn:
			if hasLatest && latest.Type == string(checkin.CheckinTypeIn) {
				return checkin.ErrAlreadyClockedIn
			}
		case checkin.CheckinTypeOut:
			if !hasLatest || latest.Type != string(checkin.CheckinTypeIn) {
				return checkin.ErrNotClockedIn
			}
		}

		model := &database.CheckinRecordModel{
			UserID: record.UserID,
			Type:   string(record.Type),
			Date:   record.Date,
			Time:   record.Time,
		}
		if !record.CreatedAt.IsZero() {
			model.CreatedAt = record.CreatedAt
		}
		if err := tx.Create(model).Error; err != nil {
			return err
		}
		record.ID = model.ID
		record.CreatedAt = model.CreatedAt

		if record.Type == checkin.CheckinTypeIn {
			if err := tx.Model(&database.UserModel{}).Where("id = ?", record.UserID).Updates(map[string]interface{}{
				"checkin_count":     gorm.Expr("checkin_count + 1"),
				"last_checkin_date": record.Date,
				"updated_at":        gorm.Expr("CURRENT_TIMESTAMP"),
			}).Error; err != nil {
				return err
			}
		}
		if idempotencyKey != "" {
			if err := tx.Create(&database.CheckinIdempotencyModel{
				UserID:         record.UserID,
				Action:         string(record.Type),
				IdempotencyKey: idempotencyKey,
				RecordID:       record.ID,
				RecordDate:     record.Date,
				RecordTime:     record.Time,
			}).Error; err != nil {
				return err
			}
		}
		created = true
		return nil
	})
	return created, err
}

func (r *CheckinRepo) FindByUserID(ctx context.Context, userID int64) ([]*checkin.CheckinRecord, error) {
	var models []database.CheckinRecordModel
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC, id DESC").Find(&models).Error; err != nil {
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
	if err := r.db.WithContext(ctx).Where("user_id = ? AND date = ?", userID, date).Order("created_at, id").Find(&models).Error; err != nil {
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
	if err := r.db.WithContext(ctx).Where("date = ?", date).Order("created_at DESC, id DESC").Find(&models).Error; err != nil {
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
	if err := r.db.WithContext(ctx).Where("user_id = ? AND date >= ? AND date <= ?", userID, startDate, endDate).Order("created_at, id").Find(&models).Error; err != nil {
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
	if err := r.db.WithContext(ctx).Where("date >= ? AND date <= ?", startDate, endDate).Order("user_id, created_at, id").Find(&models).Error; err != nil {
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
	if err := r.db.WithContext(ctx).Where("user_id = ? AND date = ?", userID, date).Order("created_at DESC, id DESC").First(&model).Error; err != nil {
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
