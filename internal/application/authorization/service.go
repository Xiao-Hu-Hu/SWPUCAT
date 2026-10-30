package authorization

import (
	"context"

	"SWPUCAT/internal/domain/shared"
	"SWPUCAT/internal/domain/user"
)

type Actor struct {
	UserID      int64
	DisplayName string
	Role        user.Role
	AuthVersion int64
}

func (a Actor) HasAnyRole(roles ...user.Role) bool {
	for _, role := range roles {
		if a.Role == role {
			return true
		}
	}
	return false
}

type Service struct {
	users UserReader
}

type UserReader interface {
	FindByID(ctx context.Context, id int64) (*user.User, error)
}

func NewService(users UserReader) *Service {
	return &Service{users: users}
}

// ResolveActor reloads the current account on every authenticated request.
// JWT claims identify the session, while the database remains authoritative
// for account status, role and credential version.
func (s *Service) ResolveActor(ctx context.Context, userID, tokenAuthVersion int64) (Actor, error) {
	u, err := s.users.FindByID(ctx, userID)
	if err != nil || !u.IsActive() || u.AuthVersion != tokenAuthVersion || !u.Role.IsValid() {
		return Actor{}, shared.ErrUnauthorized
	}
	return Actor{
		UserID:      u.ID,
		DisplayName: string(u.Nickname),
		Role:        u.Role,
		AuthVersion: u.AuthVersion,
	}, nil
}
