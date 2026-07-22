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
	UploaderID   int64
	UploaderName string
	Approved     bool
	Rejected     bool
	RejectReason string
	ReviewerID   int64
	ReviewerName string
	CreatedAt    time.Time
}

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
		Approved:     true,
		CreatedAt:    time.Now(),
	}, nil
}

func NewFile(name, description, fileSize, fileKey string, categoryID, uploaderID int64, uploaderName string, approved bool) (*KnowledgeItem, error) {
	if name == "" {
		return nil, ErrEmptyItemName
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
		Approved:     approved,
		CreatedAt:    time.Now(),
	}, nil
}

func (k *KnowledgeItem) Approve() {
	k.Approved = true
}

func (k *KnowledgeItem) IsOwnedBy(userID int64) bool {
	return k.UploaderID == userID
}
