package knowledge

import "context"
import "time"

type Repository interface {
	CreateItem(ctx context.Context, item *KnowledgeItem) error
	FindItemByID(ctx context.Context, id int64) (*KnowledgeItem, error)
	FindItems(ctx context.Context, filter ItemFilter) ([]*KnowledgeItem, error)
	FindItemsByUploader(ctx context.Context, uploaderID int64) ([]*KnowledgeItem, error)
	UpdateItem(ctx context.Context, item *KnowledgeItem, expectedStatus ItemStatus) error
	UpdateLifecycle(ctx context.Context, item *KnowledgeItem, expectedStatus ItemStatus, eventType string, actorID int64, actorName, reason string) error
	DeleteItem(ctx context.Context, id, deletedBy int64) error
	CountItems(ctx context.Context) (int64, error)

	CreateCategory(ctx context.Context, cat *Category) error
	FindAllCategories(ctx context.Context) ([]*Category, error)
	FindCategoryByID(ctx context.Context, id int64) (*Category, error)
	DeleteCategory(ctx context.Context, id int64) error
	CountItemsByCategory(ctx context.Context, categoryID int64) (int64, error)

	CreateInsight(ctx context.Context, insight *Insight) error
	FindInsightByItemID(ctx context.Context, itemID int64) (*Insight, error)
	FindInsightByID(ctx context.Context, insightID int64) (*Insight, error)
	UpdateInsight(ctx context.Context, insight *Insight, expectedStatus InsightStatus) error
	ListDueInsights(ctx context.Context, now time.Time, page, pageSize int) ([]*Insight, error)
	SaveRecommendationFeedback(ctx context.Context, feedback *RecommendationFeedback) error
}

type ItemFilter struct {
	CategoryID *int64
	Search     string
	Status     *ItemStatus
	Approved   *bool
	Page       int
	PageSize   int
}
