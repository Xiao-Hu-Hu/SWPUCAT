package database

import "time"

type AgentActionConfirmationModel struct {
	TokenHash        string     `gorm:"column:token_hash;size:64;primaryKey"`
	UserID           int64      `gorm:"column:user_id;not null"`
	SessionID        string     `gorm:"column:session_id;size:128;not null"`
	ToolName         string     `gorm:"column:tool_name;size:128;not null"`
	ParametersDigest string     `gorm:"column:parameters_digest;size:64;not null"`
	Status           string     `gorm:"size:16;not null"`
	ResultJSON       string     `gorm:"column:result_json;type:jsonb;not null"`
	ErrorCode        *string    `gorm:"column:error_code;size:64"`
	ExpiresAt        time.Time  `gorm:"column:expires_at;not null"`
	CreatedAt        time.Time  `gorm:"column:created_at;not null"`
	CompletedAt      *time.Time `gorm:"column:completed_at"`
}

func (AgentActionConfirmationModel) TableName() string { return "agent_action_confirmations" }
