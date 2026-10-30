package notification

import "context"

type Repository interface {
	Create(ctx context.Context, item *Notification) error
	ListForUser(ctx context.Context, userID int64, unreadOnly bool, page, pageSize int) ([]*Notification, error)
	CountUnread(ctx context.Context, userID int64) (int64, error)
	MarkRead(ctx context.Context, userID, notificationID int64) error
	GetPreferences(ctx context.Context, userID int64) (*Preferences, error)
	SavePreferences(ctx context.Context, preferences *Preferences) error
}
