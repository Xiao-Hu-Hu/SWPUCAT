package announcementdraft

import (
	"errors"
	"strings"
	"time"
)

type Status string

const (
	StatusDraft     Status = "draft"
	StatusConfirmed Status = "confirmed"
	StatusPublished Status = "published"
	StatusDiscarded Status = "discarded"
)

var ErrInvalidDraft = errors.New("invalid announcement draft")

type Draft struct {
	ID               int64
	Status           Status
	Title            string
	Content          string
	Pinned           bool
	SourceJSON       string
	RecipientIDsJSON string
	EmailRequested   bool
	EmailResultJSON  string
	AnnouncementID   int64
	CreatedBy        int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
	ConfirmedAt      *time.Time
	PublishedAt      *time.Time
}

func New(createdBy int64, title, content, sourceJSON, recipientIDsJSON string, pinned, emailRequested bool) (*Draft, error) {
	if createdBy <= 0 || strings.TrimSpace(title) == "" || len([]rune(title)) > 256 || strings.TrimSpace(content) == "" || len([]rune(content)) > 120000 {
		return nil, ErrInvalidDraft
	}
	if sourceJSON == "" {
		sourceJSON = "{}"
	}
	if recipientIDsJSON == "" {
		recipientIDsJSON = "[]"
	}
	now := time.Now()
	return &Draft{Status: StatusDraft, Title: strings.TrimSpace(title), Content: content, Pinned: pinned, SourceJSON: sourceJSON, RecipientIDsJSON: recipientIDsJSON, EmailRequested: emailRequested, EmailResultJSON: "{}", CreatedBy: createdBy, CreatedAt: now, UpdatedAt: now}, nil
}

func (d *Draft) Confirm() error {
	if d == nil || d.Status != StatusDraft {
		return ErrInvalidDraft
	}
	now := time.Now()
	d.Status, d.ConfirmedAt, d.UpdatedAt = StatusConfirmed, &now, now
	return nil
}

func (d *Draft) Discard() error {
	if d == nil || d.Status == StatusPublished || d.Status == StatusDiscarded {
		return ErrInvalidDraft
	}
	d.Status = StatusDiscarded
	d.UpdatedAt = time.Now()
	return nil
}

func (d *Draft) MarkPublished(announcementID int64, emailResult string) error {
	if d == nil || d.Status != StatusConfirmed || announcementID <= 0 {
		return ErrInvalidDraft
	}
	now := time.Now()
	d.Status, d.AnnouncementID, d.EmailResultJSON, d.PublishedAt, d.UpdatedAt = StatusPublished, announcementID, emailResult, &now, now
	return nil
}
