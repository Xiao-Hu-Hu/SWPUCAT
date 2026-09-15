package repository

import (
	"context"
	"time"

	"SWPUCAT/internal/domain/conversation"
	"SWPUCAT/internal/infrastructure/database"

	"gorm.io/gorm"
)

// ConversationRepo persists Agent history in PostgreSQL. All methods that use
// a session carry userID into the SQL predicate, so a guessed session id cannot
// disclose another user's summary or messages.
type ConversationRepo struct {
	db *gorm.DB
}

func NewConversationRepo(db *gorm.DB) *ConversationRepo {
	return &ConversationRepo{db: db}
}

func (r *ConversationRepo) Create(ctx context.Context, item *conversation.Conversation) error {
	if r == nil || r.db == nil || item == nil {
		return conversation.ErrInvalidConversation
	}
	model := &database.AgentConversationModel{
		UserID:          item.UserID,
		SessionID:       item.SessionID,
		Title:           item.Title,
		RollingSummary:  item.RollingSummary,
		LastSourcesJSON: item.LastSourcesJSON,
		LastActiveAt:    item.LastActiveAt,
		CreatedAt:       item.CreatedAt,
		UpdatedAt:       item.UpdatedAt,
	}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	item.ID = model.ID
	item.CreatedAt = model.CreatedAt
	item.UpdatedAt = model.UpdatedAt
	item.LastActiveAt = model.LastActiveAt
	return nil
}

func (r *ConversationRepo) FindBySession(ctx context.Context, userID int64, sessionID string) (*conversation.Conversation, error) {
	if r == nil || r.db == nil || userID <= 0 {
		return nil, conversation.ErrInvalidConversation
	}
	var model database.AgentConversationModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND session_id = ?", userID, sessionID).
		First(&model).Error
	if err != nil {
		return nil, err
	}
	return toDomainConversation(&model), nil
}

func (r *ConversationRepo) ListRecentMessages(ctx context.Context, userID int64, sessionID string, limit int) ([]*conversation.Message, error) {
	if r == nil || r.db == nil || userID <= 0 || limit < 1 {
		return nil, conversation.ErrInvalidConversation
	}
	var models []database.AgentMessageModel
	err := r.db.WithContext(ctx).
		Table((database.AgentMessageModel{}).TableName()).
		Select("agent_messages.*").
		Joins("JOIN agent_conversations ON agent_conversations.id = agent_messages.conversation_id").
		Where("agent_conversations.user_id = ? AND agent_conversations.session_id = ?", userID, sessionID).
		Order("agent_messages.created_at DESC, agent_messages.id DESC").
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	// The database reads the most recent rows first for an efficient index scan;
	// callers receive chronological history ready for a chat model.
	items := make([]*conversation.Message, 0, len(models))
	for i := len(models) - 1; i >= 0; i-- {
		items = append(items, toDomainMessage(&models[i]))
	}
	return items, nil
}

func (r *ConversationRepo) RecordTurn(ctx context.Context, userID int64, sessionID string, userMessage, assistantMessage *conversation.Message, summary, lastSourcesJSON string, activeAt time.Time) error {
	if r == nil || r.db == nil || userID <= 0 || userMessage == nil || assistantMessage == nil || activeAt.IsZero() {
		return conversation.ErrInvalidMessage
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record database.AgentConversationModel
		if err := tx.Where("user_id = ? AND session_id = ?", userID, sessionID).First(&record).Error; err != nil {
			return err
		}
		for _, message := range []*conversation.Message{userMessage, assistantMessage} {
			if message.ConversationID != record.ID {
				return conversation.ErrInvalidMessage
			}
			model := &database.AgentMessageModel{
				ConversationID: record.ID,
				Role:           string(message.Role),
				Content:        message.Content,
				ToolName:       message.ToolName,
				ToolResult:     message.ToolResult,
				CitationsJSON:  message.CitationsJSON,
				TokenCount:     message.TokenCount,
				CreatedAt:      message.CreatedAt,
			}
			if err := tx.Create(model).Error; err != nil {
				return err
			}
			message.ID = model.ID
			message.CreatedAt = model.CreatedAt
		}
		result := tx.Model(&database.AgentConversationModel{}).Where("id = ? AND user_id = ?", record.ID, userID).Updates(map[string]interface{}{
			"rolling_summary":   summary,
			"last_sources_json": lastSourcesJSON,
			"last_active_at":    activeAt,
			"updated_at":        activeAt,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func toDomainConversation(model *database.AgentConversationModel) *conversation.Conversation {
	return &conversation.Conversation{
		ID:              model.ID,
		UserID:          model.UserID,
		SessionID:       model.SessionID,
		Title:           model.Title,
		RollingSummary:  model.RollingSummary,
		LastSourcesJSON: model.LastSourcesJSON,
		LastActiveAt:    model.LastActiveAt,
		CreatedAt:       model.CreatedAt,
		UpdatedAt:       model.UpdatedAt,
	}
}

func toDomainMessage(model *database.AgentMessageModel) *conversation.Message {
	return &conversation.Message{
		ID:             model.ID,
		ConversationID: model.ConversationID,
		Role:           conversation.MessageRole(model.Role),
		Content:        model.Content,
		ToolName:       model.ToolName,
		ToolResult:     model.ToolResult,
		CitationsJSON:  model.CitationsJSON,
		TokenCount:     model.TokenCount,
		CreatedAt:      model.CreatedAt,
	}
}

var _ conversation.Repository = (*ConversationRepo)(nil)
