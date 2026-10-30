package checkin

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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

type AnomalyNotifier interface {
	NotifyCheckinAnomaly(ctx context.Context, recipientID int64, scopeKey string, anomalies []string) error
}

type CheckinService struct {
	checkinRepo  checkin.Repository
	userRepo     user.Repository
	publisher    shared.EventPublisher
	settingsRepo SettingsRepository
	notifier     AnomalyNotifier
	now          func() time.Time
}

func NewCheckinService(
	checkinRepo checkin.Repository,
	userRepo user.Repository,
	publisher shared.EventPublisher,
	settingsRepo SettingsRepository,
	notifiers ...AnomalyNotifier,
) *CheckinService {
	service := &CheckinService{
		checkinRepo:  checkinRepo,
		userRepo:     userRepo,
		publisher:    publisher,
		settingsRepo: settingsRepo,
		now:          time.Now,
	}
	if len(notifiers) > 0 {
		service.notifier = notifiers[0]
	}
	return service
}

func (s *CheckinService) ClockIn(ctx context.Context, userID int64, idempotencyKey string) (*CheckinRecordDTO, error) {
	today := time.Now().Format("2006-01-02")

	record, err := checkin.NewCheckinRecord(userID, checkin.CheckinTypeIn)
	if err != nil {
		return nil, err
	}
	record.Date = today
	created, err := s.checkinRepo.CreateTransition(ctx, record, idempotencyKey)
	if err != nil {
		return nil, err
	}

	if created {
		s.publisher.Publish(checkin.NewClockedIn(record.ID, userID))
	}

	return &CheckinRecordDTO{
		ID:   record.ID,
		Type: string(record.Type),
		Date: record.Date,
		Time: record.Time,
	}, nil
}

func (s *CheckinService) ClockOut(ctx context.Context, userID int64, idempotencyKey string) (*CheckinRecordDTO, error) {
	today := time.Now().Format("2006-01-02")

	record, err := checkin.NewCheckinRecord(userID, checkin.CheckinTypeOut)
	if err != nil {
		return nil, err
	}
	record.Date = today
	created, err := s.checkinRepo.CreateTransition(ctx, record, idempotencyKey)
	if err != nil {
		return nil, err
	}

	if created {
		s.publisher.Publish(checkin.NewClockedOut(record.ID, userID))
	}

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
		// The repository returns newest records first. Keep that order and take
		// the newest page instead of accidentally returning the oldest tail.
		records = records[:limit]
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
	var lastInOut *checkin.CheckinRecord
	for _, r := range records {
		switch r.Type {
		case checkin.CheckinTypeIn:
			clockIn = r.Time
			lastInOut = r
		case checkin.CheckinTypeOut:
			clockOut = r.Time
			lastInOut = r
		}
	}

	if lastInOut == nil {
		return &CheckinStatusDTO{Status: "idle"}, nil
	}
	if lastInOut.Type == checkin.CheckinTypeIn {
		return &CheckinStatusDTO{Status: "clocked_in", ClockIn: clockIn}, nil
	}
	return &CheckinStatusDTO{Status: "clocked_out", ClockIn: clockIn, ClockOut: clockOut}, nil
}

