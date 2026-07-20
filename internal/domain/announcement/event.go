package announcement

import "time"

type AnnouncementPublished struct {
	AnnID      int64
	Title      string
	AuthorID   int64
	Pinned     bool
	occurredAt time.Time
}

func NewAnnouncementPublished(annID int64, title string, authorID int64, pinned bool) *AnnouncementPublished {
	return &AnnouncementPublished{
		AnnID:      annID,
		Title:      title,
		AuthorID:   authorID,
		Pinned:     pinned,
		occurredAt: time.Now(),
	}
}

func (e *AnnouncementPublished) Type() string        { return "announcement.published" }
func (e *AnnouncementPublished) OccurredAt() time.Time { return e.occurredAt }
func (e *AnnouncementPublished) AggregateID() int64   { return e.AnnID }
