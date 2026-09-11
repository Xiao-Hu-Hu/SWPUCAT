package database

import "time"

type ReportPeriodCategoryBindingModel struct {
	PeriodID    int64   `gorm:"primaryKey"`
	CategoryID  int64   `gorm:"primaryKey;index"`
	BindingType string  `gorm:"size:16;not null;default:legacy"`
	Confidence  float64 `gorm:"type:numeric(5,4);not null;default:0"`
	Source      string  `gorm:"size:32;not null;default:manual"`
	ConfirmedBy *int64  `gorm:"index"`
	ConfirmedAt *time.Time
	CreatedAt   time.Time `gorm:"not null;default:current_timestamp"`
}

func (ReportPeriodCategoryBindingModel) TableName() string { return "report_period_category_bindings" }

type ReportMigrationRunModel struct {
	ID               int64     `gorm:"primaryKey;autoIncrement"`
	Mode             string    `gorm:"size:16;not null;default:dry_run"`
	Status           string    `gorm:"size:16;not null;default:created;index"`
	FilterCategoryID *int64    `gorm:"index"`
	StartedAt        time.Time `gorm:"not null;default:current_timestamp"`
	FinishedAt       *time.Time
	CreatedBy        *int64 `gorm:"index"`
	SummaryJSON      string `gorm:"type:jsonb;not null;default:'{}'"`
}

func (ReportMigrationRunModel) TableName() string { return "report_migration_runs" }

type ReportMigrationItemModel struct {
	ID                  int64   `gorm:"primaryKey;autoIncrement"`
	RunID               int64   `gorm:"not null;index"`
	KnowledgeItemID     int64   `gorm:"not null;index"`
	SuggestedPeriodKey  string  `gorm:"size:7;not null;default:''"`
	SuggestedAuthorID   *int64  `gorm:"index"`
	SuggestedAuthorName string  `gorm:"size:32;not null;default:''"`
	Decision            string  `gorm:"size:16;not null;default:needs_review;index"`
	Confidence          float64 `gorm:"type:numeric(5,4);not null;default:0"`
	Reason              string  `gorm:"size:512;not null;default:''"`
	TargetReportID      *int64  `gorm:"index"`
	ReviewedBy          *int64  `gorm:"index"`
	ReviewedAt          *time.Time
	CreatedAt           time.Time `gorm:"not null;default:current_timestamp"`
}

func (ReportMigrationItemModel) TableName() string { return "report_migration_items" }