func (s *CheckinService) GetStatsByPeriod(ctx context.Context, period string) ([]WeeklyStatsDTO, error) {
	now := s.now()
	startDate, endDate, valid := checkinPeriodDateRange(period, now)
	if !valid {
		// Preserve the existing public API behavior: an unknown value falls back
		// to the current week. New callers that require strict validation do so
		// before invoking this method.
		period = "week"
		startDate, endDate, _ = checkinPeriodDateRange(period, now)
	}

	records, err := s.checkinRepo.FindByDateRange(ctx, startDate, endDate)
	if err != nil {
		return nil, err
	}
	users, err := s.userRepo.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	// Initialize every active member so an absent member remains visible to the
	// captain instead of disappearing from the statistics result.
	statsMap := make(map[int64]*WeeklyStatsDTO, len(users))
	userRecords := make(map[int64][]*checkin.CheckinRecord)
	for _, u := range users {
		statsMap[u.ID] = &WeeklyStatsDTO{
			UserID:    u.ID,
			Nickname:  u.DisplayName(),
			Status:    "absent",
			Anomalies: []string{"no_checkin"},
		}
	}
	for _, r := range records {
		if _, ok := statsMap[r.UserID]; ok {
			userRecords[r.UserID] = append(userRecords[r.UserID], r)
		}
	}

	for userID, memberRecords := range userRecords {
		stat := summarizeCheckinRecords(userID, statsMap[userID].Nickname, memberRecords)
		statsMap[userID] = &stat
	}

	result := make([]WeeklyStatsDTO, 0, len(statsMap))
	for _, stat := range statsMap {
		result = append(result, *stat)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Nickname == result[j].Nickname {
			return result[i].UserID < result[j].UserID
		}
		return result[i].Nickname < result[j].Nickname
	})
	if s.notifier != nil {
		for _, stat := range result {
			anomalies := make([]string, 0, len(stat.Anomalies))
			for _, anomaly := range stat.Anomalies {
				// A missing record is not a violation without a configured
				// schedule; only notify for malformed or incomplete transitions.
				if anomaly != "no_checkin" {
					anomalies = append(anomalies, anomaly)
				}
			}
			if len(anomalies) == 0 {
				continue
			}
			if err := s.notifier.NotifyCheckinAnomaly(ctx, stat.UserID, period, anomalies); err != nil {
				log.Printf("check-in anomaly notification failed for user %d: %v", stat.UserID, err)
			}
		}
	}
	return result, nil
}

// GetPersonalWorkHours returns only the authenticated member's data. It reads
// the current month once and derives the current week from that user-scoped
// result, avoiding both a team scan and the notification side effects of the
// team statistics endpoint.
func (s *CheckinService) GetPersonalWorkHours(ctx context.Context, userID int64) (*PersonalWorkHoursDTO, error) {
	now := s.now()
	weekStart, weekEnd, _ := checkinPeriodDateRange("week", now)
	monthStart, monthEnd, _ := checkinPeriodDateRange("month", now)
	queryStart := monthStart
	if weekStart < queryStart {
		queryStart = weekStart
	}
	member, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	records, err := s.checkinRepo.FindByUserIDAndDateRange(ctx, userID, queryStart, monthEnd)
	if err != nil {
		return nil, err
	}
	weekRecords := make([]*checkin.CheckinRecord, 0, len(records))
	monthRecords := make([]*checkin.CheckinRecord, 0, len(records))
	for _, record := range records {
		if record.Date >= weekStart && record.Date <= weekEnd {
			weekRecords = append(weekRecords, record)
		}
		if record.Date >= monthStart && record.Date <= monthEnd {
			monthRecords = append(monthRecords, record)
		}
	}
	weekStats := summarizeCheckinRecords(userID, member.DisplayName(), weekRecords)
	monthStats := summarizeCheckinRecords(userID, member.DisplayName(), monthRecords)
	result := &PersonalWorkHoursDTO{
		AsOfDate: now.Format("2006-01-02"),
		UserID:   userID,
		Nickname: member.DisplayName(),
		Week:     toPersonalPeriodStats(weekStart, weekEnd, weekStats),
		Month:    toPersonalPeriodStats(monthStart, monthEnd, monthStats),
		WeeklyRequirement: WeeklyRequirementProgressDTO{
			Grade:         gradeFromStudentIDAt(string(member.StudentID), now),
			Status:        "unknown",
			UnknownReason: "weekly requirement is not configured for the member's grade",
		},
	}
	if result.WeeklyRequirement.Grade == 0 {
		result.WeeklyRequirement.UnknownReason = "student ID does not contain a valid enrollment year"
		return result, nil
	}
	for _, requirement := range s.GetRequirements(ctx).Requirements {
		if requirement.Grade != result.WeeklyRequirement.Grade || requirement.Minutes < 0 {
			continue
		}
		requiredMinutes := requirement.Minutes
		requiredHours := float64(requiredMinutes) / 60
		met := weekStats.TotalMinutes >= float64(requiredMinutes)
		shortfallMinutes := math.Max(0, float64(requiredMinutes)-weekStats.TotalMinutes)
		shortfallHours := shortfallMinutes / 60
		status := "below_target"
		if met {
			status = "met"
		}
		result.WeeklyRequirement.RequiredMinutes = &requiredMinutes
		result.WeeklyRequirement.RequiredHours = &requiredHours
		result.WeeklyRequirement.IsMet = &met
		result.WeeklyRequirement.ShortfallMinutes = &shortfallMinutes
		result.WeeklyRequirement.ShortfallHours = &shortfallHours
		result.WeeklyRequirement.Status = status
		result.WeeklyRequirement.UnknownReason = ""
	}
	return result, nil
}

