package announcement

import (
	"context"
	"SWPUCAT/internal/domain/announcement"
	"SWPUCAT/internal/domain/shared"
	"time"
)

type CreateRequest struct {
	Title   string `json:"title" validate:"required,max=256"`
	Content string `json:"content" validate:"required"`
	Pinned  bool   `json:"pinned"`
}

type UpdateRequest struct {
	Title   string `json:"title" validate:"required,max=256"`
	Content string `json:"content" validate:"required"`
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
}

func NewAnnouncementService(annRepo announcement.Repository, publisher shared.EventPublisher) *AnnouncementService {
	return &AnnouncementService{annRepo: annRepo, publisher: publisher}
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
	result := make([]AnnouncementDTO, 0, len(anns))
	for _, a := range anns {
		result = append(result, *toDTO(a))
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
