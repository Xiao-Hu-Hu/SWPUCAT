package notification

import (
	"errors"
	"strings"
	"time"
)

type Type string

const (
	TypeCheckinAnomaly      Type = "checkin_anomaly"
	TypeReportDue           Type = "report_due"
	TypeEvaluationPublished Type = "evaluation_published"
)

var (
	ErrInvalidNotification = errors.New("invalid notification")
	ErrAlreadyRead         = errors.New("notification already read")
)

type Notification struct {
	ID           int64
	RecipientID  int64
	Type         Type
	Title        string
	BodyMarkdown string
	EntityType   string
	EntityID     int64
	DedupeKey    string
	MetadataJSON string
	ReadAt       *time.Time
	ExpiresAt    *time.Time
	CreatedAt    time.Time
}

type Preferences struct {
	UserID                     int64
	CheckinAnomalyEnabled      bool
	ReportDueEnabled           bool
	EvaluationPublishedEnabled bool
	UpdatedAt                  time.Time
}

func New(recipientID int64, notificationType Type, title, body, entityType string, entityID int64, dedupeKey, metadataJSON string, expiresAt *time.Time) (*Notification, error) {
	if recipientID <= 0 || !notificationType.IsValid() || strings.TrimSpace(title) == "" || strings.TrimSpace(dedupeKey) == "" || len([]rune(title)) > 256 || len([]rune(dedupeKey)) > 256 {
		return nil, ErrInvalidNotification
	}
	if metadataJSON == "" {
		metadataJSON = "{}"
	}
	now := time.Now()
	return &Notification{RecipientID: recipientID, Type: notificationType, Title: strings.TrimSpace(title), BodyMarkdown: body, EntityType: strings.TrimSpace(entityType), EntityID: entityID, DedupeKey: strings.TrimSpace(dedupeKey), MetadataJSON: metadataJSON, ExpiresAt: expiresAt, CreatedAt: now}, nil
}

func (t Type) IsValid() bool {
	return t == TypeCheckinAnomaly || t == TypeReportDue || t == TypeEvaluationPublished
}

func (n *Notification) MarkRead() error {
	if n == nil || n.ReadAt != nil {
		return ErrAlreadyRead
	}
	now := time.Now()
	n.ReadAt = &now
	return nil
}
