package announcement

import "time"

type AnnouncementPublished struct {
	AnnID      int64
	Title      string
	AuthorID   int64
	Pinned     bool
	OccurredAt time.Time
}

func NewAnnouncementPublished(annID int64, title string, authorID int64, pinned bool) *AnnouncementPublished {
	return &AnnouncementPublished{
		AnnID:      annID,
		Title:      title,
		AuthorID:   authorID,
		Pinned:     pinned,
		OccurredAt: time.Now(),
	}
}

func (e *AnnouncementPublished) Type() string        { return "announcement.published" }
func (e *AnnouncementPublished) AggregateID() int64   { return e.AnnID }
