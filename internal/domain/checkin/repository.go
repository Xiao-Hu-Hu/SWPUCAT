package checkin

import "context"

type Repository interface {
	Create(ctx context.Context, record *CheckinRecord) error
	// CreateTransition atomically validates the user's latest attendance state and
	// persists the requested transition. The boolean is false when an
	// idempotency key replays an existing result. Implementations must serialize
	// transitions for the same user to prevent concurrent duplicate records.
	CreateTransition(ctx context.Context, record *CheckinRecord, idempotencyKey string) (created bool, err error)
	CreateMakeup(ctx context.Context, inRecord, outRecord *CheckinRecord) error
	FindByUserID(ctx context.Context, userID int64) ([]*CheckinRecord, error)
	FindByUserIDAndDate(ctx context.Context, userID int64, date string) ([]*CheckinRecord, error)
	FindByUserIDAndDateRange(ctx context.Context, userID int64, startDate, endDate string) ([]*CheckinRecord, error)
	FindByDate(ctx context.Context, date string) ([]*CheckinRecord, error)
	FindByDateRange(ctx context.Context, startDate, endDate string) ([]*CheckinRecord, error)
	CountByDate(ctx context.Context, date string) (int64, error)
	GetLastByUserAndDate(ctx context.Context, userID int64, date string) (*CheckinRecord, error)
}
