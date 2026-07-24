package checkin

type WeeklyStatsDTO struct {
	UserID       int64   `json:"user_id"`
	Nickname     string  `json:"nickname"`
	TotalMinutes float64 `json:"total_minutes"`
	TotalHours   float64 `json:"total_hours"`
	Days         int     `json:"days"`
}

type OnlineMemberDTO struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
	Online   bool   `json:"online"`
}

type TodayRecordDTO struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"user_id"`
	Type        string `json:"type"`
	Date        string `json:"date"`
	Time        string `json:"time"`
	ClockInTime string `json:"clock_in_time,omitempty"`
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
