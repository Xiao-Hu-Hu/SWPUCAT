package database

import "time"

type AnnouncementDraftModel struct {
	ID               int64     `gorm:"primaryKey;autoIncrement"`
	Status           string    `gorm:"size:16;not null;default:draft;index"`
	Title            string    `gorm:"size:256;not null"`
	Content          string    `gorm:"type:text;not null"`
	Pinned           bool      `gorm:"not null;default:false"`
	SourceJSON       string    `gorm:"column:source_json;type:jsonb;not null;default:'{}'"`
	RecipientIDsJSON string    `gorm:"column:recipient_ids_json;type:jsonb;not null;default:'[]'"`
	EmailRequested   bool      `gorm:"not null;default:false"`
	EmailResultJSON  string    `gorm:"column:email_result_json;type:jsonb;not null;default:'{}'"`
	AnnouncementID   *int64    `gorm:"index"`
	CreatedBy        int64     `gorm:"not null;index"`
	CreatedAt        time.Time `gorm:"not null;default:current_timestamp"`
	UpdatedAt        time.Time `gorm:"not null;default:current_timestamp"`
	ConfirmedAt      *time.Time
	PublishedAt      *time.Time
}

func (AnnouncementDraftModel) TableName() string { return "announcement_drafts" }

type AnnouncementDraftEventModel struct {
	ID         int64     `gorm:"primaryKey;autoIncrement"`
	DraftID    int64     `gorm:"not null;index"`
	ActorID    int64     `gorm:"not null"`
	EventType  string    `gorm:"size:32;not null"`
	DetailJSON string    `gorm:"column:detail_json;type:jsonb;not null;default:'{}'"`
	CreatedAt  time.Time `gorm:"not null;default:current_timestamp"`
}

func (AnnouncementDraftEventModel) TableName() string { return "announcement_draft_events" }
