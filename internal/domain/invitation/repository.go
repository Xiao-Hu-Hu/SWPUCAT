package invitation

import "context"

type Repository interface {
	Create(ctx context.Context, code *InvitationCode) error
	FindByCode(ctx context.Context, code string) (*InvitationCode, error)
	FindByCreatorID(ctx context.Context, creatorID int64) ([]*InvitationCode, error)
	Update(ctx context.Context, code *InvitationCode) error
	DeleteExpired(ctx context.Context) error
}
