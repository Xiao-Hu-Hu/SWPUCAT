package auth

import (
	"errors"
	"testing"
	"time"

	"SWPUCAT/internal/infrastructure/config"
)

func newTestJWTService() *JWTService {
	return NewJWTService(&config.JWTConfig{
		AccessSecret:  "test-access-secret-with-at-least-32-characters",
		RefreshSecret: "test-refresh-secret-with-at-least-32-characters",
		AccessExpiry:  time.Hour,
		RefreshExpiry: 24 * time.Hour,
	})
}

func TestAccessTokenCarriesCredentialVersion(t *testing.T) {
	service := newTestJWTService()
	token, _, err := service.GenerateAccessToken(7, "成员", "member", "202431060001", 4)
	if err != nil {
		t.Fatalf("GenerateAccessToken returned error: %v", err)
	}
	claims, err := service.ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken returned error: %v", err)
	}
	if claims.UserID != 7 || claims.AuthVersion != 4 || claims.TokenType != "access" {
		t.Fatalf("unexpected claims: %#v", claims)
	}
}

func TestRefreshTokenCannotBeUsedAsAccessToken(t *testing.T) {
	service := newTestJWTService()
	token, err := service.GenerateRefreshToken(7, 4)
	if err != nil {
		t.Fatalf("GenerateRefreshToken returned error: %v", err)
	}
	if _, err := service.ParseToken(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected invalid token, got %v", err)
	}

	userID, authVersion, err := service.ParseRefreshToken(token)
	if err != nil {
		t.Fatalf("ParseRefreshToken returned error: %v", err)
	}
	if userID != 7 || authVersion != 4 {
		t.Fatalf("unexpected refresh claims: user=%d version=%d", userID, authVersion)
	}
}
