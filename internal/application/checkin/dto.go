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
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	Type      string `json:"type"`
	Date      string `json:"date"`
	Time      string `json:"time"`
	Nickname  string `json:"nickname"`
	StudentID string `json:"student_id"`
	Avatar    string `json:"avatar"`
}
