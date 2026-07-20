package database

import "time"

type UserModel struct {
	ID              int64     `gorm:"primaryKey;autoIncrement"`
	Username        string    `gorm:"uniqueIndex;size:32;not null"`
	PasswordHash    string    `gorm:"size:256;not null"`
	Nickname        string    `gorm:"size:32;not null"`
	Role            string    `gorm:"size:16;not null;default:member"`
	CheckinCount    int64     `gorm:"not null;default:0"`
	LastCheckinDate string    `gorm:"size:10"`
	CreatedAt       time.Time `gorm:"not null;default:current_timestamp"`
	UpdatedAt       time.Time `gorm:"not null;default:current_timestamp"`
}

func (UserModel) TableName() string { return "users" }

type AnnouncementModel struct {
	ID         int64     `gorm:"primaryKey;autoIncrement"`
	Title      string    `gorm:"size:256;not null"`
	Content    string    `gorm:"type:text;not null"`
	AuthorID   int64     `gorm:"not null"`
	AuthorName string    `gorm:"size:32;not null"`
	Pinned     bool      `gorm:"not null;default:false"`
	CreatedAt  time.Time `gorm:"not null;default:current_timestamp"`
	UpdatedAt  time.Time `gorm:"not null;default:current_timestamp"`
}

func (AnnouncementModel) TableName() string { return "announcements" }

type CheckinRecordModel struct {
	ID        int64     `gorm:"primaryKey;autoIncrement"`
	UserID    int64     `gorm:"not null;index"`
	Type      string    `gorm:"size:8;not null"`
	Date      string    `gorm:"size:10;not null;index"`
	Time      string    `gorm:"size:8;not null"`
	CreatedAt time.Time `gorm:"not null;default:current_timestamp"`
}

func (CheckinRecordModel) TableName() string { return "checkin_records" }

type KnowledgeCategoryModel struct {
	ID        int64     `gorm:"primaryKey;autoIncrement"`
	Name      string    `gorm:"size:64;not null"`
	IsSystem  bool      `gorm:"not null;default:false"`
	CreatedAt time.Time `gorm:"not null;default:current_timestamp"`
}

func (KnowledgeCategoryModel) TableName() string { return "knowledge_categories" }

type KnowledgeItemModel struct {
	ID           int64     `gorm:"primaryKey;autoIncrement"`
	Type         string    `gorm:"size:8;not null"`
	Name         string    `gorm:"size:256;not null"`
	URL          string    `gorm:"size:512"`
	FileSize     string    `gorm:"size:32"`
	FileKey      string    `gorm:"size:256"`
	CategoryID   int64     `gorm:"not null;index"`
	CategoryName string    `gorm:"size:64"`
	UploaderID   int64     `gorm:"not null"`
	UploaderName string    `gorm:"size:32;not null"`
	Approved     bool      `gorm:"not null;default:false"`
	CreatedAt    time.Time `gorm:"not null;default:current_timestamp"`
}

func (KnowledgeItemModel) TableName() string { return "knowledge_items" }

type ApprovalModel struct {
	ID           int64     `gorm:"primaryKey;autoIncrement"`
	FileName     string    `gorm:"size:256;not null"`
	FileSize     string    `gorm:"size:32"`
	FileKey      string    `gorm:"size:256"`
	CategoryID   int64     `gorm:"not null"`
	UploaderID   int64     `gorm:"not null;index"`
	UploaderName string    `gorm:"size:32;not null"`
	ReviewerID   int64
	Status       string    `gorm:"size:16;not null;default:pending"`
	CreatedAt    time.Time `gorm:"not null;default:current_timestamp"`
	UpdatedAt    time.Time `gorm:"not null;default:current_timestamp"`
}

func (ApprovalModel) TableName() string { return "approvals" }

type SettingModel struct {
	ID    int64  `gorm:"primaryKey;autoIncrement"`
	Key   string `gorm:"uniqueIndex;size:64;not null"`
	Value string `gorm:"type:text;not null"`
}

func (SettingModel) TableName() string { return "settings" }
