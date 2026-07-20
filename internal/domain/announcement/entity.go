package announcement

import (
	"errors"
	"time"
)

var (
	ErrAnnouncementNotFound = errors.New("announcement not found")
	ErrEmptyTitle           = errors.New("announcement title cannot be empty")
)

type Announcement struct {
	ID         int64
	Title      string
	Content    string
	AuthorID   int64
	AuthorName string
	Pinned     bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func NewAnnouncement(title, content string, authorID int64, authorName string, pinned bool) (*Announcement, error) {
	if title == "" {
		return nil, ErrEmptyTitle
	}
	now := time.Now()
	return &Announcement{
		Title:      title,
		Content:    content,
		AuthorID:   authorID,
		AuthorName: authorName,
		Pinned:     pinned,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

func (a *Announcement) Pin() {
	a.Pinned = true
	a.UpdatedAt = time.Now()
}

func (a *Announcement) Unpin() {
	a.Pinned = false
	a.UpdatedAt = time.Now()
}

func (a *Announcement) Update(title, content string) error {
	if title == "" {
		return ErrEmptyTitle
	}
	a.Title = title
	a.Content = content
	a.UpdatedAt = time.Now()
	return nil
}
