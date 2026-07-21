package invitation

import "errors"

var (
	ErrInvalidInvitationType = errors.New("invalid invitation type")
	ErrCodeNotFound          = errors.New("invitation code not found")
	ErrCodeExpired           = errors.New("invitation code has expired")
	ErrCodeAlreadyUsed       = errors.New("invitation code already used")
	ErrInvalidCode           = errors.New("invalid invitation code")
)
