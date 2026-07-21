package invitation

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"
)

type InvitationType string

const (
	TypeCaptain InvitationType = "captain"
	TypeMember  InvitationType = "member"
)

func (t InvitationType) IsValid() bool {
	return t == TypeCaptain || t == TypeMember
}

type InvitationCode struct {
	ID        int64
	Code      string
	Type      InvitationType
	CreatorID int64
	UsedBy    *int64
	Used      bool
	ExpiresAt time.Time
	CreatedAt time.Time
}

func NewInvitationCode(invitationType InvitationType, creatorID int64) (*InvitationCode, error) {
	if !invitationType.IsValid() {
		return nil, ErrInvalidInvitationType
	}

	code, err := generateCode()
	if err != nil {
		return nil, err
	}

	return &InvitationCode{
		Code:      code,
		Type:      invitationType,
		CreatorID: creatorID,
		Used:      false,
		ExpiresAt: time.Now().Add(10 * time.Minute),
		CreatedAt: time.Now(),
	}, nil
}

func generateCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", fmt.Errorf("failed to generate code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func (ic *InvitationCode) IsValid() bool {
	return !ic.Used && time.Now().Before(ic.ExpiresAt)
}

func (ic *InvitationCode) Use(userID int64) error {
	if ic.Used {
		return ErrCodeAlreadyUsed
	}
	if time.Now().After(ic.ExpiresAt) {
		return ErrCodeExpired
	}
	ic.Used = true
	ic.UsedBy = &userID
	return nil
}
