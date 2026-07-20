package knowledge

import (
	"context"
	"SWPUCAT/internal/domain/knowledge"
	"SWPUCAT/internal/domain/shared"
	"time"
)

type CreateLinkRequest struct {
	Name       string `json:"name" validate:"required,max=256"`
	URL        string `json:"url" validate:"required,url"`
	CategoryID int64  `json:"category_id" validate:"required"`
}

type UploadFileRequest struct {
	FileName   string `json:"file_name" validate:"required,max=256"`
	FileSize   string `json:"file_size"`
	FileKey    string `json:"file_key"`
	CategoryID int64  `json:"category_id" validate:"required"`
}

type CreateCategoryRequest struct {
	Name string `json:"name" validate:"required,max=64"`
}

type KnowledgeItemDTO struct {
	ID           int64  `json:"id"`
	Type         string `json:"type"`
	Name         string `json:"name"`
	URL          string `json:"url,omitempty"`
	FileKey      string `json:"file_key,omitempty"`
	FileSize     string `json:"file_size,omitempty"`
	CategoryID   int64  `json:"category_id"`
	CategoryName string `json:"category_name"`
	UploaderID   int64  `json:"uploader_id"`
	UploaderName string `json:"uploader_name"`
	Approved     bool   `json:"approved"`
	CreatedAt    string `json:"created_at"`
}

type CategoryDTO struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	IsSystem bool   `json:"is_system"`
	Count    int64  `json:"count"`
}

type KnowledgeService struct {
	repo      knowledge.Repository
	publisher shared.EventPublisher
}

func NewKnowledgeService(repo knowledge.Repository, publisher shared.EventPublisher) *KnowledgeService {
	return &KnowledgeService{repo: repo, publisher: publisher}
}

func (s *KnowledgeService) CreateLink(ctx context.Context, uploaderID int64, uploaderName string, req CreateLinkRequest) (*KnowledgeItemDTO, error) {
	item, err := knowledge.NewLink(req.Name, req.URL, req.CategoryID, uploaderID, uploaderName)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateItem(ctx, item); err != nil {
		return nil, err
	}
	return toItemDTO(item, ""), nil
}

func (s *KnowledgeService) UploadFile(ctx context.Context, uploaderID int64, uploaderName string, isCaptain bool, req UploadFileRequest) (*KnowledgeItemDTO, error) {
	approved := isCaptain
	item, err := knowledge.NewFile(req.FileName, req.FileSize, req.FileKey, req.CategoryID, uploaderID, uploaderName, approved)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateItem(ctx, item); err != nil {
		return nil, err
	}
	s.publisher.Publish(knowledge.NewFileUploaded(item.ID, item.Name, item.UploaderID, !approved))
	return toItemDTO(item, ""), nil
}

func (s *KnowledgeService) DeleteItem(ctx context.Context, itemID, userID int64, isCaptain bool) error {
	item, err := s.repo.FindItemByID(ctx, itemID)
	if err != nil {
		return err
	}
	if !isCaptain && !item.IsOwnedBy(userID) {
		return shared.ErrForbidden
	}
	return s.repo.DeleteItem(ctx, itemID)
}

func (s *KnowledgeService) ListItems(ctx context.Context, categoryID *int64, search string) ([]KnowledgeItemDTO, error) {
	trueVal := true
	items, err := s.repo.FindItems(ctx, knowledge.ItemFilter{
		CategoryID: categoryID,
		Search:     search,
		Approved:   &trueVal,
	})
	if err != nil {
		return nil, err
	}
	result := make([]KnowledgeItemDTO, 0, len(items))
	for _, item := range items {
		result = append(result, *toItemDTO(item, ""))
	}
	return result, nil
}

func (s *KnowledgeService) CreateCategory(ctx context.Context, name string) (*CategoryDTO, error) {
	cat := &knowledge.Category{Name: name, CreatedAt: time.Now()}
	if err := s.repo.CreateCategory(ctx, cat); err != nil {
		return nil, err
	}
	return &CategoryDTO{ID: cat.ID, Name: cat.Name, IsSystem: false}, nil
}

func (s *KnowledgeService) DeleteCategory(ctx context.Context, catID int64) error {
	cat, err := s.repo.FindCategoryByID(ctx, catID)
	if err != nil {
		return err
	}
	if cat.IsSystem {
		return knowledge.ErrCategoryInUse
	}
	count, _ := s.repo.CountItemsByCategory(ctx, catID)
	if count > 0 {
		return knowledge.ErrCategoryInUse
	}
	return s.repo.DeleteCategory(ctx, catID)
}

func (s *KnowledgeService) ListCategories(ctx context.Context) ([]CategoryDTO, error) {
	cats, err := s.repo.FindAllCategories(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]CategoryDTO, 0, len(cats))
	for _, c := range cats {
		count, _ := s.repo.CountItemsByCategory(ctx, c.ID)
		result = append(result, CategoryDTO{
			ID:       c.ID,
			Name:     c.Name,
			IsSystem: c.IsSystem,
			Count:    count,
		})
	}
	return result, nil
}

func (s *KnowledgeService) GetItem(ctx context.Context, id int64) (*KnowledgeItemDTO, error) {
	item, err := s.repo.FindItemByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return toItemDTO(item, ""), nil
}

func toItemDTO(item *knowledge.KnowledgeItem, catName string) *KnowledgeItemDTO {
	return &KnowledgeItemDTO{
		ID:           item.ID,
		Type:         string(item.Type),
		Name:         item.Name,
		URL:          item.URL,
		FileKey:      item.FileKey,
		FileSize:     item.FileSize,
		CategoryID:   item.CategoryID,
		CategoryName: catName,
		UploaderID:   item.UploaderID,
		UploaderName: item.UploaderName,
		Approved:     item.Approved,
		CreatedAt:    item.CreatedAt.Format("2006-01-02"),
	}
}
