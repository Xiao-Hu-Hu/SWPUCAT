package repository

import (
	"context"
	"errors"
	"time"

	agentauditapp "SWPUCAT/internal/application/agentaudit"
	"SWPUCAT/internal/domain/agentaudit"
	"SWPUCAT/internal/infrastructure/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AgentAuditRepo appends request-level Agent audits and their tool calls. A
// request identifier is first-write-wins: replaying the same trusted ID never
// changes the original audit record.
type AgentAuditRepo struct {
	db *gorm.DB
}

func NewAgentAuditRepo(db *gorm.DB) *AgentAuditRepo {
	return &AgentAuditRepo{db: db}
}

func (r *AgentAuditRepo) CreateRun(ctx context.Context, run *agentaudit.Run) error {
	if r == nil || r.db == nil || run == nil {
		return agentaudit.ErrInvalidRun
	}
	// Validate the aggregate shape before using its request ID as a lookup key.
	// Persistence identities are checked after the winner lookup so the exact
	// aggregate returned by a successful first call can be retried.
	if err := agentauditapp.ValidatePersistenceContract(run); err != nil {
		return err
	}

	var resultRun *agentaudit.Run
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Serialize contenders for the same trusted request ID before checking
		// either the winner or conversation ownership. Without this lock, a
		// concurrent loser can miss an uncommitted winner and fail an ownership
		// check instead of returning the first writer's immutable aggregate.
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, run.RequestID).Error; err != nil {
			return err
		}
		winner, found, err := loadAgentAuditWinner(tx, run.RequestID)
		if err != nil {
			return err
		}
		if found {
			if err := validateAgentAuditReplayIdentity(run, winner); err != nil {
				return err
			}
			resultRun = winner
			return nil
		}

		// No winner exists, so this must be a genuinely new aggregate. Reject
		// every caller-supplied persistence identity before any ownership lookup
		// or insert can be influenced by it.
		if err := run.ValidateForCreate(); err != nil {
			return err
		}
		if err := validateAgentAuditConversationOwnership(tx, run); err != nil {
			return err
		}

		persistedRun := agentRunModelFromDomain(run)
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "request_id"}},
			DoNothing: true,
		}).Create(&persistedRun)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			// A concurrent writer won after the initial lookup. Load its complete
			// aggregate and never append this caller's children to it.
			winner, found, err := loadAgentAuditWinner(tx, run.RequestID)
			if err != nil {
				return err
			}
			if !found {
				return gorm.ErrRecordNotFound
			}
			resultRun = winner
			return nil
		}

		persistedTools := make([]database.AgentToolCallModel, 0, len(run.ToolCalls))
		for i := range run.ToolCalls {
			persistedTools = append(persistedTools, agentToolCallModelFromDomain(persistedRun.ID, run.CreatedAt, &run.ToolCalls[i]))
		}
		if len(persistedTools) > 0 {
			if err := tx.Create(&persistedTools).Error; err != nil {
				return err
			}
		}
		// Always return the database representation, including on the first
		// successful insert. PostgreSQL normalizes timestamptz values to
		// microsecond precision, and the winner query also establishes the one
		// canonical child ordering used by every replay.
		winner, found, err = loadAgentAuditWinner(tx, run.RequestID)
		if err != nil {
			return err
		}
		if !found {
			return gorm.ErrRecordNotFound
		}
		resultRun = winner
		return nil
	})
	if err != nil {
		return err
	}
	if resultRun == nil {
		return gorm.ErrRecordNotFound
	}
	*run = *resultRun
	return nil
}

func (r *AgentAuditRepo) ListRuns(ctx context.Context, filter agentaudit.ReadFilter) ([]agentaudit.Run, int64, error) {
	if r == nil || r.db == nil || filter.Page < 1 || filter.PageSize < 1 {
		return nil, 0, agentaudit.ErrInvalidRun
	}
	query := applyAgentAuditReadFilter(r.db.WithContext(ctx).Model(&database.AgentRunModel{}), filter)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var models []database.AgentRunModel
	if err := query.Select(
		"id", "request_id", "user_id", "conversation_id", "session_id", "actor_role", "status", "telemetry_status",
		"http_status", "failure_source", "failure_category", "failure_code", "failure_retryable", "started_at", "finished_at", "duration_ms",
		"agent_started_at", "agent_finished_at", "agent_duration_ms", "conversation_persisted", "model_call_count", "tool_call_count",
		"dropped_model_calls", "dropped_tool_calls", "usage_reported_model_calls", "usage_missing_model_calls", "prompt_tokens",
		"completion_tokens", "total_tokens", "cached_tokens", "reasoning_tokens", "created_at",
	).Order("created_at DESC, id DESC").
		Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&models).Error; err != nil {
		return nil, 0, err
	}
	result := make([]agentaudit.Run, 0, len(models))
	for i := range models {
		result = append(result, *agentAuditRunFromModels(&models[i], nil))
	}
	return result, total, nil
}

