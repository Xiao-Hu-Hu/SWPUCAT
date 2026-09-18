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
	ListRecent(ctx context.Context, userID int64, limit int) ([]*Conversation, error)
	UpdateTitle(ctx context.Context, userID int64, sessionID, title string) error
	ListRecentMessages(ctx context.Context, userID int64, sessionID string, limit int) ([]*Message, error)
	RecordTurn(ctx context.Context, userID int64, sessionID, title string, titleCustomized bool, userMessage, assistantMessage *Message, summary, lastSourcesJSON string, activeAt time.Time) error
}

// LifecycleRepository extends the normal conversation store with destructive
// lifecycle operations. Implementations must remove message content, detach
// audit conversation identifiers, and prevent a deleted session handle from
// being recreated for the same owner.
type LifecycleRepository interface {
	Repository
	DeleteOwned(ctx context.Context, userID int64, sessionID string, deletedAt time.Time) error
	CountExpired(ctx context.Context, before time.Time) (int64, error)
	DeleteExpired(ctx context.Context, before time.Time, limit int, deletedAt time.Time) (int64, error)
}

// TurnRepository adds durable idempotency and response feedback without
// changing the legacy Repository contract used by older integrations.
type TurnRepository interface {
	Repository
	FindTurnByClientID(ctx context.Context, userID int64, sessionID, clientTurnID string) (userMessage, assistantMessage *Message, err error)
	CommitTurn(ctx context.Context, userID int64, sessionID, title string, titleCustomized bool, userMessage, assistantMessage *Message, summary, lastSourcesJSON string, activeAt time.Time, regenerate bool) error
	SaveResponseFeedback(ctx context.Context, feedback *ResponseFeedback) error
}
