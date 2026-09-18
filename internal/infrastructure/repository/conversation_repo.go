package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"SWPUCAT/internal/domain/conversation"
	"SWPUCAT/internal/infrastructure/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
		TitleCustomized: item.TitleCustomized,
		RollingSummary:  item.RollingSummary,
		LastSourcesJSON: item.LastSourcesJSON,
		LastActiveAt:    item.LastActiveAt,
		CreatedAt:       item.CreatedAt,
		UpdatedAt:       item.UpdatedAt,
	}
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tombstones int64
		if err := tx.Table("agent_conversation_tombstones").
			Where("user_id = ? AND session_hash = ?", item.UserID, conversationSessionHash(item.SessionID)).
			Count(&tombstones).Error; err != nil {
			return err
		}
		if tombstones > 0 {
			return conversation.ErrDeletedConversation
		}
		return tx.Create(model).Error
	}); err != nil {
		return err
	}
	item.ID = model.ID
	item.CreatedAt = model.CreatedAt
	item.UpdatedAt = model.UpdatedAt
	item.LastActiveAt = model.LastActiveAt
	return nil
}

func (r *ConversationRepo) DeleteOwned(ctx context.Context, userID int64, sessionID string, deletedAt time.Time) error {
	if r == nil || r.db == nil || userID <= 0 || sessionID == "" || deletedAt.IsZero() {
		return conversation.ErrInvalidConversation
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record database.AgentConversationModel
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "user_id", "session_id").
			Where("user_id = ? AND session_id = ?", userID, sessionID).
			First(&record).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := insertConversationTombstone(tx, record.UserID, record.SessionID, "user", deletedAt); err != nil {
			return err
		}
		if err := detachConversationAudits(tx, []int64{record.ID}); err != nil {
			return err
		}
		result := tx.Where("id = ? AND user_id = ?", record.ID, userID).Delete(&database.AgentConversationModel{})
		if result.Error != nil {
			return result.Error
		}
		return nil
	})
}

func (r *ConversationRepo) CountExpired(ctx context.Context, before time.Time) (int64, error) {
	if r == nil || r.db == nil || before.IsZero() {
		return 0, conversation.ErrInvalidConversation
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&database.AgentConversationModel{}).
		Where("last_active_at < ?", before).Count(&count).Error
	return count, err
}

func (r *ConversationRepo) DeleteExpired(ctx context.Context, before time.Time, limit int, deletedAt time.Time) (int64, error) {
	if r == nil || r.db == nil || before.IsZero() || deletedAt.IsZero() || limit < 1 {
		return 0, conversation.ErrInvalidConversation
	}
	var deleted int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var records []database.AgentConversationModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Select("id", "user_id", "session_id").
			Where("last_active_at < ?", before).
			Order("last_active_at ASC, id ASC").
			Limit(limit).
			Find(&records).Error; err != nil {
			return err
		}
		if len(records) == 0 {
			return nil
		}
		ids := make([]int64, 0, len(records))
		for _, record := range records {
			if err := insertConversationTombstone(tx, record.UserID, record.SessionID, "retention", deletedAt); err != nil {
				return err
			}
			ids = append(ids, record.ID)
		}
		if err := detachConversationAudits(tx, ids); err != nil {
			return err
		}
		result := tx.Where("id IN ? AND last_active_at < ?", ids, before).Delete(&database.AgentConversationModel{})
		if result.Error != nil {
			return result.Error
		}
		deleted = result.RowsAffected
		return nil
	})
	return deleted, err
}

func insertConversationTombstone(tx *gorm.DB, userID int64, sessionID, reason string, deletedAt time.Time) error {
	return tx.Exec(`
		INSERT INTO agent_conversation_tombstones (user_id, session_hash, reason, deleted_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id, session_hash) DO UPDATE SET
			reason = EXCLUDED.reason,
			deleted_at = EXCLUDED.deleted_at`,
		userID, conversationSessionHash(sessionID), reason, deletedAt).Error
}

func detachConversationAudits(tx *gorm.DB, conversationIDs []int64) error {
	if len(conversationIDs) == 0 {
		return nil
	}
	return tx.Table("agent_runs").Where("conversation_id IN ?", conversationIDs).
		Updates(map[string]interface{}{"conversation_id": nil, "session_id": nil}).Error
}

func conversationSessionHash(sessionID string) string {
	digest := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(digest[:])
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

func (r *ConversationRepo) ListRecent(ctx context.Context, userID int64, limit int) ([]*conversation.Conversation, error) {
	if r == nil || r.db == nil || userID <= 0 || limit < 1 {
		return nil, conversation.ErrInvalidConversation
	}
	var models []database.AgentConversationModel
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("last_active_at DESC, id DESC").
		Limit(limit).
		Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]*conversation.Conversation, 0, len(models))
	for i := range models {
		items = append(items, toDomainConversation(&models[i]))
	}
	return items, nil
}

