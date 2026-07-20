package knowledge

import "time"

type FileUploaded struct {
	ItemID       int64
	FileName     string
	UploaderID   int64
	NeedApproval bool
	OccurredAt   time.Time
}

func NewFileUploaded(itemID int64, fileName string, uploaderID int64, needApproval bool) *FileUploaded {
	return &FileUploaded{
		ItemID:       itemID,
		FileName:     fileName,
		UploaderID:   uploaderID,
		NeedApproval: needApproval,
		OccurredAt:   time.Now(),
	}
}

func (e *FileUploaded) Type() string        { return "knowledge.file_uploaded" }
func (e *FileUploaded) AggregateID() int64   { return e.ItemID }

type FileApproved struct {
	ItemID     int64
	ApprovedBy int64
	OccurredAt time.Time
}

func NewFileApproved(itemID, approvedBy int64) *FileApproved {
	return &FileApproved{ItemID: itemID, ApprovedBy: approvedBy, OccurredAt: time.Now()}
}

func (e *FileApproved) Type() string        { return "knowledge.file_approved" }
func (e *FileApproved) AggregateID() int64   { return e.ItemID }
