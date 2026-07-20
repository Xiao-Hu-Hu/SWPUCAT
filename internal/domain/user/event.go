package user

import "time"

type UserRegistered struct {
	UserID     int64
	Username   string
	Nickname   string
	occurredAt time.Time
}

func NewUserRegistered(userID int64, username, nickname string) *UserRegistered {
	return &UserRegistered{
		UserID:     userID,
		Username:   username,
		Nickname:   nickname,
		occurredAt: time.Now(),
	}
}

func (e *UserRegistered) Type() string        { return "user.registered" }
func (e *UserRegistered) OccurredAt() time.Time { return e.occurredAt }
func (e *UserRegistered) AggregateID() int64   { return e.UserID }

type CaptainTransferred struct {
	FromUserID int64
	ToUserID   int64
	occurredAt time.Time
}

func NewCaptainTransferred(fromID, toID int64) *CaptainTransferred {
	return &CaptainTransferred{
		FromUserID: fromID,
		ToUserID:   toID,
		occurredAt: time.Now(),
	}
}

func (e *CaptainTransferred) Type() string        { return "user.captain_transferred" }
func (e *CaptainTransferred) OccurredAt() time.Time { return e.occurredAt }
func (e *CaptainTransferred) AggregateID() int64   { return e.ToUserID }

type MemberRemoved struct {
	UserID     int64
	RemovedBy  int64
	occurredAt time.Time
}

func NewMemberRemoved(userID, removedBy int64) *MemberRemoved {
	return &MemberRemoved{
		UserID:     userID,
		RemovedBy:  removedBy,
		occurredAt: time.Now(),
	}
}

func (e *MemberRemoved) Type() string        { return "user.member_removed" }
func (e *MemberRemoved) OccurredAt() time.Time { return e.occurredAt }
func (e *MemberRemoved) AggregateID() int64   { return e.UserID }
