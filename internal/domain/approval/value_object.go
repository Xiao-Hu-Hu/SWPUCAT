package approval

import "errors"

type ApprovalStatus string

const (
	StatusPending  ApprovalStatus = "pending"
	StatusApproved ApprovalStatus = "approved"
	StatusRejected ApprovalStatus = "rejected"
)

func (s ApprovalStatus) IsValid() bool {
	return s == StatusPending || s == StatusApproved || s == StatusRejected
}

func (s ApprovalStatus) IsPending() bool {
	return s == StatusPending
}

var (
	ErrApprovalNotFound = errors.New("approval not found")
	ErrAlreadyProcessed = errors.New("approval already processed")
)