func toPersonalPeriodStats(startsOn, endsOn string, stats WeeklyStatsDTO) PersonalPeriodStatsDTO {
	return PersonalPeriodStatsDTO{
		StartsOn:        startsOn,
		EndsOn:          endsOn,
		TotalMinutes:    stats.TotalMinutes,
		TotalHours:      stats.TotalHours,
		RecordDays:      stats.Days,
		AttendanceState: stats.Status,
		Anomalies:       append([]string{}, stats.Anomalies...),
	}
}

func checkinPeriodDateRange(period string, now time.Time) (string, string, bool) {
	endDate := now.Format("2006-01-02")
	switch period {
	case "today":
		return endDate, endDate, true
	case "week":
		weekday := now.Weekday()
		if weekday == time.Sunday {
			weekday = 7
		}
		return now.AddDate(0, 0, -int(weekday-1)).Format("2006-01-02"), endDate, true
	case "month":
		return now.AddDate(0, 0, -now.Day()+1).Format("2006-01-02"), endDate, true
	default:
		return "", "", false
	}
}

func summarizeCheckinRecords(userID int64, nickname string, records []*checkin.CheckinRecord) WeeklyStatsDTO {
	result := WeeklyStatsDTO{
		UserID:    userID,
		Nickname:  nickname,
		Status:    "absent",
		Anomalies: []string{"no_checkin"},
	}
	if len(records) == 0 {
		return result
	}

	dateRecords := make(map[string][]*checkin.CheckinRecord)
	for _, record := range records {
		dateRecords[record.Date] = append(dateRecords[record.Date], record)
	}
	var totalMinutes float64
	anomalySet := make(map[string]struct{})
	for _, dayRecords := range dateRecords {
		sort.SliceStable(dayRecords, func(i, j int) bool {
			if dayRecords[i].CreatedAt.Equal(dayRecords[j].CreatedAt) {
				return dayRecords[i].ID < dayRecords[j].ID
			}
			return dayRecords[i].CreatedAt.Before(dayRecords[j].CreatedAt)
		})
		var openIn *checkin.CheckinRecord
		for _, record := range dayRecords {
			switch record.Type {
			case checkin.CheckinTypeIn:
				if openIn != nil {
					anomalySet["duplicate_clock_in"] = struct{}{}
				}
				openIn = record
			case checkin.CheckinTypeOut:
				if openIn == nil {
					anomalySet["orphan_clock_out"] = struct{}{}
					continue
				}
				inTime, inErr := time.Parse("15:04:05", openIn.Time)
				outTime, outErr := time.Parse("15:04:05", record.Time)
				if inErr != nil || outErr != nil || outTime.Before(inTime) {
					anomalySet["invalid_time_pair"] = struct{}{}
				} else {
					totalMinutes += outTime.Sub(inTime).Minutes()
				}
				openIn = nil
			case checkin.CheckinTypeMakeup:
				totalMinutes += float64(record.Minutes)
			default:
				anomalySet["invalid_type"] = struct{}{}
			}
		}
		if openIn != nil {
			anomalySet["missing_clock_out"] = struct{}{}
		}
	}
	result.TotalMinutes = totalMinutes
	result.TotalHours = totalMinutes / 60
	result.Days = len(dateRecords)
	result.Anomalies = make([]string, 0, len(anomalySet))
	for anomaly := range anomalySet {
		result.Anomalies = append(result.Anomalies, anomaly)
	}
	sort.Strings(result.Anomalies)
	if len(result.Anomalies) == 0 {
		result.Status = "normal"
	} else {
		result.Status = "attention"
	}
	return result
}

