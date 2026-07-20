package checkin

import (
	"time"
)

type CheckinRecord struct {
	ID        int64
	UserID    int64
	Type      CheckinType
	Date      string
	Time      string
	CreatedAt time.Time
}

func NewCheckinRecord(userID int64, checkinType CheckinType) (*CheckinRecord, error) {
	if !checkinType.IsValid() {
		return nil, ErrInvalidCheckinType
	}
	now := time.Now()
	return &CheckinRecord{
		UserID:    userID,
		Type:      checkinType,
		Date:      now.Format("2006-01-02"),
		Time:      now.Format("15:04:05"),
		CreatedAt: now,
	}, nil
}
