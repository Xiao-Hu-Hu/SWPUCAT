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
