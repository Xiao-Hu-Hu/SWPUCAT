package database

import "time"

type UserModel struct {
	ID              int64  `gorm:"primaryKey;autoIncrement"`
	Username        string `gorm:"uniqueIndex;size:32;not null"`
	StudentID       string `gorm:"uniqueIndex;size:12"`
	Email           string `gorm:"size:256"`
	PasswordHash    string `gorm:"size:256;not null"`
	Nickname        string `gorm:"size:32;not null"`
	Avatar          string `gorm:"size:256"`
	TechDirection   string `gorm:"size:32"`
	Role            string `gorm:"size:16;not null;default:member"`
	Status          string `gorm:"size:16;not null;default:active;index"`
	AuthVersion     int64  `gorm:"not null;default:1"`
	DisabledAt      *time.Time
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

type NotificationModel struct {
	ID               int64      `gorm:"primaryKey;autoIncrement"`
	RecipientID      int64      `gorm:"not null;index"`
	NotificationType string     `gorm:"column:notification_type;size:32;not null;index"`
	Title            string     `gorm:"size:256;not null"`
	BodyMarkdown     string     `gorm:"type:text;not null;default:''"`
	EntityType       string     `gorm:"size:32;not null;default:''"`
	EntityID         *int64     `gorm:"index"`
	DedupeKey        string     `gorm:"size:256;not null"`
	MetadataJSON     string     `gorm:"column:metadata_json;type:jsonb;not null;default:'{}'"`
	ReadAt           *time.Time `gorm:"index"`
	ExpiresAt        *time.Time
	CreatedAt        time.Time `gorm:"not null;default:current_timestamp;index"`
}

func (NotificationModel) TableName() string { return "notifications" }

type NotificationPreferenceModel struct {
	UserID                     int64     `gorm:"primaryKey"`
	CheckinAnomalyEnabled      bool      `gorm:"not null;default:true"`
	ReportDueEnabled           bool      `gorm:"not null;default:true"`
	EvaluationPublishedEnabled bool      `gorm:"not null;default:true"`
	UpdatedAt                  time.Time `gorm:"not null;default:current_timestamp"`
}

func (NotificationPreferenceModel) TableName() string { return "notification_preferences" }

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

type CheckinIdempotencyModel struct {
	ID             int64     `gorm:"primaryKey;autoIncrement"`
	UserID         int64     `gorm:"not null;index"`
	Action         string    `gorm:"size:8;not null"`
	IdempotencyKey string    `gorm:"size:128;not null"`
	RecordID       int64     `gorm:"not null"`
	RecordDate     string    `gorm:"size:10;not null"`
	RecordTime     string    `gorm:"size:8;not null"`
	CreatedAt      time.Time `gorm:"not null;default:current_timestamp"`
}

func (CheckinIdempotencyModel) TableName() string { return "checkin_idempotency_keys" }

type CheckinAutoClockoutRunModel struct {
	ID           int64     `gorm:"primaryKey;autoIncrement"`
	RunDate      string    `gorm:"size:10;not null;uniqueIndex"`
	Status       string    `gorm:"size:16;not null"`
	StartedAt    time.Time `gorm:"not null"`
	FinishedAt   *time.Time
	Processed    int       `gorm:"not null;default:0"`
	ErrorMessage string    `gorm:"size:512"`
	CreatedAt    time.Time `gorm:"not null;default:current_timestamp"`
}

func (CheckinAutoClockoutRunModel) TableName() string { return "checkin_auto_clockout_runs" }

type KnowledgeCategoryModel struct {
	ID        int64     `gorm:"primaryKey;autoIncrement"`
	Name      string    `gorm:"size:64;not null"`
	IsSystem  bool      `gorm:"not null;default:false"`
	CreatedAt time.Time `gorm:"not null;default:current_timestamp"`
}

func (KnowledgeCategoryModel) TableName() string { return "knowledge_categories" }

type KnowledgeItemModel struct {
	ID                   int64      `gorm:"primaryKey;autoIncrement"`
	Type                 string     `gorm:"size:8;not null"`
	Name                 string     `gorm:"size:256;not null"`
	Description          string     `gorm:"type:text"`
	URL                  string     `gorm:"size:512"`
	FileSize             string     `gorm:"size:32"`
	FileKey              string     `gorm:"size:256"`
	CategoryID           int64      `gorm:"not null;index"`
	CategoryName         string     `gorm:"size:64"`
	UploaderID           int64      `gorm:"not null"`
	UploaderName         string     `gorm:"size:32;not null"`
	Status               string     `gorm:"size:16;not null;default:pending;index"`
	Approved             bool       `gorm:"not null;default:false"`
	Rejected             bool       `gorm:"not null;default:false"`
	RejectReason         string     `gorm:"size:512"`
	ReviewerID           int64      `gorm:"index"`
	ReviewerName         string     `gorm:"size:32"`
	SourceOrigin         string     `gorm:"size:24;not null;default:user;index"`
	SourceKey            string     `gorm:"size:128"`
	Revision             int        `gorm:"not null;default:1"`
	QuarantinedAt        *time.Time `gorm:"index"`
	QuarantinedBy        int64
	QuarantineReasonCode string     `gorm:"size:64"`
	QuarantineReason     string     `gorm:"size:512"`
	DeletedAt            *time.Time `gorm:"index"`
	DeletedBy            int64
	CreatedAt            time.Time `gorm:"not null;default:current_timestamp"`
}

func (KnowledgeItemModel) TableName() string { return "knowledge_items" }

type KnowledgeItemLifecycleEventModel struct {
	ID         int64     `gorm:"primaryKey;autoIncrement"`
	ItemID     int64     `gorm:"not null;index"`
	EventType  string    `gorm:"size:32;not null;index"`
	FromStatus string    `gorm:"size:16;not null"`
	ToStatus   string    `gorm:"size:16;not null"`
	ActorID    int64     `gorm:"not null;index"`
	ActorName  string    `gorm:"size:32;not null"`
	ReasonCode string    `gorm:"size:64;not null"`
	Reason     string    `gorm:"size:512;not null"`
	SourceKey  string    `gorm:"size:128"`
	Revision   int       `gorm:"not null;default:1"`
	CreatedAt  time.Time `gorm:"not null;default:current_timestamp;index"`
}

func (KnowledgeItemLifecycleEventModel) TableName() string {
	return "knowledge_item_lifecycle_events"
}

type ApprovalModel struct {
	ID           int64  `gorm:"primaryKey;autoIncrement"`
	FileName     string `gorm:"size:256;not null"`
	FileSize     string `gorm:"size:32"`
	FileKey      string `gorm:"size:256"`
	Description  string `gorm:"type:text;not null;default:''"`
	CategoryID   int64  `gorm:"not null"`
	UploaderID   int64  `gorm:"not null;index"`
	UploaderName string `gorm:"size:32;not null"`
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

type VerificationCodeModel struct {
	ID        int64     `gorm:"primaryKey;autoIncrement"`
	Email     string    `gorm:"size:256;not null;index"`
	Code      string    `gorm:"size:6;not null"`
	Used      bool      `gorm:"not null;default:false"`
	ExpiresAt time.Time `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null;default:current_timestamp"`
}

func (VerificationCodeModel) TableName() string { return "verification_codes" }

type InvitationCodeModel struct {
	ID        int64     `gorm:"primaryKey;autoIncrement"`
	Code      string    `gorm:"uniqueIndex;size:6;not null"`
	Type      string    `gorm:"size:16;not null"`
	CreatorID int64     `gorm:"not null;index"`
	UsedBy    *int64    `gorm:"index"`
	Used      bool      `gorm:"not null;default:false"`
	ExpiresAt time.Time `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null;default:current_timestamp"`
}

func (InvitationCodeModel) TableName() string { return "invitation_codes" }
