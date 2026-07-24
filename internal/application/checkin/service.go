package checkin

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"SWPUCAT/internal/domain/checkin"
	"SWPUCAT/internal/domain/shared"
	"SWPUCAT/internal/domain/user"
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

type SettingsRepository interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}

type CheckinService struct {
	checkinRepo  checkin.Repository
	userRepo     user.Repository
	publisher    shared.EventPublisher
	settingsRepo SettingsRepository
}

func NewCheckinService(
	checkinRepo checkin.Repository,
	userRepo user.Repository,
	publisher shared.EventPublisher,
	settingsRepo SettingsRepository,
) *CheckinService {
	return &CheckinService{
		checkinRepo:  checkinRepo,
		userRepo:     userRepo,
		publisher:    publisher,
		settingsRepo: settingsRepo,
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

	// Find clock_in and clock_out times
	var clockIn, clockOut string
	for _, r := range records {
		if r.Type == checkin.CheckinTypeIn {
			clockIn = r.Time
		} else if r.Type == checkin.CheckinTypeOut {
			clockOut = r.Time
		}
	}

	last := records[len(records)-1]
	if last.Type == checkin.CheckinTypeIn {
		return &CheckinStatusDTO{Status: "clocked_in", ClockIn: clockIn}, nil
	}
	return &CheckinStatusDTO{Status: "clocked_out", ClockIn: clockIn, ClockOut: clockOut}, nil
}

func (s *CheckinService) GetStatsByPeriod(ctx context.Context, period string) ([]WeeklyStatsDTO, error) {
	now := time.Now()
	var startDate, endDate string

	switch period {
	case "today":
		startDate = now.Format("2006-01-02")
		endDate = startDate
	case "month":
		startDate = now.AddDate(0, 0, -now.Day()+1).Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	default: // week
		weekday := now.Weekday()
		if weekday == time.Sunday {
			weekday = 7
		}
		startDate = now.AddDate(0, 0, -int(weekday-1)).Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	}

	records, err := s.checkinRepo.FindByDateRange(ctx, startDate, endDate)
	if err != nil {
		return nil, err
	}

	// Group records by user (skip invalid user_id 0)
	type userRecords struct {
		records []*checkin.CheckinRecord
	}
	userMap := make(map[int64]*userRecords)
	for _, r := range records {
		if r.UserID == 0 {
			continue
		}
		if _, ok := userMap[r.UserID]; !ok {
			userMap[r.UserID] = &userRecords{}
		}
		userMap[r.UserID].records = append(userMap[r.UserID].records, r)
	}

	// Calculate total time per user
	statsMap := make(map[int64]*WeeklyStatsDTO)
	for userID, ur := range userMap {
		var totalMinutes float64
		days := make(map[string]bool)

		// Group by date
		dateRecords := make(map[string][]*checkin.CheckinRecord)
		for _, r := range ur.records {
			dateRecords[r.Date] = append(dateRecords[r.Date], r)
		}

		for date, dayRecords := range dateRecords {
			days[date] = true
			// Find in/out pairs
			var lastIn *checkin.CheckinRecord
			for _, dr := range dayRecords {
				if dr.Type == checkin.CheckinTypeIn {
					lastIn = dr
				} else if dr.Type == checkin.CheckinTypeOut && lastIn != nil {
					inTime, _ := time.Parse("15:04:05", lastIn.Time)
					outTime, _ := time.Parse("15:04:05", dr.Time)
					totalMinutes += outTime.Sub(inTime).Minutes()
					lastIn = nil
				}
			}
		}

		statsMap[userID] = &WeeklyStatsDTO{
			UserID:       userID,
			TotalMinutes: totalMinutes,
			TotalHours:   totalMinutes / 60,
			Days:         len(days),
		}
	}

	// Fetch user info
	userIDs := make([]int64, 0, len(statsMap))
	for uid := range statsMap {
		userIDs = append(userIDs, uid)
	}
	users, err := s.userRepo.FindByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		if stat, ok := statsMap[u.ID]; ok {
			stat.Nickname = u.DisplayName()
		}
	}

	result := make([]WeeklyStatsDTO, 0, len(statsMap))
	for _, stat := range statsMap {
		result = append(result, *stat)
	}
	return result, nil
}

