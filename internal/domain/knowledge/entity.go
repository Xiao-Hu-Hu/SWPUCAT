package knowledge

import "time"

type Category struct {
	ID        int64
	Name      string
	IsSystem  bool
	CreatedAt time.Time
}

type KnowledgeItem struct {
	ID           int64
	Type         ItemType
	Name         string
	Description  string
	URL          string
	FileKey      string
	FileSize     string
	CategoryID   int64
	CategoryName string
	UploaderID   int64
	UploaderName string
	Status       ItemStatus
	Approved     bool
	Rejected     bool
	RejectReason string
	ReviewerID   int64
	ReviewerName string
	SourceOrigin string
	SourceKey    string
	Revision     int
	DeletedAt    *time.Time
	DeletedBy    int64
	CreatedAt    time.Time
}

const (
	SourceOriginUser            = "user"
	SourceOriginOfficialCatalog = "official_catalog"
)

func NewLink(name, url, description string, categoryID, uploaderID int64, uploaderName string) (*KnowledgeItem, error) {
	if name == "" {
		return nil, ErrEmptyItemName
	}
	if url == "" {
		return nil, ErrEmptyURL
	}
	return &KnowledgeItem{
		Type:         ItemTypeLink,
		Name:         name,
		Description:  description,
		URL:          url,
		CategoryID:   categoryID,
		UploaderID:   uploaderID,
		UploaderName: uploaderName,
		Status:       ItemStatusApproved,
		Approved:     true,
		SourceOrigin: SourceOriginUser,
		Revision:     1,
		CreatedAt:    time.Now(),
	}, nil
}

func NewFile(name, description, fileSize, fileKey string, categoryID, uploaderID int64, uploaderName string, approved bool) (*KnowledgeItem, error) {
	if name == "" {
		return nil, ErrEmptyItemName
	}
	status := ItemStatusPending
	if approved {
		status = ItemStatusApproved
	}
	return &KnowledgeItem{
		Type:         ItemTypeFile,
		Name:         name,
		Description:  description,
		FileKey:      fileKey,
		FileSize:     fileSize,
		CategoryID:   categoryID,
		UploaderID:   uploaderID,
		UploaderName: uploaderName,
		Status:       status,
		Approved:     approved,
		SourceOrigin: SourceOriginUser,
		Revision:     1,
		CreatedAt:    time.Now(),
	}, nil
}

func (k *KnowledgeItem) Approve() {
	k.RejectReason = ""
	k.SetStatus(ItemStatusApproved)
}

func (k *KnowledgeItem) Reject(reason string) {
	k.RejectReason = reason
	k.SetStatus(ItemStatusRejected)
}

func (k *KnowledgeItem) MarkDeleted(deletedBy int64) {
	now := time.Now()
	k.DeletedAt = &now
	k.DeletedBy = deletedBy
	k.SetStatus(ItemStatusDeleted)
}

func (k *KnowledgeItem) SetStatus(status ItemStatus) {
	k.Status = status
	switch status {
	case ItemStatusApproved:
		k.Approved = true
		k.Rejected = false
	case ItemStatusRejected:
		k.Approved = false
		k.Rejected = true
	case ItemStatusPending, ItemStatusDeleted:
		k.Approved = false
		k.Rejected = false
	}
}

func (k *KnowledgeItem) EffectiveStatus() ItemStatus {
	if k.Status.IsValid() {
		return k.Status
	}
	if k.Rejected {
		return ItemStatusRejected
	}
	if k.Approved {
		return ItemStatusApproved
	}
	return ItemStatusPending
}

func (k *KnowledgeItem) IsDeleted() bool {
	return k.EffectiveStatus() == ItemStatusDeleted
}

func (k *KnowledgeItem) IsOwnedBy(userID int64) bool {
	return k.UploaderID == userID
}

func (k *KnowledgeItem) IsOfficial() bool {
	return k.SourceOrigin == SourceOriginOfficialCatalog && k.SourceKey != ""
}

func (k *KnowledgeItem) IsVisibleTo(viewerID int64, canReview bool) bool {
	if k.IsDeleted() {
		return false
	}
	if canReview || k.IsOwnedBy(viewerID) {
		return true
	}
	return k.EffectiveStatus() == ItemStatusApproved
}
