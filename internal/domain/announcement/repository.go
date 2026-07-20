package announcement

import "context"

type Repository interface {
	Create(ctx context.Context, ann *Announcement) error
	FindByID(ctx context.Context, id int64) (*Announcement, error)
	FindAll(ctx context.Context) ([]*Announcement, error)
	FindPinned(ctx context.Context) ([]*Announcement, error)
	Update(ctx context.Context, ann *Announcement) error
	Delete(ctx context.Context, id int64) error
	Count(ctx context.Context) (int64, error)
}
