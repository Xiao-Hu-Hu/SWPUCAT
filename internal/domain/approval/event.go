package approval

import "time"

type ApprovalSubmitted struct {
	ApprovalID int64
	UploaderID int64
	FileName   string
	occurredAt time.Time
}

func NewApprovalSubmitted(approvalID, uploaderID int64, fileName string) *ApprovalSubmitted {
	return &ApprovalSubmitted{ApprovalID: approvalID, UploaderID: uploaderID, FileName: fileName, occurredAt: time.Now()}
}

func (e *ApprovalSubmitted) Type() string        { return "approval.submitted" }
func (e *ApprovalSubmitted) OccurredAt() time.Time { return e.occurredAt }
func (e *ApprovalSubmitted) AggregateID() int64   { return e.ApprovalID }

type ApprovalReviewed struct {
	ApprovalID int64
	ReviewerID int64
	Approved   bool
	occurredAt time.Time
}

func NewApprovalReviewed(approvalID, reviewerID int64, approved bool) *ApprovalReviewed {
	return &ApprovalReviewed{ApprovalID: approvalID, ReviewerID: reviewerID, Approved: approved, occurredAt: time.Now()}
}

func (e *ApprovalReviewed) Type() string        { return "approval.reviewed" }
func (e *ApprovalReviewed) OccurredAt() time.Time { return e.occurredAt }
func (e *ApprovalReviewed) AggregateID() int64   { return e.ApprovalID }
