package checkin

import "errors"

type CheckinType string

const (
	CheckinTypeIn  CheckinType = "in"
	CheckinTypeOut CheckinType = "out"
	// CheckinTypeMakeup is a compensation record that adds a fixed amount of
	// work minutes rather than a live in/out session. It has no clock time.
	CheckinTypeMakeup CheckinType = "makeup"
)

func (t CheckinType) IsValid() bool {
	return t == CheckinTypeIn || t == CheckinTypeOut || t == CheckinTypeMakeup
}

var (
	ErrAlreadyClockedIn   = errors.New("already clocked in, please clock out first")
	ErrNotClockedIn       = errors.New("not clocked in, cannot clock out")
	ErrInvalidCheckinType = errors.New("invalid checkin type")
)
