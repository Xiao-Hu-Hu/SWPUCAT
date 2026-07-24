package announcement

import (
	"context"
	"fmt"
	"log"
	"sync"

	"SWPUCAT/internal/domain/announcement"
	"SWPUCAT/internal/domain/shared"
	"SWPUCAT/internal/domain/user"
	"time"
)

type EmailService interface {
	SendAnnouncementNotification(to string, title string, content string) error
}

type CreateRequest struct {
	Title   string `json:"title" validate:"required,max=256"`
	Content string `json:"content" validate:"required"`
	Pinned  bool   `json:"pinned"`
}

type UpdateRequest struct {
	Title   string `json:"title" validate:"required,max=256"`
	Content string `json:"content" validate:"required"`
	Pinned  *bool  `json:"pinned,omitempty"`
}

type AnnouncementDTO struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	AuthorID   int64  `json:"author_id"`
	AuthorName string `json:"author_name"`
	Pinned     bool   `json:"pinned"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type AnnouncementService struct {
	annRepo   announcement.Repository
	publisher shared.EventPublisher
	userRepo  user.Repository
	emailSvc  EmailService
}

func NewAnnouncementService(annRepo announcement.Repository, publisher shared.EventPublisher, userRepo user.Repository, emailSvc EmailService) *AnnouncementService {
	return &AnnouncementService{annRepo: annRepo, publisher: publisher, userRepo: userRepo, emailSvc: emailSvc}
}

func (s *AnnouncementService) Create(ctx context.Context, operatorID int64, operatorName string, req CreateRequest) (*AnnouncementDTO, error) {
	ann, err := announcement.NewAnnouncement(req.Title, req.Content, operatorID, operatorName, req.Pinned)
	if err != nil {
		return nil, err
	}
	if err := s.annRepo.Create(ctx, ann); err != nil {
		return nil, err
	}
	s.publisher.Publish(announcement.NewAnnouncementPublished(ann.ID, ann.Title, ann.AuthorID, ann.Pinned))
	return toDTO(ann), nil
}

func (s *AnnouncementService) Update(ctx context.Context, annID int64, req UpdateRequest) error {
	ann, err := s.annRepo.FindByID(ctx, annID)
	if err != nil {
		return err
	}
	if err := ann.Update(req.Title, req.Content); err != nil {
		return err
	}
	if req.Pinned != nil {
		if *req.Pinned {
			ann.Pin()
		} else {
			ann.Unpin()
		}
	}
	return s.annRepo.Update(ctx, ann)
}

func (s *AnnouncementService) Delete(ctx context.Context, annID int64) error {
	return s.annRepo.Delete(ctx, annID)
}

func (s *AnnouncementService) List(ctx context.Context) ([]AnnouncementDTO, error) {
	anns, err := s.annRepo.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	// Collect author IDs that need name resolution
	authorIDs := make(map[int64]bool)
	for _, a := range anns {
		if a.AuthorName == "" {
			authorIDs[a.AuthorID] = true
		}
	}

	// Batch fetch authors
	authorNames := make(map[int64]string)
	if len(authorIDs) > 0 {
		ids := make([]int64, 0, len(authorIDs))
		for id := range authorIDs {
			ids = append(ids, id)
		}
		users, _ := s.userRepo.FindByIDs(ctx, ids)
		for _, u := range users {
			authorNames[u.ID] = u.DisplayName()
		}
	}

	result := make([]AnnouncementDTO, 0, len(anns))
	for _, a := range anns {
		dto := toDTO(a)
		if dto.AuthorName == "" {
			if name, ok := authorNames[a.AuthorID]; ok {
				dto.AuthorName = name
			}
		}
		result = append(result, *dto)
	}
	return result, nil
}

func toDTO(a *announcement.Announcement) *AnnouncementDTO {
	return &AnnouncementDTO{
		ID:         a.ID,
		Title:      a.Title,
		Content:    a.Content,
		AuthorID:   a.AuthorID,
		AuthorName: a.AuthorName,
		Pinned:     a.Pinned,
		CreatedAt:  a.CreatedAt.Format(time.DateTime),
		UpdatedAt:  a.UpdatedAt.Format(time.DateTime),
	}
}

func (s *AnnouncementService) NotifyMembers(ctx context.Context, annID int64, userIDs []int64) (int, error) {
	ann, err := s.annRepo.FindByID(ctx, annID)
	if err != nil {
		return 0, fmt.Errorf("announcement not found")
	}

	users, err := s.userRepo.FindByIDs(ctx, userIDs)
	if err != nil {
		return 0, err
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	successCount := 0

	for _, u := range users {
		if u.Email == "" {
			continue
		}
		wg.Add(1)
		go func(email string) {
			defer wg.Done()
			if err := s.emailSvc.SendAnnouncementNotification(email, ann.Title, ann.Content); err != nil {
				log.Printf("[ERROR] Failed to send announcement notification to %s: %v", email, err)
				return
			}
			mu.Lock()
			successCount++
			mu.Unlock()
		}(string(u.Email))
	}

	wg.Wait()
	return successCount, nil
}
