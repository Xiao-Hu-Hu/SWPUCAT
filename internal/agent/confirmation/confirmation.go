package confirmation

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	ToolClockIn           = "clock_in"
	ToolClockOut          = "clock_out"
	ToolDraftAnnouncement = "draft_announcement"
	defaultTTL            = 5 * time.Minute
	maxTTL                = 15 * time.Minute
	maxTokenBytes         = 4096
	maxParameterBytes     = 256 << 10
)

var (
	ErrInvalidToken = errors.New("invalid action confirmation token")
	ErrExpiredToken = errors.New("action confirmation token expired")
)

type Proposal struct {
	Tool       string          `json:"tool"`
	Title      string          `json:"title"`
	Summary    string          `json:"summary"`
	Parameters json.RawMessage `json:"parameters"`
	Token      string          `json:"token,omitempty"`
	ExpiresAt  string          `json:"expires_at"`
}

type DraftAnnouncementParameters struct {
	Title        string  `json:"title"`
	Content      string  `json:"content"`
	Pinned       bool    `json:"pinned"`
	SourceNote   string  `json:"source_note,omitempty"`
	RecipientIDs []int64 `json:"recipient_ids,omitempty"`
}

type Claims struct {
	Version          int    `json:"v"`
	UserID           int64  `json:"uid"`
	SessionID        string `json:"sid"`
	Tool             string `json:"tool"`
	ParametersDigest string `json:"params"`
	IssuedAtUnix     int64  `json:"iat"`
	ExpiresAtUnix    int64  `json:"exp"`
	Nonce            string `json:"nonce"`
}

type Signer struct {
	key    []byte
	ttl    time.Duration
	now    func() time.Time
	random io.Reader
}

func NewSigner(secret string, ttl time.Duration) (*Signer, error) {
	if len(strings.TrimSpace(secret)) < 32 {
		return nil, errors.New("confirmation secret must be at least 32 characters")
	}
	if ttl <= 0 {
		ttl = defaultTTL
	}
	if ttl > maxTTL {
		return nil, errors.New("confirmation ttl must not exceed 15 minutes")
	}
	digest := sha256.Sum256([]byte("swpucat-agent-confirmation-v1\x00" + secret))
	return &Signer{key: digest[:], ttl: ttl, now: time.Now, random: rand.Reader}, nil
}

func (s *Signer) Propose(userID int64, sessionID, toolName, title, summary string, parameters any) (*Proposal, error) {
	if s == nil || userID <= 0 || strings.TrimSpace(sessionID) == "" || !SupportedTool(toolName) {
		return nil, ErrInvalidToken
	}
	canonical, err := CanonicalParameters(parameters)
	if err != nil {
		return nil, err
	}
	nonceBytes := make([]byte, 16)
	if _, err := io.ReadFull(s.random, nonceBytes); err != nil {
		return nil, fmt.Errorf("create confirmation nonce: %w", err)
	}
	now := s.now().UTC().Truncate(time.Second)
	claims := Claims{
		Version: 1, UserID: userID, SessionID: strings.TrimSpace(sessionID), Tool: toolName,
		ParametersDigest: ParametersDigest(canonical), IssuedAtUnix: now.Unix(), ExpiresAtUnix: now.Add(s.ttl).Unix(),
		Nonce: hex.EncodeToString(nonceBytes),
	}
	token, err := s.sign(claims)
	if err != nil {
		return nil, err
	}
	return &Proposal{
		Tool: toolName, Title: strings.TrimSpace(title), Summary: strings.TrimSpace(summary),
		Parameters: canonical, Token: token, ExpiresAt: time.Unix(claims.ExpiresAtUnix, 0).UTC().Format(time.RFC3339),
	}, nil
}

func (s *Signer) Verify(token string, userID int64, sessionID, toolName string, parameters json.RawMessage) (*Claims, json.RawMessage, error) {
	if s == nil || userID <= 0 || strings.TrimSpace(sessionID) == "" || !SupportedTool(toolName) || len(token) == 0 || len(token) > maxTokenBytes {
		return nil, nil, ErrInvalidToken
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, nil, ErrInvalidToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, ErrInvalidToken
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(signature) != sha256.Size {
		return nil, nil, ErrInvalidToken
	}
	expected := hmac.New(sha256.New, s.key)
	_, _ = expected.Write(payload)
	if !hmac.Equal(signature, expected.Sum(nil)) {
		return nil, nil, ErrInvalidToken
	}
	var claims Claims
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&claims); err != nil {
		return nil, nil, ErrInvalidToken
	}
	canonical, err := CanonicalParameters(parameters)
	if err != nil {
		return nil, nil, ErrInvalidToken
	}
	now := s.now().UTC()
	issuedAt, expiresAt := time.Unix(claims.IssuedAtUnix, 0), time.Unix(claims.ExpiresAtUnix, 0)
	if claims.Version != 1 || claims.UserID != userID || claims.SessionID != strings.TrimSpace(sessionID) || claims.Tool != toolName ||
		len(claims.Nonce) != 32 || claims.ParametersDigest != ParametersDigest(canonical) ||
		issuedAt.After(now.Add(time.Minute)) || !expiresAt.After(issuedAt) || expiresAt.Sub(issuedAt) > maxTTL {
		return nil, nil, ErrInvalidToken
	}
	if !expiresAt.After(now) {
		return nil, nil, ErrExpiredToken
	}
	return &claims, canonical, nil
}

func (s *Signer) sign(claims Claims) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func CanonicalParameters(parameters any) (json.RawMessage, error) {
	data, err := json.Marshal(parameters)
	if err != nil || len(data) == 0 || len(data) > maxParameterBytes {
		return nil, errors.New("invalid action parameters")
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, errors.New("invalid action parameters")
	}
	canonical, err := json.Marshal(value)
	if err != nil || len(canonical) > maxParameterBytes {
		return nil, errors.New("invalid action parameters")
	}
	return canonical, nil
}

func ParametersDigest(parameters json.RawMessage) string {
	digest := sha256.Sum256(parameters)
	return hex.EncodeToString(digest[:])
}

func TokenDigest(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func SupportedTool(toolName string) bool {
	switch toolName {
	case ToolClockIn, ToolClockOut, ToolDraftAnnouncement:
		return true
	default:
		return false
	}
}
