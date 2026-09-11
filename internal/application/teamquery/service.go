package teamquery

import (
	checkinapp "SWPUCAT/internal/application/checkin"
	reportapp "SWPUCAT/internal/application/report"
	"SWPUCAT/internal/domain/report"
	"SWPUCAT/internal/domain/shared"
	"context"
	"sort"
	"strings"
)

type Request struct {
	Period       string
	PeriodID     int64
	MinHours     *float64
	ReportStatus string
	Limit        int
}

type MemberResult struct {
	UserID          int64    `json:"user_id"`
	Nickname        string   `json:"nickname"`
	TotalHours      float64  `json:"total_hours"`
	Days            int      `json:"days"`
	AttendanceState string   `json:"attendance_state"`
	Anomalies       []string `json:"anomalies,omitempty"`
	ReportState     string   `json:"report_state,omitempty"`
	Reasons         []string `json:"reasons"`
}

type Result struct {
	Period       string         `json:"period"`
	PeriodID     int64          `json:"period_id,omitempty"`
	MinHours     *float64       `json:"min_hours,omitempty"`
	ReportStatus string         `json:"report_status,omitempty"`
	Limit        int            `json:"limit"`
	Members      []MemberResult `json:"members"`
}

type Service struct {
	checkin *checkinapp.CheckinService
	report  *reportapp.Service
}

func NewService(checkin *checkinapp.CheckinService, report *reportapp.Service) *Service {
	return &Service{checkin: checkin, report: report}
}

func (s *Service) Query(ctx context.Context, req Request) (*Result, error) {
	period := strings.TrimSpace(req.Period)
	if period == "" {
		period = "week"
	}
	if period != "today" && period != "week" && period != "month" || req.PeriodID < 0 || req.MinHours != nil && (*req.MinHours < 0 || *req.MinHours > 10000) {
		return nil, shared.ErrValidation
	}
	if req.ReportStatus != "" && req.ReportStatus != string(report.ReportStatusDraft) && req.ReportStatus != string(report.ReportStatusSubmitted) && req.ReportStatus != string(report.ReportStatusReviewed) && req.ReportStatus != "missing" {
		return nil, shared.ErrValidation
	}
	limit := req.Limit
	if limit < 1 || limit > 200 {
		limit = 100
	}
	stats, err := s.checkin.GetStatsByPeriod(ctx, period)
	if err != nil {
		return nil, err
	}
	reportStates := make(map[int64]string)
	if req.PeriodID > 0 && s.report != nil {
		reports, listErr := s.report.ListReports(ctx, 0, true, &req.PeriodID, "", 1, 200)
		if listErr != nil {
			return nil, listErr
		}
		for _, item := range reports {
			reportStates[item.AuthorID] = item.Status
		}
	}
	result := &Result{Period: period, PeriodID: req.PeriodID, MinHours: req.MinHours, ReportStatus: req.ReportStatus, Limit: limit, Members: make([]MemberResult, 0, limit)}
	for _, stat := range stats {
		state := reportStates[stat.UserID]
		if state == "" {
			state = "missing"
		}
		if req.ReportStatus != "" && state != req.ReportStatus {
			continue
		}
		if req.MinHours != nil && stat.TotalHours >= *req.MinHours {
			continue
		}
		reasons := make([]string, 0, 2)
		if req.MinHours != nil {
			reasons = append(reasons, "总工时低于查询阈值")
		}
		if state == "missing" {
			reasons = append(reasons, "月报尚未提交")
		}
		result.Members = append(result.Members, MemberResult{UserID: stat.UserID, Nickname: stat.Nickname, TotalHours: stat.TotalHours, Days: stat.Days, AttendanceState: stat.Status, Anomalies: stat.Anomalies, ReportState: state, Reasons: reasons})
	}
	sort.SliceStable(result.Members, func(i, j int) bool { return result.Members[i].UserID < result.Members[j].UserID })
	if len(result.Members) > limit {
		result.Members = result.Members[:limit]
	}
	return result, nil
}
