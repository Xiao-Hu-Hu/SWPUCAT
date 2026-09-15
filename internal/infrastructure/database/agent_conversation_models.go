package database

import "time"

// AgentConversationModel and AgentMessageModel map to the dedicated durable
// Agent history tables. They intentionally do not contain a GORM association:
// all repository operations add the user ownership predicate explicitly.
type AgentConversationModel struct {
	ID              int64     `gorm:"primaryKey;autoIncrement"`
	UserID          int64     `gorm:"not null;uniqueIndex:idx_agent_conversation_owner_session;index:idx_agent_conversation_owner_active"`
	SessionID       string    `gorm:"size:128;not null;uniqueIndex:idx_agent_conversation_owner_session"`
	Title           string    `gorm:"size:256;not null;default:''"`
	RollingSummary  string    `gorm:"type:text;not null;default:''"`
	LastSourcesJSON string    `gorm:"column:last_sources_json;type:jsonb;not null;default:'[]'"`
	LastActiveAt    time.Time `gorm:"not null;index:idx_agent_conversation_owner_active"`
	CreatedAt       time.Time `gorm:"not null;default:current_timestamp"`
	UpdatedAt       time.Time `gorm:"not null;default:current_timestamp"`
}

func (AgentConversationModel) TableName() string { return "agent_conversations" }

type AgentMessageModel struct {
	ID             int64     `gorm:"primaryKey;autoIncrement"`
	ConversationID int64     `gorm:"not null;index:idx_agent_message_conversation_created"`
	Role           string    `gorm:"size:16;not null"`
	Content        string    `gorm:"type:text;not null"`
	ToolName       string    `gorm:"size:128;not null;default:''"`
	ToolResult     string    `gorm:"type:text;not null;default:''"`
	CitationsJSON  string    `gorm:"column:citations_json;type:jsonb;not null;default:'[]'"`
	TokenCount     int       `gorm:"not null;default:0"`
	CreatedAt      time.Time `gorm:"not null;default:current_timestamp;index:idx_agent_message_conversation_created"`
}

func (AgentMessageModel) TableName() string { return "agent_messages" }
