package checkin

import "errors"

type CheckinType string

const (
	CheckinTypeIn  CheckinType = "in"
	CheckinTypeOut CheckinType = "out"
)

func (t CheckinType) IsValid() bool {
	return t == CheckinTypeIn || t == CheckinTypeOut
}

var (
	ErrAlreadyClockedIn   = errors.New("already clocked in, please clock out first")
	ErrNotClockedIn       = errors.New("not clocked in, cannot clock out")
	ErrInvalidCheckinType = errors.New("invalid checkin type")
)
