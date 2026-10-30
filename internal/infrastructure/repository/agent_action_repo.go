package repository

import (
	"context"
	"errors"
	"time"

	"SWPUCAT/internal/application/agentaction"
	"SWPUCAT/internal/domain/shared"
	"SWPUCAT/internal/infrastructure/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AgentActionRepo struct{ db *gorm.DB }

func NewAgentActionRepo(db *gorm.DB) *AgentActionRepo { return &AgentActionRepo{db: db} }

func (r *AgentActionRepo) Claim(ctx context.Context, record *agentaction.Record) (*agentaction.Record, bool, error) {
	if r == nil || r.db == nil || record == nil {
		return nil, false, shared.ErrValidation
	}
	model := actionRecordToModel(record)
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(model)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 1 {
		return record, true, nil
	}
	var existing database.AgentActionConfirmationModel
	if err := r.db.WithContext(ctx).Where("token_hash = ?", record.TokenHash).First(&existing).Error; err != nil {
		return nil, false, err
	}
	return actionModelToRecord(&existing), false, nil
}

func (r *AgentActionRepo) FindLatest(ctx context.Context, userID int64, sessionID, toolName, parametersDigest string) (*agentaction.Record, error) {
	if r == nil || r.db == nil || userID <= 0 || sessionID == "" || toolName == "" || parametersDigest == "" {
		return nil, shared.ErrValidation
	}
	var model database.AgentActionConfirmationModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND session_id = ? AND tool_name = ? AND parameters_digest = ?",
			userID, sessionID, toolName, parametersDigest).
		Order("created_at DESC").First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return actionModelToRecord(&model), nil
}

func (r *AgentActionRepo) Complete(ctx context.Context, tokenHash, status, resultJSON, errorCode string, completedAt time.Time) error {
	if r == nil || r.db == nil || tokenHash == "" || (status != "succeeded" && status != "failed") || completedAt.IsZero() {
		return shared.ErrValidation
	}
	updates := map[string]any{"status": status, "result_json": resultJSON, "completed_at": completedAt, "error_code": nil}
	if errorCode != "" {
		updates["error_code"] = errorCode
	}
	result := r.db.WithContext(ctx).Model(&database.AgentActionConfirmationModel{}).
		Where("token_hash = ? AND status = ?", tokenHash, "executing").Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return shared.ErrConflict
	}
	return nil
}

func actionRecordToModel(record *agentaction.Record) *database.AgentActionConfirmationModel {
	var errorCode *string
	if record.ErrorCode != "" {
		copy := record.ErrorCode
		errorCode = &copy
	}
	return &database.AgentActionConfirmationModel{
		TokenHash: record.TokenHash, UserID: record.UserID, SessionID: record.SessionID, ToolName: record.ToolName,
		ParametersDigest: record.ParametersDigest, Status: record.Status, ResultJSON: record.ResultJSON,
		ErrorCode: errorCode, ExpiresAt: record.ExpiresAt, CreatedAt: record.CreatedAt, CompletedAt: record.CompletedAt,
	}
}

func actionModelToRecord(model *database.AgentActionConfirmationModel) *agentaction.Record {
	record := &agentaction.Record{
		TokenHash: model.TokenHash, UserID: model.UserID, SessionID: model.SessionID, ToolName: model.ToolName,
		ParametersDigest: model.ParametersDigest, Status: model.Status, ResultJSON: model.ResultJSON,
		ExpiresAt: model.ExpiresAt, CreatedAt: model.CreatedAt, CompletedAt: model.CompletedAt,
	}
	if model.ErrorCode != nil {
		record.ErrorCode = *model.ErrorCode
	}
	return record
}

var _ agentaction.Repository = (*AgentActionRepo)(nil)
