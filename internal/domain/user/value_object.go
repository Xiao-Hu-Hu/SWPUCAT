package user

import (
	"errors"
	"regexp"
	"strings"
)

type Role string

const (
	RoleCaptain Role = "captain"
	RoleMember  Role = "member"
)

func (r Role) IsValid() bool {
	return r == RoleCaptain || r == RoleMember
}

func (r Role) IsCaptain() bool {
	return r == RoleCaptain
}

type Username string

var usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)

func NewUsername(raw string) (Username, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("username cannot be empty")
	}
	if !usernameRegex.MatchString(raw) {
		return "", errors.New("username must be 3-32 characters, alphanumeric and underscore only")
	}
	return Username(raw), nil
}

func (u Username) String() string {
	return string(u)
}

type StudentID string

var studentIDRegex = regexp.MustCompile(`^\d{12}$`)

func NewStudentID(raw string) (StudentID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("student ID cannot be empty")
	}
	if !studentIDRegex.MatchString(raw) {
		return "", errors.New("student ID must be exactly 12 digits (e.g., 202431060420)")
	}
	return StudentID(raw), nil
}

func (s StudentID) String() string {
	return string(s)
}

type Nickname string

func NewNickname(raw string) (Nickname, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("nickname cannot be empty")
	}
	if len([]rune(raw)) > 64 {
		return "", errors.New("nickname cannot exceed 64 characters")
	}
	return Nickname(raw), nil
}

func (n Nickname) String() string {
	return string(n)
}

type PasswordHash string

func (p PasswordHash) String() string {
	return string(p)
}
