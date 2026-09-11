package database

import "time"

type ReportPeriodModel struct {
	ID            int64      `gorm:"primaryKey;autoIncrement"`
	PeriodKey     string     `gorm:"size:7;not null;uniqueIndex"`
	StartsOn      time.Time  `gorm:"type:date;not null"`
	EndsOn        time.Time  `gorm:"type:date;not null"`
	Timezone      string     `gorm:"size:64;not null;default:Asia/Shanghai"`
	DueAt         *time.Time `gorm:"index"`
	Status        string     `gorm:"size:16;not null;default:open;index"`
	RosterVersion int64      `gorm:"not null;default:1"`
	CreatedBy     int64      `gorm:"not null"`
	CreatedByName string     `gorm:"size:32;not null;default:''"`
	CreatedAt     time.Time  `gorm:"not null;default:current_timestamp"`
	UpdatedAt     time.Time  `gorm:"not null;default:current_timestamp"`
}

func (ReportPeriodModel) TableName() string { return "report_periods" }

type ReportPeriodMemberModel struct {
	PeriodID        int64      `gorm:"primaryKey"`
	UserID          int64      `gorm:"primaryKey;index"`
	DisplayName     string     `gorm:"size:32;not null;default:''"`
	Required        bool       `gorm:"not null;default:true;index"`
	Exempt          bool       `gorm:"not null;default:false"`
	ExemptionReason string     `gorm:"size:512;not null;default:''"`
	EffectiveFrom   *time.Time `gorm:"type:date"`
	EffectiveUntil  *time.Time `gorm:"type:date"`
	CreatedAt       time.Time  `gorm:"not null;default:current_timestamp"`
}

func (ReportPeriodMemberModel) TableName() string { return "report_period_members" }

type ReportModel struct {
	ID             int64     `gorm:"primaryKey;autoIncrement"`
	PeriodID       int64     `gorm:"not null;index"`
	ReportType     string    `gorm:"column:report_type;size:16;not null;default:monthly"`
	AuthorID       int64     `gorm:"not null;index"`
	AuthorName     string    `gorm:"size:32;not null;default:''"`
	Status         string    `gorm:"size:16;not null;default:draft;index"`
	Visibility     string    `gorm:"size:16;not null;default:private"`
	CurrentVersion int       `gorm:"not null;default:0"`
	CreatedAt      time.Time `gorm:"not null;default:current_timestamp"`
	UpdatedAt      time.Time `gorm:"not null;default:current_timestamp"`
}

func (ReportModel) TableName() string { return "reports" }

type ReportVersionModel struct {
	ID               int64      `gorm:"primaryKey;autoIncrement"`
	ReportID         int64      `gorm:"not null;index"`
	Version          int        `gorm:"not null"`
	Status           string     `gorm:"size:16;not null;default:draft"`
	Title            string     `gorm:"size:256;not null;default:''"`
	BodyMarkdown     string     `gorm:"type:text;not null;default:''"`
	ContentHash      string     `gorm:"size:128;not null;default:''"`
	ParseStatus      string     `gorm:"size:16;not null;default:pending;index"`
	ParseError       string     `gorm:"size:512;not null;default:''"`
	ParserVersion    string     `gorm:"size:32;not null;default:''"`
	ParsedJSON       string     `gorm:"type:jsonb;not null;default:'{}'"`
	ParsedAt         *time.Time `gorm:"index"`
	QualityStatus    string     `gorm:"size:16;not null;default:pending;index"`
	QualityJSON      string     `gorm:"type:jsonb;not null;default:'{}'"`
	QualityCheckedAt *time.Time `gorm:"index"`
	CreatedBy        int64      `gorm:"not null"`
	SubmittedBy      *int64     `gorm:"index"`
	SubmittedAt      *time.Time
	CreatedAt        time.Time `gorm:"not null;default:current_timestamp"`
}

func (ReportVersionModel) TableName() string { return "report_versions" }

type ReportVersionAssetModel struct {
	ReportVersionID int64     `gorm:"primaryKey"`
	KnowledgeItemID int64     `gorm:"primaryKey;index"`
	AssetRole       string    `gorm:"primaryKey;size:32;not null;default:evidence"`
	Locator         string    `gorm:"size:512;not null;default:''"`
	Note            string    `gorm:"size:512;not null;default:''"`
	CreatedAt       time.Time `gorm:"not null;default:current_timestamp"`
}

func (ReportVersionAssetModel) TableName() string { return "report_version_assets" }

type ReportQualityDecisionModel struct {
	ReportVersionID int64     `gorm:"primaryKey"`
	FindingIndex    int       `gorm:"primaryKey"`
	Decision        string    `gorm:"size:16;not null"`
	Note            string    `gorm:"size:512;not null;default:''"`
	DecidedBy       int64     `gorm:"not null"`
	DecidedAt       time.Time `gorm:"not null;default:current_timestamp"`
}

func (ReportQualityDecisionModel) TableName() string { return "report_quality_decisions" }
