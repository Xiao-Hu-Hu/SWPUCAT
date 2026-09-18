// Package conversation contains the durable, user-owned state used to rebuild
// an Agent conversation after the Agent process has restarted.
package conversation

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DefaultTitle      = "新会话"
	MaxSessionIDRunes = 128
	MaxTitleRunes     = 256
	MaxSummaryRunes   = 32000
	MaxMessageRunes   = 120000
	MaxToolNameRunes  = 128
	IdentifierLength  = 32
	MaxFeedbackRunes  = 500
)

var (
	ErrInvalidConversation = errors.New("invalid conversation")
	ErrInvalidMessage      = errors.New("invalid conversation message")
	ErrDeletedConversation = errors.New("conversation was permanently deleted")
	ErrTurnNotFound        = errors.New("conversation turn was not found")
	ErrTurnConflict        = errors.New("conversation turn conflicts with an existing request")
	ErrInvalidFeedback     = errors.New("invalid conversation feedback")
)

type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

func (r MessageRole) IsValid() bool {
	switch r {
	case RoleSystem, RoleUser, RoleAssistant, RoleTool:
		return true
	default:
		return false
	}
}

// Conversation is deliberately scoped to an authenticated user. SessionID is
// only an opaque client handle; it is never sufficient authorization on its
// own.
type Conversation struct {
	ID              int64
	UserID          int64
	SessionID       string
	Title           string
	TitleCustomized bool
	RollingSummary  string
	LastSourcesJSON string
	LastActiveAt    time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Message struct {
	ID             int64
	ConversationID int64
	Role           MessageRole
	Content        string
	ToolName       string
	ToolResult     string
	CitationsJSON  string
	TokenCount     int
	ClientTurnID   string
	GenerationID   string
	RequestID      string
	CreatedAt      time.Time
}

type ResponseFeedback struct {
	ID        int64
	UserID    int64
	MessageID int64
	RequestID string
	Helpful   bool
	Note      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func New(userID int64, sessionID, title string) (*Conversation, error) {
	sessionID = strings.TrimSpace(sessionID)
	title = strings.TrimSpace(title)
	if userID <= 0 || sessionID == "" || utf8.RuneCountInString(sessionID) > MaxSessionIDRunes || utf8.RuneCountInString(title) > MaxTitleRunes {
		return nil, ErrInvalidConversation
	}
	now := time.Now()
	return &Conversation{
		UserID:          userID,
		SessionID:       sessionID,
		Title:           title,
		TitleCustomized: title != "" && title != DefaultTitle,
		LastSourcesJSON: "[]",
		LastActiveAt:    now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

func (c *Conversation) IsOwnedBy(userID int64) bool {
	return c != nil && userID > 0 && c.UserID == userID
}

func (c *Conversation) UpdateContext(summary, lastSourcesJSON string, activeAt time.Time) error {
	if c == nil || utf8.RuneCountInString(summary) > MaxSummaryRunes || strings.TrimSpace(lastSourcesJSON) == "" {
		return ErrInvalidConversation
	}
	if activeAt.IsZero() {
		activeAt = time.Now()
	}
	c.RollingSummary = strings.TrimSpace(summary)
	c.LastSourcesJSON = lastSourcesJSON
	c.LastActiveAt = activeAt
	c.UpdatedAt = activeAt
	return nil
}

func NewMessage(conversationID int64, role MessageRole, content, toolName, toolResult, citationsJSON string, tokenCount int) (*Message, error) {
	content = strings.TrimSpace(content)
	toolName = strings.TrimSpace(toolName)
	if conversationID <= 0 || !role.IsValid() || content == "" || utf8.RuneCountInString(content) > MaxMessageRunes || utf8.RuneCountInString(toolName) > MaxToolNameRunes || tokenCount < 0 {
		return nil, ErrInvalidMessage
	}
	if strings.TrimSpace(citationsJSON) == "" {
		citationsJSON = "[]"
	}
	return &Message{
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
		ToolName:       toolName,
		ToolResult:     toolResult,
		CitationsJSON:  citationsJSON,
		TokenCount:     tokenCount,
		CreatedAt:      time.Now(),
	}, nil
}
