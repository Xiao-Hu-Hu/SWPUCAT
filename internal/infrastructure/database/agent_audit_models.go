package database

import "time"

// AgentRunModel maps the append-only request-level execution record. Pointer
// fields intentionally preserve the distinction between an observed zero and
// unavailable telemetry.
type AgentRunModel struct {
	ID             int64   `gorm:"primaryKey;autoIncrement"`
	RequestID      string  `gorm:"size:32;not null;uniqueIndex"`
	UserID         *int64  `gorm:"index"`
	ConversationID *int64  `gorm:"index"`
	SessionID      *string `gorm:"size:128;index"`
	ActorRole      *string `gorm:"size:16"`

	Status           string  `gorm:"size:16;not null;index"`
	TelemetryStatus  string  `gorm:"size:16;not null"`
	HTTPStatus       *int    `gorm:"type:smallint"`
	FailureSource    *string `gorm:"size:32"`
	FailureCategory  *string `gorm:"size:32"`
	FailureCode      *string `gorm:"size:64"`
	FailureRetryable *bool

	StartedAt  time.Time `gorm:"not null"`
	FinishedAt time.Time `gorm:"not null"`
	DurationMS int64     `gorm:"not null"`

	AgentStartedAt  *time.Time
	AgentFinishedAt *time.Time
	AgentDurationMS *int64

	ConversationPersisted *bool

	ModelCallCount          *int
	ToolCallCount           *int
	DroppedModelCalls       *int
	DroppedToolCalls        *int
	UsageReportedModelCalls *int `gorm:"column:usage_reported_model_calls"`
	UsageMissingModelCalls  *int `gorm:"column:usage_missing_model_calls"`

	PromptTokens     *int64
	CompletionTokens *int64
	TotalTokens      *int64
	CachedTokens     *int64
	ReasoningTokens  *int64

	UsageJSON      *string   `gorm:"column:usage_json;type:json"`
	ModelCallsJSON *string   `gorm:"column:model_calls_json;type:json"`
	CreatedAt      time.Time `gorm:"not null;default:current_timestamp"`
}

func (AgentRunModel) TableName() string { return "agent_runs" }

// AgentToolCallModel contains only bounded argument/result projections. Raw
// payloads and free-form error Message/Param fields have no persistence slot.
type AgentToolCallModel struct {
	ID       int64   `gorm:"primaryKey;autoIncrement"`
	RunID    int64   `gorm:"not null;uniqueIndex:uq_agent_tool_calls_run_sequence;index"`
	Sequence int64   `gorm:"not null;uniqueIndex:uq_agent_tool_calls_run_sequence"`
	CallID   *string `gorm:"size:64"`
	ToolName string  `gorm:"size:128;not null;index"`
	IsWrite  bool    `gorm:"not null;default:false;index"`
	Status   string  `gorm:"size:16;not null"`

	StartedAt  time.Time `gorm:"not null;index"`
	DurationMS int64     `gorm:"not null"`

	ArgumentsJSON          string `gorm:"column:arguments_json;type:json;not null"`
	ArgumentsOriginalBytes int64  `gorm:"not null"`
	ArgumentsCapturedBytes int    `gorm:"not null"`
	ArgumentsRedacted      bool   `gorm:"not null"`
	ArgumentsTruncated     bool   `gorm:"not null"`

	ResultJSON          string `gorm:"column:result_json;type:json;not null"`
	ResultOriginalBytes int64  `gorm:"not null"`
	ResultCapturedBytes int    `gorm:"not null"`
	ResultRedacted      bool   `gorm:"not null"`
	ResultTruncated     bool   `gorm:"not null"`

	ErrorCategory   *string `gorm:"size:32"`
	ErrorCode       *string `gorm:"size:64"`
	ErrorType       *string `gorm:"size:64"`
	ErrorHTTPStatus *int    `gorm:"type:smallint"`
	ErrorRetryable  *bool

	BusinessObjectType   *string   `gorm:"size:32;index:idx_agent_tool_calls_business_object"`
	BusinessObjectID     *int64    `gorm:"index:idx_agent_tool_calls_business_object"`
	BusinessObjectStatus *string   `gorm:"size:32"`
	CreatedAt            time.Time `gorm:"not null;default:current_timestamp"`
}

func (AgentToolCallModel) TableName() string { return "agent_tool_calls" }
