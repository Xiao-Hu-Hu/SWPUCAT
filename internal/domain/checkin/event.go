package checkin

import "time"

type ClockedIn struct {
	RecordID   int64
	UserID     int64
	OccurredAt time.Time
}

func NewClockedIn(recordID, userID int64) *ClockedIn {
	return &ClockedIn{RecordID: recordID, UserID: userID, OccurredAt: time.Now()}
}

func (e *ClockedIn) Type() string        { return "checkin.clocked_in" }
func (e *ClockedIn) AggregateID() int64   { return e.RecordID }

type ClockedOut struct {
	RecordID   int64
	UserID     int64
	OccurredAt time.Time
}

func NewClockedOut(recordID, userID int64) *ClockedOut {
	return &ClockedOut{RecordID: recordID, UserID: userID, OccurredAt: time.Now()}
}

func (e *ClockedOut) Type() string        { return "checkin.clocked_out" }
func (e *ClockedOut) AggregateID() int64   { return e.RecordID }
