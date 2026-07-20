package dashboard

import (
	"context"
	"SWPUCAT/internal/domain/announcement"
	"SWPUCAT/internal/domain/checkin"
	"SWPUCAT/internal/domain/knowledge"
	"SWPUCAT/internal/domain/user"
	"time"
)

type DashboardDTO struct {
	MemberCount       int64             `json:"member_count"`
	TodayCheckins     int64             `json:"today_checkins"`
	AnnouncementCount int64             `json:"announcement_count"`
	KnowledgeCount    int64             `json:"knowledge_count"`
	RecentActivities  []ActivityDTO     `json:"recent_activities"`
	OnlineMembers     []OnlineMemberDTO `json:"online_members"`
}

type ActivityDTO struct {
	Text  string `json:"text"`
	Color string `json:"color"`
	Time  string `json:"time"`
}

type OnlineMemberDTO struct {
	ID       int64  `json:"id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

type DashboardService struct {
	userRepo    user.Repository
	checkinRepo checkin.Repository
	annRepo     announcement.Repository
	knowRepo    knowledge.Repository
}

func NewDashboardService(
	userRepo user.Repository,
	checkinRepo checkin.Repository,
	annRepo announcement.Repository,
	knowRepo knowledge.Repository,
) *DashboardService {
	return &DashboardService{
		userRepo:    userRepo,
		checkinRepo: checkinRepo,
		annRepo:     annRepo,
		knowRepo:    knowRepo,
	}
}

func (s *DashboardService) GetDashboard(ctx context.Context) (*DashboardDTO, error) {
	today := time.Now().Format("2006-01-02")

	memberCount, _ := s.userRepo.Count(ctx)
	todayCheckins, _ := s.checkinRepo.CountByDate(ctx, today)
	annCount, _ := s.annRepo.Count(ctx)
	knowledgeCount, _ := s.knowRepo.CountItems(ctx)

	var activities []ActivityDTO
	anns, _ := s.annRepo.FindAll(ctx)
	for i, a := range anns {
		if i >= 3 {
			break
		}
		activities = append(activities, ActivityDTO{
			Text:  "公告：" + a.Title,
			Color: "warning",
			Time:  a.CreatedAt.Format("2006-01-02 15:04"),
		})
	}

	todayRecords, _ := s.checkinRepo.FindByDate(ctx, today)
	checkedIn := make(map[int64]bool)
	checkedOut := make(map[int64]bool)
	for _, r := range todayRecords {
		if r.Type == checkin.CheckinTypeIn {
			checkedIn[r.UserID] = true
		} else {
			checkedOut[r.UserID] = true
		}
	}
	var onlineIDs []int64
	for id := range checkedIn {
		if !checkedOut[id] {
			onlineIDs = append(onlineIDs, id)
		}
	}
	var onlineMembers []OnlineMemberDTO
	if len(onlineIDs) > 0 {
		onlineUsers, _ := s.userRepo.FindByIDs(ctx, onlineIDs)
		for _, u := range onlineUsers {
			nickname := string(u.Nickname)
			avatar := ""
			if len(nickname) > 0 {
				avatar = string([]rune(nickname)[0])
			}
			onlineMembers = append(onlineMembers, OnlineMemberDTO{
				ID:       u.ID,
				Nickname: nickname,
				Avatar:   avatar,
			})
		}
	}

	return &DashboardDTO{
		MemberCount:       memberCount,
		TodayCheckins:     todayCheckins,
		AnnouncementCount: annCount,
		KnowledgeCount:    knowledgeCount,
		RecentActivities:  activities,
		OnlineMembers:     onlineMembers,
	}, nil
}
