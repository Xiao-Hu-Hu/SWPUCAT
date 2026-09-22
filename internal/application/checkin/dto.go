package checkin

type WeeklyStatsDTO struct {
	UserID       int64    `json:"user_id"`
	Nickname     string   `json:"nickname"`
	TotalMinutes float64  `json:"total_minutes"`
	TotalHours   float64  `json:"total_hours"`
	Days         int      `json:"days"`
	Status       string   `json:"status"`
	Anomalies    []string `json:"anomalies,omitempty"`
}

type PersonalPeriodStatsDTO struct {
	StartsOn        string   `json:"starts_on"`
	EndsOn          string   `json:"ends_on"`
	TotalMinutes    float64  `json:"total_minutes"`
	TotalHours      float64  `json:"total_hours"`
	RecordDays      int      `json:"record_days"`
	AttendanceState string   `json:"attendance_state"`
	Anomalies       []string `json:"anomalies"`
}

type WeeklyRequirementProgressDTO struct {
	Grade            int      `json:"grade,omitempty"`
	RequiredMinutes  *int     `json:"required_minutes,omitempty"`
	RequiredHours    *float64 `json:"required_hours,omitempty"`
	IsMet            *bool    `json:"is_met,omitempty"`
	ShortfallMinutes *float64 `json:"shortfall_minutes,omitempty"`
	ShortfallHours   *float64 `json:"shortfall_hours,omitempty"`
	Status           string   `json:"status"`
	UnknownReason    string   `json:"unknown_reason,omitempty"`
}

// PersonalWorkHoursDTO contains only the authenticated member's data. The
// configured target is weekly, so no unsupported monthly target is inferred.
type PersonalWorkHoursDTO struct {
	AsOfDate          string                       `json:"as_of_date"`
	UserID            int64                        `json:"user_id"`
	Nickname          string                       `json:"nickname"`
	Week              PersonalPeriodStatsDTO       `json:"week"`
	Month             PersonalPeriodStatsDTO       `json:"month"`
	WeeklyRequirement WeeklyRequirementProgressDTO `json:"weekly_requirement"`
}

type OnlineMemberDTO struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
	Online   bool   `json:"online"`
}

// OnlineSummaryDTO is the privacy-minimized projection used by the Agent.
// Members receive only aggregate/self state; managers may also receive a
// bounded list of display names. IDs, tokens, IPs and activity trails are
// deliberately absent.
type OnlineSummaryDTO struct {
	AsOfDate      string   `json:"as_of_date"`
	TotalMembers  int      `json:"total_members"`
	OnlineCount   int      `json:"online_count"`
	SelfOnline    *bool    `json:"self_online,omitempty"`
	OnlineMembers []string `json:"online_members,omitempty"`
	Truncated     bool     `json:"truncated,omitempty"`
}

type TodayRecordDTO struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"user_id"`
	Type        string `json:"type"`
	Date        string `json:"date"`
	Time        string `json:"time"`
	ClockInTime string `json:"clock_in_time,omitempty"`
	Minutes     int    `json:"minutes,omitempty"`
	Nickname    string `json:"nickname"`
	StudentID   string `json:"student_id"`
	Avatar      string `json:"avatar"`
}

type MakeupRequest struct {
	UserID  int64  `json:"user_id" validate:"required"`
	Date    string `json:"date" validate:"required"`
	Minutes int    `json:"minutes" validate:"required,min=1,max=480"`
}

type CheckinRequirement struct {
	Grade   int `json:"grade"`
	Minutes int `json:"minutes"`
}

type RequirementsDTO struct {
	Requirements []CheckinRequirement `json:"requirements"`
}
