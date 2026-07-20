package approval

import "time"

type ApprovalSubmitted struct {
	ApprovalID int64
	UploaderID int64
	FileName   string
	OccurredAt time.Time
}

func NewApprovalSubmitted(approvalID, uploaderID int64, fileName string) *ApprovalSubmitted {
	return &ApprovalSubmitted{ApprovalID: approvalID, UploaderID: uploaderID, FileName: fileName, OccurredAt: time.Now()}
}

func (e *ApprovalSubmitted) Type() string        { return "approval.submitted" }
func (e *ApprovalSubmitted) AggregateID() int64   { return e.ApprovalID }

type ApprovalReviewed struct {
	ApprovalID int64
	ReviewerID int64
	Approved   bool
	OccurredAt time.Time
}

func NewApprovalReviewed(approvalID, reviewerID int64, approved bool) *ApprovalReviewed {
	return &ApprovalReviewed{ApprovalID: approvalID, ReviewerID: reviewerID, Approved: approved, OccurredAt: time.Now()}
}

func (e *ApprovalReviewed) Type() string        { return "approval.reviewed" }
func (e *ApprovalReviewed) AggregateID() int64   { return e.ApprovalID }
