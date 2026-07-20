package checkin

import "context"

type Repository interface {
	Create(ctx context.Context, record *CheckinRecord) error
	FindByUserID(ctx context.Context, userID int64) ([]*CheckinRecord, error)
	FindByUserIDAndDate(ctx context.Context, userID int64, date string) ([]*CheckinRecord, error)
	FindByDate(ctx context.Context, date string) ([]*CheckinRecord, error)
	CountByDate(ctx context.Context, date string) (int64, error)
	GetLastByUserAndDate(ctx context.Context, userID int64, date string) (*CheckinRecord, error)
}