func (s *CheckinService) AutoClockOut(ctx context.Context, date string) error {
	_, _, err := s.checkinRepo.RunAutoClockOut(ctx, date, "23:59:59")
	return err
}

func (s *CheckinService) GetOnlineMembers(ctx context.Context) ([]OnlineMemberDTO, error) {
	return s.getOnlineMembersAt(ctx, s.now().Format("2006-01-02"))
}

func (s *CheckinService) getOnlineMembersAt(ctx context.Context, date string) ([]OnlineMemberDTO, error) {
	users, err := s.userRepo.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]OnlineMemberDTO, 0, len(users))
	for _, u := range users {
		lastRecord, _ := s.checkinRepo.GetLastByUserAndDate(ctx, u.ID, date)
		online := lastRecord != nil && lastRecord.Type == checkin.CheckinTypeIn
		result = append(result, OnlineMemberDTO{
			UserID:   u.ID,
			Nickname: u.DisplayName(),
			Online:   online,
		})
	}
	return result, nil
}

func (s *CheckinService) GetOnlineSummary(ctx context.Context, viewerID int64, includeMemberNames bool) (*OnlineSummaryDTO, error) {
	if viewerID <= 0 {
		return nil, shared.ErrValidation
	}
	now := s.now()
	members, err := s.getOnlineMembersAt(ctx, now.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	result := &OnlineSummaryDTO{
		AsOfDate: now.Format("2006-01-02"), TotalMembers: len(members),
		OnlineMembers: make([]string, 0),
	}
	selfOnline := false
	selfFound := false
	for _, member := range members {
		if member.UserID == viewerID {
			selfOnline, selfFound = member.Online, true
		}
		if !member.Online {
			continue
		}
		result.OnlineCount++
		if includeMemberNames {
			result.OnlineMembers = append(result.OnlineMembers, member.Nickname)
		}
	}
	if selfFound {
		result.SelfOnline = &selfOnline
	}
	if includeMemberNames {
		sort.Strings(result.OnlineMembers)
		if len(result.OnlineMembers) > 50 {
			result.OnlineMembers = result.OnlineMembers[:50]
			result.Truncated = true
		}
	} else {
		result.OnlineMembers = nil
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
		if r.Type == checkin.CheckinTypeMakeup {
			dto.Minutes = r.Minutes
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
	if req.Minutes < 1 || req.Minutes > 480 {
		return fmt.Errorf("%w: minutes must be between 1 and 480", shared.ErrValidation)
	}

	u, err := s.userRepo.FindByID(ctx, req.UserID)
	if err != nil {
		return user.ErrUserNotFound
	}
	if !u.IsActive() {
		return shared.ErrForbidden
	}

	// Validate date format
	parsedDate, err := time.Parse("2006-01-02", req.Date)
	if err != nil || parsedDate.Format("2006-01-02") != req.Date {
		return fmt.Errorf("%w: date must use YYYY-MM-DD", shared.ErrValidation)
	}
	if req.Date > time.Now().Format("2006-01-02") {
		return fmt.Errorf("%w: date cannot be in the future", shared.ErrValidation)
	}

	// A makeup adds the requested work minutes directly to the member's weekly
	// total as a standalone 补签 record, with no clock-in/out time.
	record := &checkin.CheckinRecord{
		UserID:    req.UserID,
		Type:      checkin.CheckinTypeMakeup,
		Date:      req.Date,
		Time:      "",
		Minutes:   req.Minutes,
		CreatedAt: time.Now(),
	}
	return s.checkinRepo.CreateMakeup(ctx, record)
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
	return gradeFromStudentIDAt(studentID, time.Now())
}

func gradeFromStudentIDAt(studentID string, now time.Time) int {
	if len(studentID) < 4 {
		return 0
	}
	enrollmentYear, err := strconv.Atoi(studentID[:4])
	if err != nil {
		return 0
	}
	currentYear := now.Year()
	currentMonth := int(now.Month())
	grade := currentYear - enrollmentYear + 1
	if currentMonth < 9 {
		grade--
	}
	if grade < 1 {
		grade = 1
	}
	if grade > 4 {
		grade = 4
	}
	return grade
}
