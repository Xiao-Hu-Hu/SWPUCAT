package approval

import "time"

type Approval struct {
	ID           int64
	FileName     string
	FileSize     string
	FileKey      string
	CategoryID   int64
	UploaderID   int64
	UploaderName string
	Status       ApprovalStatus
	ReviewerID   *int64
	ReviewedAt   *time.Time
	CreatedAt    time.Time
}

func NewApproval(fileName, fileSize, fileKey string, categoryID, uploaderID int64, uploaderName string) *Approval {
	return &Approval{
		FileName:     fileName,
		FileSize:     fileSize,
		FileKey:      fileKey,
		CategoryID:   categoryID,
		UploaderID:   uploaderID,
		UploaderName: uploaderName,
		Status:       StatusPending,
		CreatedAt:    time.Now(),
	}
}

func (a *Approval) Approve(reviewerID int64) error {
	if !a.Status.IsPending() {
		return ErrAlreadyProcessed
	}
	a.Status = StatusApproved
	a.ReviewerID = &reviewerID
	now := time.Now()
	a.ReviewedAt = &now
	return nil
}

func (a *Approval) Reject(reviewerID int64) error {
	if !a.Status.IsPending() {
		return ErrAlreadyProcessed
	}
	a.Status = StatusRejected
	a.ReviewerID = &reviewerID
	now := time.Now()
	a.ReviewedAt = &now
	return nil
}
