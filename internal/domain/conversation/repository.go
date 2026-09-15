package conversation

import (
	"context"
	"time"
)

// Repository methods take both userID and sessionID for every read/write that
// is addressed by a session. This keeps ownership enforcement in the
// persistence boundary rather than relying on callers to remember a filter.
type Repository interface {
	Create(ctx context.Context, item *Conversation) error
	FindBySession(ctx context.Context, userID int64, sessionID string) (*Conversation, error)
	ListRecentMessages(ctx context.Context, userID int64, sessionID string, limit int) ([]*Message, error)
	RecordTurn(ctx context.Context, userID int64, sessionID string, userMessage, assistantMessage *Message, summary, lastSourcesJSON string, activeAt time.Time) error
}
