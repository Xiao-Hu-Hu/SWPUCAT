package authorization

import (
	"context"
	"errors"
	"testing"

	"SWPUCAT/internal/domain/shared"
	"SWPUCAT/internal/domain/user"
)

type fakeUserReader struct {
	user *user.User
	err  error
}

func (f fakeUserReader) FindByID(context.Context, int64) (*user.User, error) {
	return f.user, f.err
}

func TestResolveActorUsesCurrentAccountState(t *testing.T) {
	account := &user.User{
		ID:          42,
		Nickname:    user.Nickname("测试成员"),
		Role:        user.RoleCaptain,
		Status:      user.StatusActive,
		AuthVersion: 3,
	}
	service := NewService(fakeUserReader{user: account})

	actor, err := service.ResolveActor(context.Background(), 42, 3)
	if err != nil {
		t.Fatalf("ResolveActor returned error: %v", err)
	}
	if actor.UserID != 42 || actor.Role != user.RoleCaptain || actor.DisplayName != "测试成员" {
		t.Fatalf("unexpected actor: %#v", actor)
	}
}

func TestResolveActorRejectsRevokedOrInactiveAccount(t *testing.T) {
	tests := []struct {
		name         string
		account      *user.User
		tokenVersion int64
	}{
		{
			name:         "version mismatch",
			account:      &user.User{ID: 1, Role: user.RoleMember, Status: user.StatusActive, AuthVersion: 2},
			tokenVersion: 1,
		},
		{
			name:         "inactive account",
			account:      &user.User{ID: 1, Role: user.RoleMember, Status: user.StatusDisabled, AuthVersion: 2},
			tokenVersion: 2,
		},
		{
			name:         "invalid role",
			account:      &user.User{ID: 1, Role: user.Role("unknown"), Status: user.StatusActive, AuthVersion: 2},
			tokenVersion: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(fakeUserReader{user: tt.account})
			_, err := service.ResolveActor(context.Background(), 1, tt.tokenVersion)
			if !errors.Is(err, shared.ErrUnauthorized) {
				t.Fatalf("expected unauthorized, got %v", err)
			}
		})
	}
}

func TestResolveActorRejectsMissingAccount(t *testing.T) {
	service := NewService(fakeUserReader{err: errors.New("not found")})
	_, err := service.ResolveActor(context.Background(), 1, 1)
	if !errors.Is(err, shared.ErrUnauthorized) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
}