func (r *AgentAuditRepo) FindRun(ctx context.Context, id int64) (*agentaudit.Run, error) {
	if r == nil || r.db == nil || id <= 0 {
		return nil, agentaudit.ErrInvalidRun
	}
	var run database.AgentRunModel
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&run).Error; err != nil {
		return nil, err
	}
	var tools []database.AgentToolCallModel
	if err := r.db.WithContext(ctx).Where("run_id = ?", id).Order("sequence ASC").Find(&tools).Error; err != nil {
		return nil, err
	}
	result := agentAuditRunFromModels(&run, tools)
	if err := agentauditapp.ValidatePersistenceContract(result); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *AgentAuditRepo) SummarizeRuns(ctx context.Context, filter agentaudit.ReadFilter) (agentaudit.ReadSummary, error) {
	result := agentaudit.ReadSummary{StatusCounts: map[string]int64{}, ToolCounts: map[string]int64{}}
	if r == nil || r.db == nil {
		return result, agentaudit.ErrInvalidRun
	}
	var runs []database.AgentRunModel
	query := applyAgentAuditReadFilter(r.db.WithContext(ctx).Model(&database.AgentRunModel{}), filter)
	if err := query.Select("id", "status", "duration_ms", "prompt_tokens", "completion_tokens", "total_tokens", "cached_tokens").Find(&runs).Error; err != nil {
		return result, err
	}
	runIDs := make([]int64, 0, len(runs))
	for i := range runs {
		run := &runs[i]
		runIDs = append(runIDs, run.ID)
		result.TotalRuns++
		result.StatusCounts[run.Status]++
		result.TotalDurationMS += run.DurationMS
		result.RunsWithDuration++
		if run.PromptTokens != nil && run.CompletionTokens != nil && run.TotalTokens != nil {
			result.PromptTokens += *run.PromptTokens
			result.CompletionTokens += *run.CompletionTokens
			result.TotalTokens += *run.TotalTokens
			if run.CachedTokens != nil {
				result.CachedTokens += *run.CachedTokens
			}
			result.RunsWithTokenUsage++
		}
	}
	if len(runIDs) == 0 {
		return result, nil
	}
	type toolCount struct {
		Name  string `gorm:"column:tool_name"`
		Count int64  `gorm:"column:count"`
	}
	var counts []toolCount
	if err := r.db.WithContext(ctx).Model(&database.AgentToolCallModel{}).
		Select("tool_name, COUNT(*) AS count").Where("run_id IN ?", runIDs).
		Group("tool_name").Order("tool_name ASC").Scan(&counts).Error; err != nil {
		return result, err
	}
	for _, item := range counts {
		result.ToolCounts[item.Name] = item.Count
	}
	return result, nil
}

func applyAgentAuditReadFilter(query *gorm.DB, filter agentaudit.ReadFilter) *gorm.DB {
	query = query.Where("created_at >= ? AND created_at < ?", filter.From, filter.To)
	if filter.Status != "" {
		query = query.Where("status = ?", string(filter.Status))
	}
	return query
}

// loadAgentAuditWinner reads the complete immutable aggregate. Tool calls are
// always returned in canonical sequence order so every idempotent caller sees
// the same database fact regardless of its attempted input ordering.
func loadAgentAuditWinner(tx *gorm.DB, requestID string) (*agentaudit.Run, bool, error) {
	var persistedRun database.AgentRunModel
	if err := tx.Where("request_id = ?", requestID).Take(&persistedRun).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}

	var persistedTools []database.AgentToolCallModel
	if err := tx.Where("run_id = ?", persistedRun.ID).Order("sequence ASC").Find(&persistedTools).Error; err != nil {
		return nil, false, err
	}
	winner := agentAuditRunFromModels(&persistedRun, persistedTools)
	// JSON columns preserve the exact bounded projection text, so a loaded
	// winner must still satisfy the same privacy and consistency gate that ran
	// before insertion. This also catches manual or legacy storage corruption.
	if err := agentauditapp.ValidatePersistenceContract(winner); err != nil {
		return nil, false, err
	}
	return winner, true, nil
}

