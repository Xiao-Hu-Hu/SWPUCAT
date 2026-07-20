package approval

import "context"

type Repository interface {
	Create(ctx context.Context, approval *Approval) error
	FindByID(ctx context.Context, id int64) (*Approval, error)
	FindPending(ctx context.Context) ([]*Approval, error)
	FindByUploaderID(ctx context.Context, uploaderID int64) ([]*Approval, error)
	Update(ctx context.Context, approval *Approval) error
	Delete(ctx context.Context, id int64) error
	CountPending(ctx context.Context) (int64, error)
}
