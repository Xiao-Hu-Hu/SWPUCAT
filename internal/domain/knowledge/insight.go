package knowledge

import (
	"errors"
	"strings"
	"time"
)

type InsightStatus string

const (
	InsightProposed    InsightStatus = "proposed"
	InsightPublished   InsightStatus = "published"
	InsightInvalidated InsightStatus = "invalidated"
)

var ErrInvalidInsight = errors.New("invalid knowledge insight")

type Insight struct {
	ID                int64
	KnowledgeItemID   int64
	SourceFingerprint string
	Status            InsightStatus
	SummaryMarkdown   string
	TagsJSON          string
	Model             string
	PromptVersion     string
	ReviewDueAt       *time.Time
	CreatedBy         int64
	ReviewedBy        int64
	ReviewedAt        *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type RecommendationFeedback struct {
	UserID          int64
	KnowledgeItemID int64
	Helpful         bool
	Note            string
	CreatedAt       time.Time
}

func NewInsight(itemID, createdBy int64, fingerprint, summary, tagsJSON, model, promptVersion string, reviewDueAt *time.Time) (*Insight, error) {
	if itemID <= 0 || createdBy <= 0 || strings.TrimSpace(fingerprint) == "" || strings.TrimSpace(summary) == "" || len([]rune(summary)) > 12000 {
		return nil, ErrInvalidInsight
	}
	if tagsJSON == "" {
		tagsJSON = "[]"
	}
	now := time.Now()
	return &Insight{KnowledgeItemID: itemID, SourceFingerprint: strings.TrimSpace(fingerprint), Status: InsightProposed, SummaryMarkdown: summary, TagsJSON: tagsJSON, Model: strings.TrimSpace(model), PromptVersion: strings.TrimSpace(promptVersion), ReviewDueAt: reviewDueAt, CreatedBy: createdBy, CreatedAt: now, UpdatedAt: now}, nil
}

func (i *Insight) Publish(reviewerID int64) error {
	if i == nil || i.Status != InsightProposed || reviewerID <= 0 {
		return ErrInvalidInsight
	}
	now := time.Now()
	i.Status, i.ReviewedBy, i.ReviewedAt, i.UpdatedAt = InsightPublished, reviewerID, &now, now
	return nil
}

func (i *Insight) Invalidate() error {
	if i == nil || i.Status == InsightInvalidated {
		return ErrInvalidInsight
	}
	i.Status = InsightInvalidated
	i.UpdatedAt = time.Now()
	return nil
}