func (r *ConversationRepo) UpdateTitle(ctx context.Context, userID int64, sessionID, title string) error {
	if r == nil || r.db == nil || userID <= 0 || sessionID == "" || title == "" {
		return conversation.ErrInvalidConversation
	}
	result := r.db.WithContext(ctx).
		Model(&database.AgentConversationModel{}).
		Where("user_id = ? AND session_id = ?", userID, sessionID).
		Updates(map[string]interface{}{
			"title":            title,
			"title_customized": true,
			"updated_at":       time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
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

func (r *ConversationRepo) RecordTurn(ctx context.Context, userID int64, sessionID, title string, titleCustomized bool, userMessage, assistantMessage *conversation.Message, summary, lastSourcesJSON string, activeAt time.Time) error {
	if r == nil || r.db == nil || userID <= 0 || userMessage == nil || assistantMessage == nil || activeAt.IsZero() {
		return conversation.ErrInvalidMessage
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record database.AgentConversationModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND session_id = ?", userID, sessionID).First(&record).Error; err != nil {
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
			"title":             title,
			"title_customized":  titleCustomized,
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

func (r *ConversationRepo) FindTurnByClientID(ctx context.Context, userID int64, sessionID, clientTurnID string) (*conversation.Message, *conversation.Message, error) {
	if r == nil || r.db == nil || userID <= 0 || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(clientTurnID) == "" {
		return nil, nil, conversation.ErrInvalidMessage
	}
	var models []database.AgentMessageModel
	err := r.db.WithContext(ctx).
		Table((database.AgentMessageModel{}).TableName()).
		Select("agent_messages.*").
		Joins("JOIN agent_conversations ON agent_conversations.id = agent_messages.conversation_id").
		Where("agent_conversations.user_id = ? AND agent_conversations.session_id = ? AND agent_messages.client_turn_id = ?", userID, sessionID, clientTurnID).
		Where("agent_messages.role IN ?", []string{string(conversation.RoleUser), string(conversation.RoleAssistant)}).
		Order("agent_messages.id ASC").
		Find(&models).Error
	if err != nil {
		return nil, nil, err
	}
	var userMessage, assistantMessage *conversation.Message
	for index := range models {
		message := toDomainMessage(&models[index])
		switch message.Role {
		case conversation.RoleUser:
			userMessage = message
		case conversation.RoleAssistant:
			assistantMessage = message
		}
	}
	if userMessage == nil {
		return nil, nil, conversation.ErrTurnNotFound
	}
	return userMessage, assistantMessage, nil
}

func (r *ConversationRepo) CommitTurn(ctx context.Context, userID int64, sessionID, title string, titleCustomized bool, userMessage, assistantMessage *conversation.Message, summary, lastSourcesJSON string, activeAt time.Time, regenerate bool) error {
	if r == nil || r.db == nil || userID <= 0 || userMessage == nil || assistantMessage == nil || activeAt.IsZero() ||
		strings.TrimSpace(userMessage.ClientTurnID) == "" || userMessage.ClientTurnID != assistantMessage.ClientTurnID ||
		strings.TrimSpace(assistantMessage.GenerationID) == "" || strings.TrimSpace(assistantMessage.RequestID) == "" {
		return conversation.ErrInvalidMessage
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record database.AgentConversationModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND session_id = ?", userID, sessionID).First(&record).Error; err != nil {
			return err
		}

		var stored []database.AgentMessageModel
		if err := tx.Where("conversation_id = ? AND client_turn_id = ?", record.ID, userMessage.ClientTurnID).
			Where("role IN ?", []string{string(conversation.RoleUser), string(conversation.RoleAssistant)}).
			Order("id ASC").Find(&stored).Error; err != nil {
			return err
		}
		var storedUser, storedAssistant *database.AgentMessageModel
		for index := range stored {
			switch stored[index].Role {
			case string(conversation.RoleUser):
				storedUser = &stored[index]
			case string(conversation.RoleAssistant):
				storedAssistant = &stored[index]
			}
		}
		if storedUser != nil && storedUser.Content != userMessage.Content {
			return conversation.ErrTurnConflict
		}
		if storedAssistant != nil && storedUser == nil {
			return conversation.ErrInvalidMessage
		}
		if regenerate && storedUser == nil {
			return conversation.ErrTurnNotFound
		}
		if storedAssistant != nil {
			storedGenerationID := stringValue(storedAssistant.GenerationID)
			if storedGenerationID == assistantMessage.GenerationID {
				*userMessage = *toDomainMessage(storedUser)
				*assistantMessage = *toDomainMessage(storedAssistant)
				return nil
			}
			if !regenerate {
				return conversation.ErrTurnConflict
			}
		}

		if storedUser == nil {
			if userMessage.ConversationID != record.ID {
				return conversation.ErrInvalidMessage
			}
			model := agentMessageModel(userMessage)
			if err := tx.Create(model).Error; err != nil {
				return err
			}
			userMessage.ID = model.ID
			userMessage.CreatedAt = model.CreatedAt
		} else {
			*userMessage = *toDomainMessage(storedUser)
		}

		if storedAssistant == nil {
			if assistantMessage.ConversationID != record.ID {
				return conversation.ErrInvalidMessage
			}
			model := agentMessageModel(assistantMessage)
			if err := tx.Create(model).Error; err != nil {
				return err
			}
			assistantMessage.ID = model.ID
			assistantMessage.CreatedAt = model.CreatedAt
		} else {
			if err := tx.Where("message_id = ?", storedAssistant.ID).Delete(&database.AgentResponseFeedbackModel{}).Error; err != nil {
				return err
			}
			result := tx.Model(&database.AgentMessageModel{}).Where("id = ? AND conversation_id = ? AND role = ?", storedAssistant.ID, record.ID, string(conversation.RoleAssistant)).Updates(map[string]interface{}{
				"content":        assistantMessage.Content,
				"tool_name":      assistantMessage.ToolName,
				"tool_result":    assistantMessage.ToolResult,
				"citations_json": assistantMessage.CitationsJSON,
				"token_count":    assistantMessage.TokenCount,
				"generation_id":  assistantMessage.GenerationID,
				"request_id":     assistantMessage.RequestID,
				"created_at":     assistantMessage.CreatedAt,
			})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return conversation.ErrTurnNotFound
			}
			assistantMessage.ID = storedAssistant.ID
		}

		result := tx.Model(&database.AgentConversationModel{}).Where("id = ? AND user_id = ?", record.ID, userID).Updates(map[string]interface{}{
			"title": title, "title_customized": titleCustomized, "rolling_summary": summary,
			"last_sources_json": lastSourcesJSON, "last_active_at": activeAt, "updated_at": activeAt,
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

func (r *ConversationRepo) SaveResponseFeedback(ctx context.Context, feedback *conversation.ResponseFeedback) error {
	if r == nil || r.db == nil || feedback == nil || feedback.UserID <= 0 || feedback.MessageID <= 0 ||
		strings.TrimSpace(feedback.RequestID) == "" || feedback.CreatedAt.IsZero() || feedback.UpdatedAt.IsZero() {
		return conversation.ErrInvalidFeedback
	}
	result := r.db.WithContext(ctx).Exec(`
		INSERT INTO agent_response_feedback (user_id, message_id, request_id, helpful, note, created_at, updated_at)
		SELECT ?, messages.id, ?, ?, ?, ?, ?
		FROM agent_messages AS messages
		JOIN agent_conversations AS conversations ON conversations.id = messages.conversation_id
		JOIN agent_runs AS runs ON runs.request_id = ?
			AND runs.user_id = conversations.user_id
			AND runs.conversation_id = conversations.id
		WHERE conversations.user_id = ?
			AND messages.id = ?
			AND messages.role = 'assistant'
			AND messages.request_id = ?
		ON CONFLICT (user_id, message_id) DO UPDATE SET
			request_id = EXCLUDED.request_id,
			helpful = EXCLUDED.helpful,
			note = EXCLUDED.note,
			updated_at = EXCLUDED.updated_at`,
		feedback.UserID, feedback.RequestID, feedback.Helpful, feedback.Note, feedback.CreatedAt, feedback.UpdatedAt,
		feedback.RequestID, feedback.UserID, feedback.MessageID, feedback.RequestID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return conversation.ErrInvalidFeedback
	}
	return nil
}

func agentMessageModel(message *conversation.Message) *database.AgentMessageModel {
	return &database.AgentMessageModel{
		ConversationID: message.ConversationID, Role: string(message.Role), Content: message.Content,
		ToolName: message.ToolName, ToolResult: message.ToolResult, CitationsJSON: message.CitationsJSON,
		TokenCount: message.TokenCount, ClientTurnID: stringPointer(message.ClientTurnID),
		GenerationID: stringPointer(message.GenerationID), RequestID: stringPointer(message.RequestID), CreatedAt: message.CreatedAt,
	}
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func toDomainConversation(model *database.AgentConversationModel) *conversation.Conversation {
	return &conversation.Conversation{
		ID:              model.ID,
		UserID:          model.UserID,
		SessionID:       model.SessionID,
		Title:           model.Title,
		TitleCustomized: model.TitleCustomized,
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
		ClientTurnID:   stringValue(model.ClientTurnID),
		GenerationID:   stringValue(model.GenerationID),
		RequestID:      stringValue(model.RequestID),
		CreatedAt:      model.CreatedAt,
	}
}

var _ conversation.Repository = (*ConversationRepo)(nil)
var _ conversation.LifecycleRepository = (*ConversationRepo)(nil)
var _ conversation.TurnRepository = (*ConversationRepo)(nil)
