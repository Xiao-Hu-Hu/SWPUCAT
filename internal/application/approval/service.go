package approval

import (
	"SWPUCAT/internal/domain/approval"
	"SWPUCAT/internal/domain/knowledge"
	"SWPUCAT/internal/domain/shared"
	"context"
)

type ApprovalDTO struct {
	ID           int64  `json:"id"`
	FileName     string `json:"file_name"`
	FileSize     string `json:"file_size"`
	Description  string `json:"description,omitempty"`
	CategoryID   int64  `json:"category_id"`
	UploaderID   int64  `json:"uploader_id"`
	UploaderName string `json:"uploader_name"`
	Status       string `json:"status"`
	CreatedAt    string `json:"created_at"`
}

type ApprovalService struct {
	approvalRepo  approval.Repository
	knowledgeRepo knowledge.Repository
	publisher     shared.EventPublisher
}

func NewApprovalService(
	approvalRepo approval.Repository,
	knowledgeRepo knowledge.Repository,
	publisher shared.EventPublisher,
) *ApprovalService {
	return &ApprovalService{
		approvalRepo:  approvalRepo,
		knowledgeRepo: knowledgeRepo,
		publisher:     publisher,
	}
}

func (s *ApprovalService) Submit(ctx context.Context, fileName, fileSize, fileKey, description string, categoryID, uploaderID int64, uploaderName string) (*ApprovalDTO, error) {
	a := approval.NewApproval(fileName, fileSize, fileKey, description, categoryID, uploaderID, uploaderName)
	if err := s.approvalRepo.Create(ctx, a); err != nil {
		return nil, err
	}
	s.publisher.Publish(approval.NewApprovalSubmitted(a.ID, uploaderID, fileName))
	return toDTO(a), nil
}

func (s *ApprovalService) Approve(ctx context.Context, approvalID, reviewerID int64) error {
	a, err := s.approvalRepo.FindByID(ctx, approvalID)
	if err != nil {
		return err
	}
	if err := a.Approve(reviewerID); err != nil {
		return err
	}

	item, err := knowledge.NewFile(
		a.FileName, a.Description, a.FileSize, a.FileKey,
		a.CategoryID, a.UploaderID, a.UploaderName, true,
	)
	if err != nil {
		return err
	}
	if err := s.knowledgeRepo.CreateItem(ctx, item); err != nil {
		return err
	}

	if err := s.approvalRepo.Update(ctx, a); err != nil {
		return err
	}
	s.publisher.Publish(approval.NewApprovalReviewed(a.ID, reviewerID, true))
	return nil
}

func (s *ApprovalService) Reject(ctx context.Context, approvalID, reviewerID int64) error {
	a, err := s.approvalRepo.FindByID(ctx, approvalID)
	if err != nil {
		return err
	}
	if err := a.Reject(reviewerID); err != nil {
		return err
	}
	if err := s.approvalRepo.Update(ctx, a); err != nil {
		return err
	}
	s.publisher.Publish(approval.NewApprovalReviewed(a.ID, reviewerID, false))
	return nil
}

func (s *ApprovalService) ListPending(ctx context.Context) ([]ApprovalDTO, error) {
	approvals, err := s.approvalRepo.FindPending(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]ApprovalDTO, 0, len(approvals))
	for _, a := range approvals {
		result = append(result, *toDTO(a))
	}
	return result, nil
}

func toDTO(a *approval.Approval) *ApprovalDTO {
	return &ApprovalDTO{
		ID:           a.ID,
		FileName:     a.FileName,
		FileSize:     a.FileSize,
		Description:  a.Description,
		CategoryID:   a.CategoryID,
		UploaderID:   a.UploaderID,
		UploaderName: a.UploaderName,
		Status:       string(a.Status),
		CreatedAt:    a.CreatedAt.Format("2006-01-02"),
	}
}