// validateAgentAuditReplayIdentity accepts either a fresh all-zero aggregate
// that lost the request-ID race, or the exact persistence identities populated
// by an earlier successful CreateRun call. Partial, mismatched, or invented
// identities remain invalid.
func validateAgentAuditReplayIdentity(candidate, winner *agentaudit.Run) error {
	if candidate == nil || winner == nil {
		return agentaudit.ErrInvalidRun
	}
	if candidate.ID == 0 {
		for i := range candidate.ToolCalls {
			if candidate.ToolCalls[i].ID != 0 || candidate.ToolCalls[i].RunID != 0 {
				return agentaudit.ErrInvalidToolCall
			}
		}
		return nil
	}
	if candidate.ID != winner.ID {
		return agentaudit.ErrInvalidRun
	}
	if len(candidate.ToolCalls) != len(winner.ToolCalls) {
		return agentaudit.ErrInvalidToolCall
	}

	winnerTools := make(map[int64]agentaudit.ToolCall, len(winner.ToolCalls))
	for i := range winner.ToolCalls {
		winnerTools[winner.ToolCalls[i].Sequence] = winner.ToolCalls[i]
	}
	for i := range candidate.ToolCalls {
		persisted, ok := winnerTools[candidate.ToolCalls[i].Sequence]
		if !ok || candidate.ToolCalls[i].ID == 0 || candidate.ToolCalls[i].ID != persisted.ID || candidate.ToolCalls[i].RunID != winner.ID {
			return agentaudit.ErrInvalidToolCall
		}
	}
	return nil
}

func validateAgentAuditConversationOwnership(tx *gorm.DB, run *agentaudit.Run) error {
	if run.ConversationID == nil {
		return nil
	}
	if run.UserID == nil || run.SessionID == nil {
		return agentaudit.ErrConversationOwnershipMismatch
	}
	var owner database.AgentConversationModel
	err := tx.Select("id").
		Where("id = ? AND user_id = ? AND session_id = ?", *run.ConversationID, *run.UserID, *run.SessionID).
		Take(&owner).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return agentaudit.ErrConversationOwnershipMismatch
	}
	return err
}

func agentRunModelFromDomain(run *agentaudit.Run) database.AgentRunModel {
	return database.AgentRunModel{
		ID:                      run.ID,
		RequestID:               run.RequestID,
		UserID:                  run.UserID,
		ConversationID:          run.ConversationID,
		SessionID:               run.SessionID,
		ActorRole:               run.ActorRole,
		Status:                  string(run.Status),
		TelemetryStatus:         string(run.TelemetryStatus),
		HTTPStatus:              run.HTTPStatus,
		FailureSource:           run.FailureSource,
		FailureCategory:         run.FailureCategory,
		FailureCode:             run.FailureCode,
		FailureRetryable:        run.FailureRetryable,
		StartedAt:               run.StartedAt,
		FinishedAt:              run.FinishedAt,
		DurationMS:              run.DurationMS,
		AgentStartedAt:          run.AgentStartedAt,
		AgentFinishedAt:         run.AgentFinishedAt,
		AgentDurationMS:         run.AgentDurationMS,
		ConversationPersisted:   run.ConversationPersisted,
		ModelCallCount:          run.ModelCallCount,
		ToolCallCount:           run.ToolCallCount,
		DroppedModelCalls:       run.DroppedModelCalls,
		DroppedToolCalls:        run.DroppedToolCalls,
		UsageReportedModelCalls: run.ReportedModelCalls,
		UsageMissingModelCalls:  run.MissingModelCalls,
		PromptTokens:            run.PromptTokens,
		CompletionTokens:        run.CompletionTokens,
		TotalTokens:             run.TotalTokens,
		CachedTokens:            run.CachedTokens,
		ReasoningTokens:         run.ReasoningTokens,
		UsageJSON:               run.UsageJSON,
		ModelCallsJSON:          run.ModelCallsJSON,
		CreatedAt:               run.CreatedAt,
	}
}