func (s *CheckinService) AutoClockOut(ctx context.Context, date string) error {
	records, err := s.checkinRepo.FindByDate(ctx, date)
	if err != nil {
		return err
	}

	userLastRecord := make(map[int64]*checkin.CheckinRecord)
	for _, r := range records {
		if r.UserID == 0 {
			continue
		}
		userLastRecord[r.UserID] = r
	}

	for userID, lastRecord := range userLastRecord {
		if lastRecord.Type == checkin.CheckinTypeIn {
			record, err := checkin.NewCheckinRecord(userID, checkin.CheckinTypeOut)
			if err != nil {
				continue
			}
			record.Time = "23:59:59"
			record.Date = date
			s.checkinRepo.Create(ctx, record)
		}
	}
	return nil
}

func (s *CheckinService) GetOnlineMembers(ctx context.Context) ([]OnlineMemberDTO, error) {
	today := time.Now().Format("2006-01-02")

	users, err := s.userRepo.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]OnlineMemberDTO, 0, len(users))
	for _, u := range users {
		lastRecord, _ := s.checkinRepo.GetLastByUserAndDate(ctx, u.ID, today)
		online := lastRecord != nil && lastRecord.Type == checkin.CheckinTypeIn
		result = append(result, OnlineMemberDTO{
			UserID:   u.ID,
			Nickname: u.DisplayName(),
			Online:   online,
		})
	}
	return result, nil
}

func (s *CheckinService) GetAllTodayRecords(ctx context.Context) ([]TodayRecordDTO, error) {
	today := time.Now().Format("2006-01-02")

	records, err := s.checkinRepo.FindByDate(ctx, today)
	if err != nil {
		return nil, err
	}

	userIDSet := make(map[int64]bool)
	for _, r := range records {
		if r.UserID > 0 {
			userIDSet[r.UserID] = true
		}
	}
	userIDs := make([]int64, 0, len(userIDSet))
	for uid := range userIDSet {
		userIDs = append(userIDs, uid)
	}

	users, err := s.userRepo.FindByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	userMap := make(map[int64]*user.User)
	for _, u := range users {
		userMap[u.ID] = u
	}

	// Group records by user and pair in/out in chronological order
	type recWithIdx struct {
		rec *checkin.CheckinRecord
		idx int
	}
	userRecs := make(map[int64][]recWithIdx)
	for i, r := range records {
		userRecs[r.UserID] = append(userRecs[r.UserID], recWithIdx{rec: r, idx: i})
	}

	// clockInTime for each "out" record, keyed by record index
	clockInForOut := make(map[int]int)
	for _, recs := range userRecs {
		// Sort by time ascending to pair correctly
		for i := 1; i < len(recs); i++ {
			for j := i; j > 0 && recs[j].rec.Time < recs[j-1].rec.Time; j-- {
				recs[j], recs[j-1] = recs[j-1], recs[j]
			}
		}
		var lastInIdx int = -1
		for _, wr := range recs {
			if wr.rec.Type == checkin.CheckinTypeIn {
				lastInIdx = wr.idx
			} else if wr.rec.Type == checkin.CheckinTypeOut && lastInIdx >= 0 {
				clockInForOut[wr.idx] = lastInIdx
				lastInIdx = -1
			}
		}
	}

	result := make([]TodayRecordDTO, 0, len(records))
	for i, r := range records {
		if r.UserID == 0 {
			continue
		}
		dto := TodayRecordDTO{
			ID:     r.ID,
			UserID: r.UserID,
			Type:   string(r.Type),
			Date:   r.Date,
			Time:   r.Time,
		}
		if r.Type == checkin.CheckinTypeOut {
			if inIdx, ok := clockInForOut[i]; ok {
				dto.ClockInTime = records[inIdx].Time
			}
		}
		if u, ok := userMap[r.UserID]; ok {
			dto.Nickname = u.DisplayName()
			dto.StudentID = string(u.StudentID)
			dto.Avatar = u.Avatar
		}
		result = append(result, dto)
	}
	return result, nil
}

