package auth

import (
	"SWPUCAT/internal/infrastructure/config"
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token has expired")
)

type JWTService struct {
	accessSecret  []byte
	refreshSecret []byte
	accessExpiry  time.Duration
	refreshExpiry time.Duration
}

func NewJWTService(cfg *config.JWTConfig) *JWTService {
	return &JWTService{
		accessSecret:  []byte(cfg.AccessSecret),
		refreshSecret: []byte(cfg.RefreshSecret),
		accessExpiry:  cfg.AccessExpiry,
		refreshExpiry: cfg.RefreshExpiry,
	}
}

type Claims struct {
	UserID      int64  `json:"user_id"`
	Username    string `json:"username"`
	StudentID   string `json:"student_id,omitempty"`
	Role        string `json:"role"`
	AuthVersion int64  `json:"auth_version"`
	TokenType   string `json:"token_type"`
	jwt.RegisteredClaims
}

func (s *JWTService) GenerateAccessToken(userID int64, username, role, studentID string, authVersion int64) (string, int64, error) {
	expiresAt := time.Now().Add(s.accessExpiry)
	claims := Claims{
		UserID:      userID,
		Username:    username,
		StudentID:   studentID,
		Role:        role,
		AuthVersion: authVersion,
		TokenType:   "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   strconv.FormatInt(userID, 10),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.accessSecret)
	if err != nil {
		return "", 0, err
	}
	return tokenString, expiresAt.Unix(), nil
}

func (s *JWTService) GenerateRefreshToken(userID, authVersion int64) (string, error) {
	claims := Claims{
		UserID:      userID,
		AuthVersion: authVersion,
		TokenType:   "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.refreshExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   strconv.FormatInt(userID, 10),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.refreshSecret)
}

func (s *JWTService) ParseToken(tokenString string) (*Claims, error) {
	return parseToken(tokenString, s.accessSecret, "access")
}

func (s *JWTService) ParseRefreshToken(tokenString string) (int64, int64, error) {
	claims, err := parseToken(tokenString, s.refreshSecret, "refresh")
	if err != nil {
		return 0, 0, err
	}
	return claims.UserID, claims.AuthVersion, nil
}

func parseToken(tokenString string, secret []byte, expectedType string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, ErrInvalidToken
		}
		return secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid || claims.TokenType != expectedType || claims.UserID <= 0 || claims.AuthVersion <= 0 {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
