package database

import "time"

type EvaluationExceptionModel struct {
	ID            int64     `gorm:"primaryKey;autoIncrement"`
	PeriodID      int64     `gorm:"not null;index"`
	UserID        int64     `gorm:"not null;index"`
	ExceptionType string    `gorm:"size:32;not null"`
	StartsOn      time.Time `gorm:"type:date;not null"`
	EndsOn        time.Time `gorm:"type:date;not null"`
	Reason        string    `gorm:"size:512;not null"`
	Status        string    `gorm:"size:16;not null;default:pending;index"`
	ApprovedBy    *int64    `gorm:"index"`
	ApprovedAt    *time.Time
	CreatedAt     time.Time `gorm:"not null;default:current_timestamp"`
}

func (EvaluationExceptionModel) TableName() string { return "evaluation_exceptions" }

type EvaluationSnapshotModel struct {
	ID              int64     `gorm:"primaryKey;autoIncrement"`
	PeriodID        int64     `gorm:"not null;index"`
	PeriodKey       string    `gorm:"size:7;not null"`
	StartsOn        time.Time `gorm:"type:date;not null"`
	EndsOn          time.Time `gorm:"type:date;not null"`
	Timezone        string    `gorm:"size:64;not null"`
	CutoffAt        time.Time `gorm:"not null"`
	RosterVersion   int64     `gorm:"not null"`
	RuleVersion     string    `gorm:"size:64;not null"`
	SourceWatermark string    `gorm:"size:128;not null;default:''"`
	SnapshotHash    string    `gorm:"size:128;not null;uniqueIndex"`
	Status          string    `gorm:"size:16;not null;default:draft;index"`
	CreatedBy       int64     `gorm:"not null"`
	CreatedAt       time.Time `gorm:"not null;default:current_timestamp"`
}

func (EvaluationSnapshotModel) TableName() string { return "evaluation_snapshots" }

type EvaluationSnapshotMemberModel struct {
	SnapshotID      int64   `gorm:"primaryKey"`
	UserID          int64   `gorm:"primaryKey;index"`
	DisplayName     string  `gorm:"size:32;not null;default:''"`
	Required        bool    `gorm:"not null;default:true"`
	Exempt          bool    `gorm:"not null;default:false"`
	TotalMinutes    float64 `gorm:"type:numeric(12,2);not null;default:0"`
	TotalHours      float64 `gorm:"type:numeric(12,2);not null;default:0"`
	AttendanceDays  int     `gorm:"not null;default:0"`
	AchievedWeeks   int     `gorm:"not null;default:0"`
	AnomaliesJSON   string  `gorm:"type:jsonb;not null;default:'[]'"`
	ReportStatus    string  `gorm:"size:16;not null;default:unknown"`
	ReportVersionID *int64  `gorm:"index"`
	DataState       string  `gorm:"size:16;not null;default:complete"`
	UnknownReason   string  `gorm:"size:512;not null;default:''"`
}

func (EvaluationSnapshotMemberModel) TableName() string { return "evaluation_snapshot_members" }
