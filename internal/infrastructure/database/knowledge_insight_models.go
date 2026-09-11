package database

import "time"

type KnowledgeItemInsightModel struct {
	ID                int64      `gorm:"primaryKey;autoIncrement"`
	KnowledgeItemID   int64      `gorm:"not null;index"`
	SourceFingerprint string     `gorm:"size:128;not null"`
	Status            string     `gorm:"size:16;not null;default:proposed;index"`
	SummaryMarkdown   string     `gorm:"type:text;not null;default:''"`
	TagsJSON          string     `gorm:"type:jsonb;not null;default:'[]'"`
	Model             string     `gorm:"size:128;not null;default:''"`
	PromptVersion     string     `gorm:"size:64;not null;default:''"`
	ReviewDueAt       *time.Time `gorm:"index"`
	CreatedBy         int64      `gorm:"not null"`
	ReviewedBy        *int64     `gorm:"index"`
	ReviewedAt        *time.Time
	CreatedAt         time.Time `gorm:"not null;default:current_timestamp"`
	UpdatedAt         time.Time `gorm:"not null;default:current_timestamp"`
}

func (KnowledgeItemInsightModel) TableName() string { return "knowledge_item_insights" }

type KnowledgeRecommendationFeedbackModel struct {
	UserID          int64     `gorm:"primaryKey"`
	KnowledgeItemID int64     `gorm:"primaryKey"`
	Helpful         bool      `gorm:"not null"`
	Note            string    `gorm:"size:512;not null;default:''"`
	CreatedAt       time.Time `gorm:"not null;default:current_timestamp"`
}

func (KnowledgeRecommendationFeedbackModel) TableName() string {
	return "knowledge_recommendation_feedback"
}
