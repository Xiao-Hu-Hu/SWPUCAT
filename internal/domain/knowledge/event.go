package knowledge

import "time"

type FileUploaded struct {
	ItemID       int64
	FileName     string
	UploaderID   int64
	NeedApproval bool
	occurredAt   time.Time
}

func NewFileUploaded(itemID int64, fileName string, uploaderID int64, needApproval bool) *FileUploaded {
	return &FileUploaded{
		ItemID:       itemID,
		FileName:     fileName,
		UploaderID:   uploaderID,
		NeedApproval: needApproval,
		occurredAt:   time.Now(),
	}
}

func (e *FileUploaded) Type() string        { return "knowledge.file_uploaded" }
func (e *FileUploaded) OccurredAt() time.Time { return e.occurredAt }
func (e *FileUploaded) AggregateID() int64   { return e.ItemID }

type FileApproved struct {
	ItemID     int64
	ApprovedBy int64
	occurredAt time.Time
}

func NewFileApproved(itemID, approvedBy int64) *FileApproved {
	return &FileApproved{ItemID: itemID, ApprovedBy: approvedBy, occurredAt: time.Now()}
}

func (e *FileApproved) Type() string        { return "knowledge.file_approved" }
func (e *FileApproved) OccurredAt() time.Time { return e.occurredAt }
func (e *FileApproved) AggregateID() int64   { return e.ItemID }