func (s *CheckinService) Makeup(ctx context.Context, req MakeupRequest) error {
	// Validate user exists
	_, err := s.userRepo.FindByID(ctx, req.UserID)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	// Validate date format
	if _, err := time.Parse("2006-01-02", req.Date); err != nil {
		return fmt.Errorf("invalid date format, use YYYY-MM-DD")
	}

	// Create in record
	inRecord := &checkin.CheckinRecord{
		UserID:    req.UserID,
		Type:      checkin.CheckinTypeIn,
		Date:      req.Date,
		Time:      "08:00:00",
		CreatedAt: time.Now(),
	}
	if err := s.checkinRepo.Create(ctx, inRecord); err != nil {
		return err
	}

	// Calculate out time
	inTime, _ := time.Parse("15:04:05", "08:00:00")
	outTime := inTime.Add(time.Duration(req.Minutes) * time.Minute)

	outRecord := &checkin.CheckinRecord{
		UserID:    req.UserID,
		Type:      checkin.CheckinTypeOut,
		Date:      req.Date,
		Time:      outTime.Format("15:04:05"),
		CreatedAt: time.Now(),
	}
	return s.checkinRepo.Create(ctx, outRecord)
}

func (s *CheckinService) GetRequirements(ctx context.Context) RequirementsDTO {
	defaults := RequirementsDTO{
		Requirements: []CheckinRequirement{
			{Grade: 1, Minutes: 600},
			{Grade: 2, Minutes: 480},
			{Grade: 3, Minutes: 360},
			{Grade: 4, Minutes: 240},
		},
	}

	val, err := s.settingsRepo.Get(ctx, "checkin_requirements")
	if err != nil || val == "" {
		return defaults
	}

	var result RequirementsDTO
	if err := json.Unmarshal([]byte(val), &result); err != nil {
		return defaults
	}
	if len(result.Requirements) == 0 {
		return defaults
	}
	return result
}

func (s *CheckinService) SetRequirements(ctx context.Context, req RequirementsDTO) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return s.settingsRepo.Set(ctx, "checkin_requirements", string(data))
}

func (s *CheckinService) PublishReport(ctx context.Context) (string, string, error) {
	// Get weekly stats
	stats, err := s.GetStatsByPeriod(ctx, "week")
	if err != nil {
		return "", "", err
	}

	// Get all users for student IDs
	users, err := s.userRepo.FindAll(ctx)
	if err != nil {
		return "", "", err
	}
	userMap := make(map[int64]*user.User)
	for _, u := range users {
		userMap[u.ID] = u
	}

	// Get requirements
	requirements := s.GetRequirements(ctx)
	reqMap := make(map[int]int)
	for _, r := range requirements.Requirements {
		reqMap[r.Grade] = r.Minutes
	}

	// Build report rows
	now := time.Now()
	weekday := now.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	weekStart := now.AddDate(0, 0, -int(weekday-1)).Format("2006-01-02")
	weekEnd := now.Format("2006-01-02")

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## 本周打卡报告（%s ~ %s）\n\n", weekStart, weekEnd))
	sb.WriteString("| 姓名 | 学号 | 年级 | 打卡时长 | 要求时长 | 状态 |\n")
	sb.WriteString("|------|------|------|----------|----------|------|\n")

	// Sort stats by nickname for consistent output
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Nickname < stats[j].Nickname
	})

	for _, stat := range stats {
		u := userMap[stat.UserID]
		studentID := ""
		grade := 0
		if u != nil {
			studentID = string(u.StudentID)
			grade = gradeFromStudentID(studentID)
		}

		requiredMin := reqMap[grade]
		totalMin := stat.TotalMinutes
		shortfall := float64(requiredMin) - totalMin

		status := "达标"
		if shortfall > 0 {
			status = fmt.Sprintf("-%.1fh", math.Round(shortfall*10)/10/60)
		}

		sb.WriteString(fmt.Sprintf("| %s | %s | %d | %.1fh | %.1fh | %s |\n",
			stat.Nickname,
			studentID,
			grade,
			math.Round(totalMin*10)/10/60,
			math.Round(float64(requiredMin)*10)/10/60,
			status,
		))
	}

	title := fmt.Sprintf("本周打卡报告（%s ~ %s）", weekStart, weekEnd)
	return title, sb.String(), nil
}

func gradeFromStudentID(studentID string) int {
	if len(studentID) < 4 {
		return 0
	}
	enrollmentYear, err := strconv.Atoi(studentID[:4])
	if err != nil {
		return 0
	}
	currentYear := time.Now().Year()
	grade := currentYear - enrollmentYear + 1
	if grade < 1 {
		grade = 1
	}
	if grade > 4 {
		grade = 4
	}
	return grade
}
