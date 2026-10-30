package announcementdraft

import "context"

type Repository interface {
	Create(ctx context.Context, draft *Draft) error
	FindByID(ctx context.Context, id int64) (*Draft, error)
	List(ctx context.Context, status Status) ([]*Draft, error)
	Update(ctx context.Context, draft *Draft, actorID int64, expectedStatus Status) error
}
