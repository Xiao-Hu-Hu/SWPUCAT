package checkin

import (
	"context"
	"SWPUCAT/internal/domain/checkin"
	"SWPUCAT/internal/domain/shared"
	"SWPUCAT/internal/domain/user"
	"time"
)

type CheckinRecordDTO struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
	Date string `json:"date"`
	Time string `json:"time"`
}

type CheckinStatusDTO struct {
	Status   string `json:"status"`
	ClockIn  string `json:"clock_in,omitempty"`
	ClockOut string `json:"clock_out,omitempty"`
}

type CheckinService struct {
	checkinRepo checkin.Repository
	userRepo    user.Repository
	publisher   shared.EventPublisher
}

func NewCheckinService(
	checkinRepo checkin.Repository,
	userRepo user.Repository,
	publisher shared.EventPublisher,
) *CheckinService {
	return &CheckinService{
		checkinRepo: checkinRepo,
		userRepo:    userRepo,
		publisher:   publisher,
	}
}

func (s *CheckinService) ClockIn(ctx context.Context, userID int64) (*CheckinRecordDTO, error) {
	today := time.Now().Format("2006-01-02")

	lastRecord, _ := s.checkinRepo.GetLastByUserAndDate(ctx, userID, today)
	if lastRecord != nil && lastRecord.Type == checkin.CheckinTypeIn {
		return nil, checkin.ErrAlreadyClockedIn
	}

	record, err := checkin.NewCheckinRecord(userID, checkin.CheckinTypeIn)
	if err != nil {
		return nil, err
	}
	if err := s.checkinRepo.Create(ctx, record); err != nil {
		return nil, err
	}

	u, err := s.userRepo.FindByID(ctx, userID)
	if err == nil {
		u.IncrementCheckinCount()
		s.userRepo.Update(ctx, u)
	}

	s.publisher.Publish(checkin.NewClockedIn(record.ID, userID))

	return &CheckinRecordDTO{
		ID:   record.ID,
		Type: string(record.Type),
		Date: record.Date,
		Time: record.Time,
	}, nil
}

func (s *CheckinService) ClockOut(ctx context.Context, userID int64) (*CheckinRecordDTO, error) {
	today := time.Now().Format("2006-01-02")

	lastRecord, _ := s.checkinRepo.GetLastByUserAndDate(ctx, userID, today)
	if lastRecord == nil || lastRecord.Type != checkin.CheckinTypeIn {
		return nil, checkin.ErrNotClockedIn
	}

	record, err := checkin.NewCheckinRecord(userID, checkin.CheckinTypeOut)
	if err != nil {
		return nil, err
	}
	if err := s.checkinRepo.Create(ctx, record); err != nil {
		return nil, err
	}

	s.publisher.Publish(checkin.NewClockedOut(record.ID, userID))

	return &CheckinRecordDTO{
		ID:   record.ID,
		Type: string(record.Type),
		Date: record.Date,
		Time: record.Time,
	}, nil
}

func (s *CheckinService) GetRecords(ctx context.Context, userID int64, limit int) ([]CheckinRecordDTO, error) {
	records, err := s.checkinRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(records) > limit {
		records = records[len(records)-limit:]
	}
	result := make([]CheckinRecordDTO, 0, len(records))
	for _, r := range records {
		result = append(result, CheckinRecordDTO{
			ID:   r.ID,
			Type: string(r.Type),
			Date: r.Date,
			Time: r.Time,
		})
	}
	return result, nil
}

func (s *CheckinService) GetStatus(ctx context.Context, userID int64) (*CheckinStatusDTO, error) {
	today := time.Now().Format("2006-01-02")
	records, err := s.checkinRepo.FindByUserIDAndDate(ctx, userID, today)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return &CheckinStatusDTO{Status: "idle"}, nil
	}
	last := records[len(records)-1]
	if last.Type == checkin.CheckinTypeIn {
		return &CheckinStatusDTO{Status: "clocked_in", ClockIn: last.Time}, nil
	}
	return &CheckinStatusDTO{Status: "clocked_out", ClockOut: last.Time}, nil
}