func agentAuditRunFromModels(run *database.AgentRunModel, tools []database.AgentToolCallModel) *agentaudit.Run {
	result := &agentaudit.Run{
		ID:                    run.ID,
		RequestID:             run.RequestID,
		UserID:                run.UserID,
		ConversationID:        run.ConversationID,
		SessionID:             run.SessionID,
		ActorRole:             run.ActorRole,
		Status:                agentaudit.RunStatus(run.Status),
		TelemetryStatus:       agentaudit.TelemetryStatus(run.TelemetryStatus),
		HTTPStatus:            run.HTTPStatus,
		FailureSource:         run.FailureSource,
		FailureCategory:       run.FailureCategory,
		FailureCode:           run.FailureCode,
		FailureRetryable:      run.FailureRetryable,
		StartedAt:             run.StartedAt,
		FinishedAt:            run.FinishedAt,
		DurationMS:            run.DurationMS,
		AgentStartedAt:        run.AgentStartedAt,
		AgentFinishedAt:       run.AgentFinishedAt,
		AgentDurationMS:       run.AgentDurationMS,
		ConversationPersisted: run.ConversationPersisted,
		ModelCallCount:        run.ModelCallCount,
		ToolCallCount:         run.ToolCallCount,
		DroppedModelCalls:     run.DroppedModelCalls,
		DroppedToolCalls:      run.DroppedToolCalls,
		ReportedModelCalls:    run.UsageReportedModelCalls,
		MissingModelCalls:     run.UsageMissingModelCalls,
		PromptTokens:          run.PromptTokens,
		CompletionTokens:      run.CompletionTokens,
		TotalTokens:           run.TotalTokens,
		CachedTokens:          run.CachedTokens,
		ReasoningTokens:       run.ReasoningTokens,
		UsageJSON:             run.UsageJSON,
		ModelCallsJSON:        run.ModelCallsJSON,
		ToolCalls:             make([]agentaudit.ToolCall, len(tools)),
		CreatedAt:             run.CreatedAt,
	}
	for i := range tools {
		result.ToolCalls[i] = agentAuditToolCallFromModel(&tools[i])
	}
	return result
}

func agentToolCallModelFromDomain(runID int64, fallbackCreatedAt time.Time, call *agentaudit.ToolCall) database.AgentToolCallModel {
	createdAt := call.CreatedAt
	if createdAt.IsZero() {
		createdAt = fallbackCreatedAt
	}
	return database.AgentToolCallModel{
		ID:                     call.ID,
		RunID:                  runID,
		Sequence:               call.Sequence,
		CallID:                 call.CallID,
		ToolName:               call.Name,
		IsWrite:                call.IsWrite,
		Status:                 call.Status,
		StartedAt:              call.StartedAt,
		DurationMS:             call.DurationMS,
		ArgumentsJSON:          call.ArgumentsJSON,
		ArgumentsOriginalBytes: call.ArgumentsOriginalBytes,
		ArgumentsCapturedBytes: int(call.ArgumentsCapturedBytes),
		ArgumentsRedacted:      call.ArgumentsRedacted,
		ArgumentsTruncated:     call.ArgumentsTruncated,
		ResultJSON:             call.ResultJSON,
		ResultOriginalBytes:    call.ResultOriginalBytes,
		ResultCapturedBytes:    int(call.ResultCapturedBytes),
		ResultRedacted:         call.ResultRedacted,
		ResultTruncated:        call.ResultTruncated,
		ErrorCategory:          call.ErrorCategory,
		ErrorCode:              call.ErrorCode,
		ErrorType:              call.ErrorType,
		ErrorHTTPStatus:        call.ErrorHTTPStatus,
		ErrorRetryable:         call.ErrorRetryable,
		BusinessObjectType:     call.BusinessObjectType,
		BusinessObjectID:       call.BusinessObjectID,
		BusinessObjectStatus:   call.BusinessObjectStatus,
		CreatedAt:              createdAt,
	}
}

func agentAuditToolCallFromModel(call *database.AgentToolCallModel) agentaudit.ToolCall {
	return agentaudit.ToolCall{
		ID:                     call.ID,
		RunID:                  call.RunID,
		Sequence:               call.Sequence,
		CallID:                 call.CallID,
		Name:                   call.ToolName,
		IsWrite:                call.IsWrite,
		Status:                 call.Status,
		StartedAt:              call.StartedAt,
		DurationMS:             call.DurationMS,
		ArgumentsJSON:          call.ArgumentsJSON,
		ArgumentsOriginalBytes: call.ArgumentsOriginalBytes,
		ArgumentsCapturedBytes: int64(call.ArgumentsCapturedBytes),
		ArgumentsRedacted:      call.ArgumentsRedacted,
		ArgumentsTruncated:     call.ArgumentsTruncated,
		ResultJSON:             call.ResultJSON,
		ResultOriginalBytes:    call.ResultOriginalBytes,
		ResultCapturedBytes:    int64(call.ResultCapturedBytes),
		ResultRedacted:         call.ResultRedacted,
		ResultTruncated:        call.ResultTruncated,
		ErrorCategory:          call.ErrorCategory,
		ErrorCode:              call.ErrorCode,
		ErrorType:              call.ErrorType,
		ErrorHTTPStatus:        call.ErrorHTTPStatus,
		ErrorRetryable:         call.ErrorRetryable,
		BusinessObjectType:     call.BusinessObjectType,
		BusinessObjectID:       call.BusinessObjectID,
		BusinessObjectStatus:   call.BusinessObjectStatus,
		CreatedAt:              call.CreatedAt,
	}
}

var _ agentaudit.Repository = (*AgentAuditRepo)(nil)
