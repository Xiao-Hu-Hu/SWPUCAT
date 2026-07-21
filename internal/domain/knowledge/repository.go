package knowledge

import "context"

type Repository interface {
	CreateItem(ctx context.Context, item *KnowledgeItem) error
	FindItemByID(ctx context.Context, id int64) (*KnowledgeItem, error)
	FindItems(ctx context.Context, filter ItemFilter) ([]*KnowledgeItem, error)
	FindItemsByUploader(ctx context.Context, uploaderID int64) ([]*KnowledgeItem, error)
	UpdateItem(ctx context.Context, item *KnowledgeItem) error
	DeleteItem(ctx context.Context, id int64) error
	CountItems(ctx context.Context) (int64, error)

	CreateCategory(ctx context.Context, cat *Category) error
	FindAllCategories(ctx context.Context) ([]*Category, error)
	FindCategoryByID(ctx context.Context, id int64) (*Category, error)
	DeleteCategory(ctx context.Context, id int64) error
	CountItemsByCategory(ctx context.Context, categoryID int64) (int64, error)
}

type ItemFilter struct {
	CategoryID *int64
	Search     string
	Approved   *bool
	Page       int
	PageSize   int
}
