package user

import (
	"errors"
	"time"
)

var (
	ErrUserNotFound         = errors.New("user not found")
	ErrUsernameExists       = errors.New("username already exists")
	ErrStudentIDExists      = errors.New("student ID already exists")
	ErrInvalidPassword      = errors.New("invalid password")
	ErrInvalidCredentials   = errors.New("invalid credentials")
	ErrCannotRemoveCaptain  = errors.New("cannot remove captain")
	ErrCannotTransferToSelf = errors.New("cannot transfer captain to self")
	ErrNotCaptain           = errors.New("user is not captain")
)

type User struct {
	ID            int64
	Username      Username
	StudentID     StudentID
	Email         string
	Nickname      Nickname
	Avatar        string
	TechDirection string
	PasswordHash  PasswordHash
	Role          Role
	JoinedAt      time.Time
	CheckinCount  int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func NewUser(username Username, nickname Nickname, passwordHash PasswordHash) *User {
	now := time.Now()
	return &User{
		Username:     username,
		Nickname:     nickname,
		PasswordHash: passwordHash,
		Role:         RoleMember,
		JoinedAt:     now,
		CheckinCount: 0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func NewUserWithStudentID(studentID StudentID, email string, nickname Nickname, passwordHash PasswordHash) *User {
	now := time.Now()
	return &User{
		Username:     Username(studentID.String()),
		StudentID:    studentID,
		Email:        email,
		Nickname:     nickname,
		PasswordHash: passwordHash,
		Role:         RoleMember,
		JoinedAt:     now,
		CheckinCount: 0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func (u *User) TransferCaptainTo(target *User) error {
	if u.Role != RoleCaptain {
		return ErrNotCaptain
	}
	if u.ID == target.ID {
		return ErrCannotTransferToSelf
	}
	if target.Role == RoleCaptain {
		return errors.New("target is already captain")
	}
	u.Role = RoleMember
	u.UpdatedAt = time.Now()
	target.Role = RoleCaptain
	target.UpdatedAt = time.Now()
	return nil
}

func (u *User) IncrementCheckinCount() {
	u.CheckinCount++
	u.UpdatedAt = time.Now()
}

func (u *User) IsCaptain() bool {
	return u.Role.IsCaptain()
}

func (u *User) DisplayName() string {
	return u.Nickname.String()
}
