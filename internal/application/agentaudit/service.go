package agentauditapp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"SWPUCAT/internal/agent/telemetry"
	"SWPUCAT/internal/domain/agentaudit"
)

const (
	maxPersistedModelCalls      = agentaudit.MaxPersistedModelCalls
	maxPersistedToolCalls       = agentaudit.MaxPersistedToolCalls
	maxPersistedToolRequests    = 32
	maxTelemetryCount           = agentaudit.MaxTelemetryCount
	maxArgumentProjectionBytes  = 2 << 10
	maxResultProjectionBytes    = 4 << 10
	maxPayloadProjectionItems   = 20
	maxPayloadProjectionDepth   = 6
	maxPersistedIdentifierRunes = 128
)

type persistedPayloadKind uint8

const (
	persistedPayloadArguments persistedPayloadKind = iota
	persistedPayloadResult
)

var (
	safeAliasPattern      = regexp.MustCompile(`^(call|response)-[1-9][0-9]{0,19}$`)
	safeIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	periodPattern         = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)
)

// Failure is intentionally machine-readable. Human error messages, provider
// response bodies and request parameters are not part of the durable audit
// boundary.
type Failure struct {
	Source    string
	Category  string
	Code      string
	Retryable *bool
}

type RecordRequest struct {
	RequestID             string
	UserID                *int64
	ConversationID        *int64
	SessionID             *string
	ActorRole             *string
	Status                agentaudit.RunStatus
	HTTPStatus            *int
	Failure               *Failure
	StartedAt             time.Time
	FinishedAt            time.Time
	ConversationPersisted *bool
	TelemetryStatus       agentaudit.TelemetryStatus
	Snapshot              *telemetry.Snapshot
}

type Service struct {
	repo agentaudit.Repository
}

func NewService(repo agentaudit.Repository) *Service {
	return &Service{repo: repo}
}

// Record converts a private Agent snapshot into the deliberately smaller
// persistence contract. A malformed snapshot is recorded as invalid rather
// than returned to callers or partially trusted.
func (s *Service) Record(ctx context.Context, req RecordRequest) (*agentaudit.Run, error) {
	if s == nil || s.repo == nil {
		return nil, agentaudit.ErrInvalidRun
	}
	meta := agentaudit.RunMetadata{
		RequestID:             strings.TrimSpace(req.RequestID),
		UserID:                cloneInt64(req.UserID),
		ConversationID:        cloneInt64(req.ConversationID),
		SessionID:             cloneTrimmedString(req.SessionID),
		ActorRole:             cloneTrimmedString(req.ActorRole),
		Status:                req.Status,
		TelemetryStatus:       req.TelemetryStatus,
		HTTPStatus:            cloneInt(req.HTTPStatus),
		StartedAt:             req.StartedAt,
		FinishedAt:            req.FinishedAt,
		ConversationPersisted: cloneBool(req.ConversationPersisted),
	}
	if req.Failure != nil {
		meta.FailureSource = optionalSafeIdentifier(req.Failure.Source, 32)
		meta.FailureCategory = optionalSafeIdentifier(req.Failure.Category, 32)
		meta.FailureCode = optionalSafeIdentifier(req.Failure.Code, 64)
		meta.FailureRetryable = cloneBool(req.Failure.Retryable)
		if strings.TrimSpace(req.Failure.Source) != "" && meta.FailureSource == nil ||
			strings.TrimSpace(req.Failure.Category) != "" && meta.FailureCategory == nil ||
			strings.TrimSpace(req.Failure.Code) != "" && meta.FailureCode == nil {
			return nil, agentaudit.ErrInvalidRun
		}
	}

	run, err := agentaudit.NewRun(meta)
	if err != nil {
		return nil, err
	}
	if req.Snapshot != nil {
		if err := projectSnapshot(run, req.Snapshot); err != nil {
			clearTelemetry(run, agentaudit.TelemetryInvalid)
		}
	} else {
		switch req.TelemetryStatus {
		case agentaudit.TelemetryNotStarted:
			setKnownZeroTelemetry(run)
		case agentaudit.TelemetryUnavailable, agentaudit.TelemetryInvalid:
			clearTelemetry(run, req.TelemetryStatus)
		default:
			clearTelemetry(run, agentaudit.TelemetryInvalid)
		}
	}
	if err := run.Validate(); err != nil {
		return nil, err
	}
	if err := ValidatePersistenceContract(run); err != nil {
		return nil, err
	}
	if err := s.repo.CreateRun(ctx, run); err != nil {
		return nil, err
	}
	return run, nil
}

type tokenTotals struct {
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	CachedTokens     int64
	ReasoningTokens  int64
}

type tokenPresence struct {
	PromptTokens     bool
	CompletionTokens bool
	TotalTokens      bool
	CachedTokens     bool
	ReasoningTokens  bool
}

type tokenProjection struct {
	PromptTokens     *int64 `json:"prompt_tokens,omitempty"`
	CompletionTokens *int64 `json:"completion_tokens,omitempty"`
	TotalTokens      *int64 `json:"total_tokens,omitempty"`
	CachedTokens     *int64 `json:"cached_tokens,omitempty"`
	ReasoningTokens  *int64 `json:"reasoning_tokens,omitempty"`
}

type usageProjection struct {
	ReportedModelCalls int              `json:"reported_model_calls"`
	MissingModelCalls  int              `json:"missing_model_calls"`
	ReportedTotals     tokenProjection  `json:"reported_totals"`
	CompleteTotals     *tokenProjection `json:"complete_totals,omitempty"`
}

type safeErrorProjection struct {
	Category   string `json:"category,omitempty"`
	Code       string `json:"code,omitempty"`
	Type       string `json:"type,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
	Retryable  bool   `json:"retryable"`
}

type safeToolRequestProjection struct {
	CallID string `json:"call_id,omitempty"`
	Name   string `json:"name"`
}

type safeModelCallProjection struct {
	Sequence              uint64                      `json:"sequence"`
	Provider              string                      `json:"provider,omitempty"`
	Model                 string                      `json:"model,omitempty"`
	Status                string                      `json:"status"`
	StartedAt             time.Time                   `json:"started_at"`
	DurationMS            int64                       `json:"duration_ms"`
	InputMessages         int                         `json:"input_messages"`
	ToolsOffered          int                         `json:"tools_offered"`
	FinishReason          string                      `json:"finish_reason,omitempty"`
	ProviderResponseAlias string                      `json:"provider_response_alias,omitempty"`
	Usage                 *tokenProjection            `json:"usage,omitempty"`
	ToolRequests          []safeToolRequestProjection `json:"tool_requests,omitempty"`
	ToolRequestsTruncated bool                        `json:"tool_requests_truncated,omitempty"`
	Error                 *safeErrorProjection        `json:"error,omitempty"`
}

// projectSnapshot accepts only a terminal snapshot taken after every reserved
// model/tool span has completed. A live snapshot can legitimately contain
// in-flight spans, but it is not a coherent final audit; callers persist that
// case as telemetry=invalid instead of presenting partial counts as complete.
func projectSnapshot(run *agentaudit.Run, snapshot *telemetry.Snapshot) error {
	const clockSkew = 5 * time.Minute
	if run == nil || snapshot == nil || snapshot.StartedAt.IsZero() || snapshot.FinishedAt.IsZero() || snapshot.DurationMS < 0 || snapshot.DurationMS > agentaudit.MaxCallDurationMS ||
		snapshot.StartedAt.Before(run.StartedAt.Add(-clockSkew)) || snapshot.StartedAt.After(run.FinishedAt.Add(clockSkew)) ||
		snapshot.FinishedAt.Before(run.StartedAt.Add(-clockSkew)) || snapshot.FinishedAt.After(run.FinishedAt.Add(clockSkew)) {
		return errors.New("invalid snapshot timing")
	}
	if !validCount(snapshot.ModelCallCount) || !validCount(snapshot.ToolCallCount) || !validCount(snapshot.DroppedModelCalls) || !validCount(snapshot.DroppedToolCalls) ||
		!validCount(snapshot.Usage.ReportedModelCalls) || !validCount(snapshot.Usage.MissingModelCalls) {
		return errors.New("invalid snapshot counts")
	}
	if len(snapshot.ModelCalls) > maxPersistedModelCalls || len(snapshot.ToolCalls) > maxPersistedToolCalls ||
		snapshot.ModelCallCount != len(snapshot.ModelCalls)+snapshot.DroppedModelCalls ||
		snapshot.ToolCallCount != len(snapshot.ToolCalls)+snapshot.DroppedToolCalls ||
		snapshot.Usage.ReportedModelCalls+snapshot.Usage.MissingModelCalls != snapshot.ModelCallCount {
		return errors.New("inconsistent snapshot counts")
	}

	models := append([]telemetry.ModelCall(nil), snapshot.ModelCalls...)
	tools := append([]telemetry.ToolCall(nil), snapshot.ToolCalls...)
	sort.SliceStable(models, func(i, j int) bool { return models[i].Sequence < models[j].Sequence })
	sort.SliceStable(tools, func(i, j int) bool { return tools[i].Sequence < tools[j].Sequence })
	sequences := make(map[uint64]struct{}, len(models)+len(tools))
	modelProjections := make([]safeModelCallProjection, 0, len(models))
	requestedTools := make(map[string]string)
	computedReported := 0
	computedMissing := snapshot.DroppedModelCalls
	var computedTotals tokenTotals
	var completePresence tokenPresence
	presenceInitialized := false
	for _, call := range models {
		if call.Sequence == 0 || call.Sequence > math.MaxInt64 || !callWithinSnapshot(snapshot, call.StartedAt, call.DurationMS) {
			return errors.New("invalid model sequence")
		}
		if _, exists := sequences[call.Sequence]; exists {
			return errors.New("duplicate telemetry sequence")
		}
		sequences[call.Sequence] = struct{}{}
		projection, reported, usage, presence, err := projectModelCall(call)
		if err != nil {
			return err
		}
		if reported {
			computedReported++
			if !addTokenTotals(&computedTotals, usage) {
				return errors.New("token total overflow")
			}
			if !presenceInitialized {
				completePresence = presence
				presenceInitialized = true
			} else {
				completePresence = intersectTokenPresence(completePresence, presence)
			}
		} else {
			computedMissing++
		}
		for _, request := range projection.ToolRequests {
			if request.CallID == "" {
				continue
			}
			if existing, found := requestedTools[request.CallID]; found && existing != request.Name {
				return errors.New("conflicting tool request aliases")
			}
			requestedTools[request.CallID] = request.Name
		}
		modelProjections = append(modelProjections, projection)
	}
	if computedReported != snapshot.Usage.ReportedModelCalls || computedMissing != snapshot.Usage.MissingModelCalls {
		return errors.New("inconsistent usage coverage")
	}
	reportedTotals, ok, err := projectTokenTotals(&snapshot.Usage.ReportedTotals, true)
	if err != nil || !ok || reportedTotals != computedTotals {
		return errors.New("inconsistent reported totals")
	}

	reportedProjection := tokenProjection{}
	if presenceInitialized {
		reportedProjection = projectPresentTokens(reportedTotals, completePresence)
	}
	usageValue := usageProjection{
		ReportedModelCalls: snapshot.Usage.ReportedModelCalls,
		MissingModelCalls:  snapshot.Usage.MissingModelCalls,
		ReportedTotals:     reportedProjection,
	}
	shouldHaveCompleteTotals := snapshot.ModelCallCount > 0 && snapshot.Usage.ReportedModelCalls > 0 && snapshot.Usage.MissingModelCalls == 0
	if (snapshot.Usage.CompleteTotals != nil) != shouldHaveCompleteTotals {
		return errors.New("inconsistent complete totals presence")
	}
	if snapshot.Usage.CompleteTotals != nil {
		complete, valid, projectErr := projectTokenTotals(snapshot.Usage.CompleteTotals, false)
		if projectErr != nil || !valid || complete != reportedTotals {
			return errors.New("invalid complete totals")
		}
		projectedComplete := projectPresentTokens(complete, completePresence)
		usageValue.CompleteTotals = &projectedComplete
		if shouldHaveCompleteTotals {
			run.PromptTokens = cloneInt64(projectedComplete.PromptTokens)
			run.CompletionTokens = cloneInt64(projectedComplete.CompletionTokens)
			run.TotalTokens = cloneInt64(projectedComplete.TotalTokens)
			run.CachedTokens = cloneInt64(projectedComplete.CachedTokens)
			run.ReasoningTokens = cloneInt64(projectedComplete.ReasoningTokens)
		}
	}

	toolProjections := make([]agentaudit.ToolCall, 0, len(tools))
	for _, call := range tools {
		if call.Sequence == 0 || call.Sequence > math.MaxInt64 || !callWithinSnapshot(snapshot, call.StartedAt, call.DurationMS) {
			return errors.New("invalid tool sequence")
		}
		if _, exists := sequences[call.Sequence]; exists {
			return errors.New("duplicate telemetry sequence")
		}
		sequences[call.Sequence] = struct{}{}
		projected, err := projectToolCall(call)
		if err != nil {
			return err
		}
		if projected.CallID != nil {
			requestedName, found := requestedTools[*projected.CallID]
			if !found || requestedName != projected.Name {
				return errors.New("uncorrelated tool call")
			}
		}
		toolProjections = append(toolProjections, projected)
	}

	usageJSON, err := json.Marshal(usageValue)
	if err != nil {
		return err
	}
	modelsJSON, err := json.Marshal(modelProjections)
	if err != nil {
		return err
	}
	modelCount, toolCount := snapshot.ModelCallCount, snapshot.ToolCallCount
	droppedModels, droppedTools := snapshot.DroppedModelCalls, snapshot.DroppedToolCalls
	reported, missing := snapshot.Usage.ReportedModelCalls, snapshot.Usage.MissingModelCalls
	agentDuration := snapshot.DurationMS
	agentStarted, agentFinished := snapshot.StartedAt, snapshot.FinishedAt
	usageText, modelsText := string(usageJSON), string(modelsJSON)
	run.TelemetryStatus = agentaudit.TelemetryCaptured
	run.AgentStartedAt = &agentStarted
	run.AgentFinishedAt = &agentFinished
	run.AgentDurationMS = &agentDuration
	run.ModelCallCount = &modelCount
	run.ToolCallCount = &toolCount
	run.DroppedModelCalls = &droppedModels
	run.DroppedToolCalls = &droppedTools
	run.ReportedModelCalls = &reported
	run.MissingModelCalls = &missing
	run.UsageJSON = &usageText
	run.ModelCallsJSON = &modelsText
	run.ToolCalls = toolProjections
	return nil
}

func projectModelCall(call telemetry.ModelCall) (safeModelCallProjection, bool, tokenTotals, tokenPresence, error) {
	if !validModelStatus(call.Status) || call.StartedAt.IsZero() || call.DurationMS < 0 || !validCount(call.InputMessages) || !validCount(call.ToolsOffered) || len(call.ToolRequests) > maxPersistedToolRequests {
		return safeModelCallProjection{}, false, tokenTotals{}, tokenPresence{}, errors.New("invalid model call")
	}
	result := safeModelCallProjection{
		Sequence:              call.Sequence,
		Provider:              safeProvider(call.Provider),
		Model:                 safeModel(call.Model),
		Status:                call.Status,
		StartedAt:             call.StartedAt,
		DurationMS:            call.DurationMS,
		InputMessages:         call.InputMessages,
		ToolsOffered:          call.ToolsOffered,
		FinishReason:          safeFinishReason(call.FinishReason),
		ProviderResponseAlias: safeAlias(call.ProviderResponseAlias, "response-"),
		ToolRequestsTruncated: call.ToolRequestsTruncated,
	}
	projectedError, err := projectError(call.Error)
	if err != nil {
		return safeModelCallProjection{}, false, tokenTotals{}, tokenPresence{}, err
	}
	result.Error = projectedError
	if !validErrorForStatus(call.Status, result.Error) {
		return safeModelCallProjection{}, false, tokenTotals{}, tokenPresence{}, errors.New("inconsistent model error")
	}
	if len(call.ToolRequests) > 0 {
		result.ToolRequests = make([]safeToolRequestProjection, 0, len(call.ToolRequests))
		for _, request := range call.ToolRequests {
			result.ToolRequests = append(result.ToolRequests, safeToolRequestProjection{
				CallID: safeAlias(request.CallID, "call-"),
				Name:   safeToolName(request.Name),
			})
		}
	}
	usage, reported, err := projectTokenTotals(call.Usage, false)
	if err != nil {
		return safeModelCallProjection{}, false, tokenTotals{}, tokenPresence{}, err
	}
	if reported {
		presence := positiveTokenPresence(usage)
		projectedUsage := projectPresentTokens(usage, presence)
		result.Usage = &projectedUsage
		return result, true, usage, presence, nil
	}
	return result, false, tokenTotals{}, tokenPresence{}, nil
}

func projectToolCall(call telemetry.ToolCall) (agentaudit.ToolCall, error) {
	if !validToolStatus(call.Status) || call.StartedAt.IsZero() || call.DurationMS < 0 || call.Sequence == 0 || call.Sequence > math.MaxInt64 {
		return agentaudit.ToolCall{}, errors.New("invalid tool call")
	}
	name := safeToolName(call.Name)
	if call.Arguments.CapturedBytes != len(call.Arguments.JSON) || call.Result.CapturedBytes != len(call.Result.JSON) {
		return agentaudit.ToolCall{}, errors.New("inconsistent payload capture size")
	}
	arguments, argumentsRedacted, argumentsTruncated, err := projectPayload(name, call.Arguments.JSON, maxArgumentProjectionBytes, knownAuditedTool(name), persistedPayloadArguments)
	if err != nil {
		return agentaudit.ToolCall{}, err
	}
	result, resultRedacted, resultTruncated, err := projectPayload(name, call.Result.JSON, maxResultProjectionBytes, knownAuditedTool(name), persistedPayloadResult)
	if err != nil {
		return agentaudit.ToolCall{}, err
	}
	if call.Arguments.OriginalBytes < 0 || call.Result.OriginalBytes < 0 {
		return agentaudit.ToolCall{}, errors.New("invalid payload sizes")
	}
	projected := agentaudit.ToolCall{
		Sequence:               int64(call.Sequence),
		CallID:                 optionalString(safeAlias(call.CallID, "call-")),
		Name:                   name,
		Status:                 call.Status,
		StartedAt:              call.StartedAt,
		DurationMS:             call.DurationMS,
		ArgumentsJSON:          arguments,
		ArgumentsOriginalBytes: int64(call.Arguments.OriginalBytes),
		ArgumentsCapturedBytes: int64(len(arguments)),
		ArgumentsRedacted:      call.Arguments.Redacted || argumentsRedacted,
		ArgumentsTruncated:     call.Arguments.Truncated || argumentsTruncated,
		ResultJSON:             result,
		ResultOriginalBytes:    int64(call.Result.OriginalBytes),
		ResultCapturedBytes:    int64(len(result)),
		ResultRedacted:         call.Result.Redacted || resultRedacted,
		ResultTruncated:        call.Result.Truncated || resultTruncated,
		IsWrite:                isWriteOrUnknownTool(name),
	}
	safeErr, err := projectError(call.Error)
	if err != nil {
		return agentaudit.ToolCall{}, err
	}
	if !validErrorForStatus(call.Status, safeErr) {
		return agentaudit.ToolCall{}, errors.New("inconsistent tool error")
	}
	if safeErr != nil {
		projected.ErrorCategory = optionalString(safeErr.Category)
		projected.ErrorCode = optionalString(safeErr.Code)
		projected.ErrorType = optionalString(safeErr.Type)
		if safeErr.HTTPStatus != 0 {
			projected.ErrorHTTPStatus = intPointer(safeErr.HTTPStatus)
		}
		projected.ErrorRetryable = boolPointer(safeErr.Retryable)
	}
	projectBusinessObject(&projected)
	return projected, nil
}

func projectBusinessObject(call *agentaudit.ToolCall) {
	if call == nil || call.Status != "success" || call.ResultJSON == "" {
		return
	}
	var value map[string]any
	decoder := json.NewDecoder(strings.NewReader(call.ResultJSON))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return
	}
	switch call.Name {
	case "draft_announcement":
		id, ok := jsonInt64(value["id"])
		if !ok || id <= 0 {
			return
		}
		call.BusinessObjectType = stringPointer("announcement_draft")
		call.BusinessObjectID = &id
		if status, ok := value["status"].(string); ok && safeIdentifierPattern.MatchString(status) {
			call.BusinessObjectStatus = &status
		}
	}
}

func projectPayload(toolName, raw string, limit int, known bool, kind persistedPayloadKind) (string, bool, bool, error) {
	if len(raw) == 0 || len(raw) > limit || !utf8.ValidString(raw) {
		return "", false, false, errors.New("invalid payload projection")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", false, false, errors.New("invalid payload projection")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return "", false, false, errors.New("multiple payload values")
	}
	normalizedOriginal, err := json.Marshal(value)
	if err != nil {
		return "", false, false, errors.New("invalid payload projection")
	}
	truncated := payloadProjectionTruncated(value, 0)
	if !known {
		value = payloadShape(value, 0)
	} else {
		value = projectKnownPayload(toolName, kind, value)
	}
	data, err := json.Marshal(value)
	if err != nil || len(data) > limit {
		return "", false, false, errors.New("payload projection exceeds limit")
	}
	redacted := !known || string(data) != string(normalizedOriginal)
	return string(data), redacted, truncated, nil
}

func projectKnownPayload(toolName string, kind persistedPayloadKind, value any) any {
	if envelope, ok := projectPayloadEnvelope(value); ok {
		return envelope
	}
	if value == nil {
		if kind == persistedPayloadResult {
			return nil
		}
		return map[string]any{}
	}
	object, ok := value.(map[string]any)
	if !ok {
		if kind == persistedPayloadArguments {
			return map[string]any{}
		}
		return nil
	}
	if kind == persistedPayloadArguments {
		return projectKnownArguments(toolName, object)
	}
	return projectKnownResult(toolName, object)
}

func projectPayloadEnvelope(value any) (map[string]any, bool) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	kind, ok := object["kind"].(string)
	if !ok {
		return nil, false
	}
	result := map[string]any{"kind": kind}
	switch kind {
	case "oversize", "opaque":
		addIntegerField(result, object, "bytes", false, 0, math.MaxInt64)
	case "bounded_projection":
		addIntegerField(result, object, "original_bytes", false, 0, math.MaxInt64)
	case "unavailable":
	default:
		return nil, false
	}
	return result, true
}

func projectKnownArguments(toolName string, object map[string]any) map[string]any {
	result := make(map[string]any)
	switch toolName {
	case "get_checkin_status", "get_my_work_hours", "get_checkin_requirements", "get_online_summary", "clock_in", "clock_out":
	case "get_my_checkin_history":
		addIntegerOrInvalidField(result, object, "limit", 0, 100)
	case "search_knowledge":
		addIntegerField(result, object, "query_length", false, 0, 1<<20)
	case "recommend_knowledge":
		addIntegerField(result, object, "need_length", false, 0, 1<<20)
	case "query_team_members":
		addTrueField(result, object, "period_id_provided")
		addNumberOrInvalidField(result, object, "min_hours", 0, 10000)
		addIntegerOrInvalidField(result, object, "limit", 0, 200)
		addEnumField(result, object, "period", false, "", "today", "week", "month", "invalid")
		addEnumField(result, object, "report_status", false, "", "missing", "draft", "submitted", "reviewed", "invalid")
	case "get_checkin_stats":
		addEnumField(result, object, "period", false, "", "today", "week", "month", "invalid")
	case "list_report_periods":
		addIntegerOrInvalidField(result, object, "limit", 0, 24)
		addPeriodKeyField(result, object, "period_key", false, true)
		addEnumField(result, object, "status", false, "", "open", "closed", "archived", "invalid")
	case "get_report_materials":
		addTrueField(result, object, "period_id_provided")
		addIntegerOrInvalidField(result, object, "offset", 0, 1000000)
		addIntegerOrInvalidField(result, object, "limit", 0, 8)
		addEnumField(result, object, "scope", false, "", "mine", "team", "invalid")
	case "draft_announcement":
		addIntegerField(result, object, "title_length", false, 0, 1<<20)
		addIntegerField(result, object, "content_length", false, 0, 1<<20)
		addIntegerField(result, object, "source_note_length", false, 0, 1<<20)
		addBoolOrInvalidField(result, object, "pinned", true)
		addIntegerField(result, object, "recipient_count", false, 0, 1000000)
	case "list_pending_knowledge":
		addIntegerOrInvalidField(result, object, "limit", 0, 50)
	case "review_knowledge_item":
		addTrueField(result, object, "item_id_provided")
		addEnumField(result, object, "decision", false, "", "approve", "reject", "other")
		addIntegerField(result, object, "reason_length", false, 0, 1<<20)
	case "generate_invitation_code":
		addEnumField(result, object, "type", false, "", "member", "captain", "other")
	case "save_evaluation_draft":
		addEnumField(result, object, "mode", false, "", "create", "update", "other")
		addTrueField(result, object, "draft_id_provided")
		addTrueField(result, object, "snapshot_id_provided")
		addIntegerField(result, object, "title_length", false, 0, 1<<20)
		addIntegerField(result, object, "summary_length", false, 0, 1<<20)
		addEnumField(result, object, "audience", false, "", "captain", "team", "private", "other")
	}
	return result
}

func projectKnownResult(toolName string, object map[string]any) map[string]any {
	result := make(map[string]any)
	switch toolName {
	case "get_checkin_status":
		addEnumField(result, object, "status", true, "idle", "clocked_in", "clocked_out", "other")
	case "clock_in", "clock_out":
		if projectConfirmationProposal(result, object) {
			break
		}
		addEnumField(result, object, "type", true, "in", "out", "other")
		addDateField(result, object, "date", true)
	case "get_my_work_hours":
		addDateField(result, object, "as_of_date", true)
		addObjectField(result, object, "week", projectPeriodStatsProjection)
		addObjectField(result, object, "month", projectPeriodStatsProjection)
		addObjectField(result, object, "weekly_requirement", projectWeeklyRequirementProjection)
	case "get_my_checkin_history":
		addNumberField(result, object, "limit", true, -math.MaxFloat64, math.MaxFloat64)
		addIntegerField(result, object, "record_count", false, 0, math.MaxInt64)
		addCountMapField(result, object, "type_counts", "in", "out", "other")
	case "get_checkin_requirements":
		addObjectArrayField(result, object, "requirements", projectRequirementProjection)
	case "search_knowledge":
		addIntegerField(result, object, "source_count", false, 0, math.MaxInt64)
		addNumberArrayField(result, object, "source_ids")
		addEnumArrayField(result, object, "match_modes", "keyword", "vector", "hybrid")
		addBoolField(result, object, "truncated", true)
		addNumberField(result, object, "omitted_sources", true, -math.MaxFloat64, math.MaxFloat64)
	case "recommend_knowledge":
		addIntegerField(result, object, "recommendation_count", false, 0, math.MaxInt64)
		addNumberArrayField(result, object, "item_ids")
	case "query_team_members":
		addEnumField(result, object, "period", true, "today", "week", "month", "other")
		addNumberField(result, object, "period_id", true, -math.MaxFloat64, math.MaxFloat64)
		addEnumField(result, object, "report_status", true, "", "missing", "draft", "submitted", "reviewed", "archived", "other")
		addNumberField(result, object, "limit", true, -math.MaxFloat64, math.MaxFloat64)
		addIntegerField(result, object, "member_count", false, 0, math.MaxInt64)
		addCountMapField(result, object, "report_states", "missing", "draft", "submitted", "reviewed", "archived", "other")
		addCountMapField(result, object, "attendance", "absent", "normal", "attention", "unknown", "anomaly", "other")
		addIntegerField(result, object, "anomaly_members", false, 0, maxPayloadProjectionItems)
	case "get_checkin_stats":
		addIntegerField(result, object, "member_count", false, 0, math.MaxInt64)
		addCountMapField(result, object, "status_counts", "absent", "normal", "attention", "unknown", "anomaly", "other")
		addIntegerField(result, object, "anomaly_members", false, 0, maxPayloadProjectionItems)
	case "list_report_periods":
		addPeriodKeyField(result, object, "period_key", true, false)
		addEnumField(result, object, "status", true, "", "open", "closed", "archived", "other")
		addNumberField(result, object, "total_periods", true, -math.MaxFloat64, math.MaxFloat64)
		addBoolField(result, object, "truncated", true)
		addObjectArrayField(result, object, "periods", projectReportPeriodProjection)
	case "get_report_materials":
		addNumberField(result, object, "period_id", true, -math.MaxFloat64, math.MaxFloat64)
		addEnumField(result, object, "scope", true, "mine", "team", "other")
		for _, key := range []string{"offset", "limit", "total_reports", "next_offset", "skipped_reports"} {
			addNumberField(result, object, key, true, -math.MaxFloat64, math.MaxFloat64)
		}
		addObjectArrayField(result, object, "materials", projectReportMaterialProjection)
	case "draft_announcement":
		if projectConfirmationProposal(result, object) {
			break
		}
		addNumberField(result, object, "id", true, -math.MaxFloat64, math.MaxFloat64)
		addEnumField(result, object, "status", true, "draft", "confirmed", "published", "discarded", "other")
		addBoolField(result, object, "pinned", true)
		addIntegerField(result, object, "recipient_count", false, 0, math.MaxInt64)
		if value, ok := object["email_requested"].(string); ok && value == "[REDACTED]" {
			result["email_requested"] = value
		}
		addNumberField(result, object, "announcement_id", true, -math.MaxFloat64, math.MaxFloat64)
	case "review_knowledge_item", "generate_invitation_code", "save_evaluation_draft":
		projectConfirmationProposal(result, object)
	case "list_pending_knowledge":
		addNumberField(result, object, "total", true, 0, math.MaxFloat64)
		addNumberField(result, object, "limit", true, 0, math.MaxFloat64)
		addIntegerField(result, object, "item_count", false, 0, math.MaxInt64)
		addBoolField(result, object, "truncated", true)
	case "get_online_summary":
		addDateField(result, object, "as_of_date", true)
		addNumberField(result, object, "total_members", true, 0, math.MaxFloat64)
		addNumberField(result, object, "online_count", true, 0, math.MaxFloat64)
		addBoolField(result, object, "self_online", true)
		addIntegerField(result, object, "name_count", false, 0, maxPayloadProjectionItems)
		addBoolField(result, object, "truncated", true)
	}
	return result
}

func projectConfirmationProposal(result, object map[string]any) bool {
	status, _ := object["status"].(string)
	if status != "confirmation_required" {
		return false
	}
	addEnumField(result, object, "status", true, "confirmation_required", "other")
	addEnumField(result, object, "tool", true, "clock_in", "clock_out", "draft_announcement", "review_knowledge_item", "generate_invitation_code", "save_evaluation_draft", "other")
	return true
}

func projectPeriodStatsProjection(object map[string]any) map[string]any {
	result := make(map[string]any)
	addDateField(result, object, "starts_on", true)
	addDateField(result, object, "ends_on", true)
	addNumberField(result, object, "total_minutes", true, -math.MaxFloat64, math.MaxFloat64)
	addNumberField(result, object, "total_hours", true, -math.MaxFloat64, math.MaxFloat64)
	addNumberField(result, object, "record_days", true, -math.MaxFloat64, math.MaxFloat64)
	addEnumField(result, object, "attendance_state", true, "absent", "normal", "attention", "unknown", "anomaly", "other")
	return result
}

func projectWeeklyRequirementProjection(object map[string]any) map[string]any {
	result := make(map[string]any)
	for _, key := range []string{"grade", "required_minutes", "required_hours", "shortfall_minutes", "shortfall_hours"} {
		addNumberField(result, object, key, true, -math.MaxFloat64, math.MaxFloat64)
	}
	addBoolField(result, object, "is_met", true)
	addEnumField(result, object, "status", true, "unknown", "below_target", "met", "other")
	return result
}

func projectRequirementProjection(object map[string]any) map[string]any {
	result := make(map[string]any)
	addNumberField(result, object, "grade", true, -math.MaxFloat64, math.MaxFloat64)
	addNumberField(result, object, "minutes", true, -math.MaxFloat64, math.MaxFloat64)
	return result
}

func projectReportPeriodProjection(object map[string]any) map[string]any {
	result := make(map[string]any)
	addNumberField(result, object, "id", true, -math.MaxFloat64, math.MaxFloat64)
	addPeriodKeyField(result, object, "period_key", true, false)
	addEnumField(result, object, "status", true, "open", "closed", "archived", "other")
	addNumberField(result, object, "member_count", true, -math.MaxFloat64, math.MaxFloat64)
	return result
}

func projectReportMaterialProjection(object map[string]any) map[string]any {
	result := make(map[string]any)
	addNumberField(result, object, "report_id", true, -math.MaxFloat64, math.MaxFloat64)
	addNumberField(result, object, "version", true, -math.MaxFloat64, math.MaxFloat64)
	addEnumField(result, object, "status", true, "draft", "submitted", "reviewed", "rejected", "invalidated", "archived", "other")
	addEnumField(result, object, "parse_status", true, "pending", "processing", "ready", "failed", "parsed", "other")
	addBoolField(result, object, "truncated", true)
	return result
}

func addIntegerField(result, object map[string]any, key string, nullable bool, minimum, maximum int64) {
	value, exists := object[key]
	if !exists {
		return
	}
	if value == nil {
		if nullable {
			result[key] = nil
		}
		return
	}
	integer, ok := jsonInt64(value)
	if ok && integer >= minimum && integer <= maximum {
		result[key] = integer
	}
}

func addIntegerOrInvalidField(result, object map[string]any, key string, minimum, maximum int64) {
	if value, ok := object[key].(string); ok && value == "invalid" {
		result[key] = value
		return
	}
	addIntegerField(result, object, key, false, minimum, maximum)
}

func addNumberField(result, object map[string]any, key string, nullable bool, minimum, maximum float64) {
	value, exists := object[key]
	if !exists {
		return
	}
	if value == nil {
		if nullable {
			result[key] = nil
		}
		return
	}
	normalized, number, ok := finiteJSONNumber(value)
	if ok && number >= minimum && number <= maximum {
		result[key] = normalized
	}
}

func addNumberOrInvalidField(result, object map[string]any, key string, minimum, maximum float64) {
	if value, ok := object[key].(string); ok && value == "invalid" {
		result[key] = value
		return
	}
	addNumberField(result, object, key, false, minimum, maximum)
}

func finiteJSONNumber(value any) (any, float64, bool) {
	switch typed := value.(type) {
	case json.Number:
		number, err := typed.Float64()
		return typed, number, err == nil && !math.IsNaN(number) && !math.IsInf(number, 0)
	case float64:
		return typed, typed, !math.IsNaN(typed) && !math.IsInf(typed, 0)
	default:
		return nil, 0, false
	}
}

func addTrueField(result, object map[string]any, key string) {
	if value, ok := object[key].(bool); ok && value {
		result[key] = true
	}
}

func addBoolField(result, object map[string]any, key string, nullable bool) {
	value, exists := object[key]
	if !exists {
		return
	}
	if value == nil {
		if nullable {
			result[key] = nil
		}
		return
	}
	if boolean, ok := value.(bool); ok {
		result[key] = boolean
	}
}

func addBoolOrInvalidField(result, object map[string]any, key string, nullable bool) {
	if value, ok := object[key].(string); ok && value == "invalid" {
		result[key] = value
		return
	}
	addBoolField(result, object, key, nullable)
}

func addEnumField(result, object map[string]any, key string, nullable bool, allowed ...string) {
	value, exists := object[key]
	if !exists {
		return
	}
	if value == nil {
		if nullable {
			result[key] = nil
		}
		return
	}
	text, ok := value.(string)
	if !ok {
		return
	}
	for _, candidate := range allowed {
		if text == candidate {
			result[key] = text
			return
		}
	}
}

func addDateField(result, object map[string]any, key string, nullable bool) {
	value, exists := object[key]
	if !exists {
		return
	}
	if value == nil {
		if nullable {
			result[key] = nil
		}
		return
	}
	text, ok := value.(string)
	if !ok {
		return
	}
	if text == "other" {
		result[key] = text
		return
	}
	if _, err := time.Parse("2006-01-02", text); err == nil {
		result[key] = text
	}
}

func addPeriodKeyField(result, object map[string]any, key string, nullable, allowInvalid bool) {
	value, exists := object[key]
	if !exists {
		return
	}
	if value == nil {
		if nullable {
			result[key] = nil
		}
		return
	}
	text, ok := value.(string)
	if !ok {
		return
	}
	if text == "" || periodPattern.MatchString(text) || allowInvalid && text == "invalid" || !allowInvalid && text == "other" {
		result[key] = text
	}
}

func addObjectField(result, object map[string]any, key string, projector func(map[string]any) map[string]any) {
	value, exists := object[key]
	if !exists {
		return
	}
	nested, ok := value.(map[string]any)
	if ok {
		result[key] = projector(nested)
	}
}

func addObjectArrayField(result, object map[string]any, key string, projector func(map[string]any) map[string]any) {
	value, exists := object[key]
	if !exists {
		return
	}
	items, ok := value.([]any)
	if !ok {
		return
	}
	if len(items) > maxPayloadProjectionItems {
		items = items[:maxPayloadProjectionItems]
	}
	projected := make([]any, 0, len(items))
	for _, item := range items {
		nested, ok := item.(map[string]any)
		if ok {
			projected = append(projected, projector(nested))
		}
	}
	result[key] = projected
}

func addNumberArrayField(result, object map[string]any, key string) {
	value, exists := object[key]
	if !exists {
		return
	}
	items, ok := value.([]any)
	if !ok {
		return
	}
	if len(items) > maxPayloadProjectionItems {
		items = items[:maxPayloadProjectionItems]
	}
	projected := make([]any, 0, len(items))
	for _, item := range items {
		normalized, _, ok := finiteJSONNumber(item)
		if ok {
			projected = append(projected, normalized)
		}
	}
	result[key] = projected
}

func addEnumArrayField(result, object map[string]any, key string, allowed ...string) {
	value, exists := object[key]
	if !exists {
		return
	}
	items, ok := value.([]any)
	if !ok {
		return
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, item := range allowed {
		allowedSet[item] = struct{}{}
	}
	seen := make(map[string]struct{})
	for _, item := range items {
		text, ok := item.(string)
		if _, allowed := allowedSet[text]; ok && allowed {
			seen[text] = struct{}{}
		}
	}
	projected := make([]string, 0, len(seen))
	for item := range seen {
		projected = append(projected, item)
	}
	sort.Strings(projected)
	result[key] = projected
}

func addCountMapField(result, object map[string]any, key string, allowed ...string) {
	value, exists := object[key]
	if !exists {
		return
	}
	counts, ok := value.(map[string]any)
	if !ok {
		return
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, item := range allowed {
		allowedSet[item] = struct{}{}
	}
	projected := make(map[string]any)
	names := make([]string, 0, len(counts))
	for name := range counts {
		if _, allowed := allowedSet[name]; allowed {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	total := int64(0)
	for _, name := range names {
		raw := counts[name]
		count, valid := jsonInt64(raw)
		if !valid || count < 1 || count > maxPayloadProjectionItems {
			continue
		}
		if total > maxPayloadProjectionItems-count {
			result[key] = map[string]any{}
			return
		}
		projected[name] = count
		total += count
	}
	result[key] = projected
}

func payloadShape(value any, depth int) any {
	if depth >= maxPayloadProjectionDepth {
		return map[string]any{"type": "depth_limited"}
	}
	switch typed := value.(type) {
	case map[string]any:
		return map[string]any{"type": "object", "key_count": len(typed), "truncated": len(typed) > 32}
	case []any:
		limit := len(typed)
		if limit > maxPayloadProjectionItems {
			limit = maxPayloadProjectionItems
		}
		types := make(map[string]struct{})
		for _, item := range typed[:limit] {
			types[payloadType(item)] = struct{}{}
		}
		items := make([]string, 0, len(types))
		for item := range types {
			items = append(items, item)
		}
		sort.Strings(items)
		return map[string]any{"type": "array", "count": len(typed), "item_types": items}
	default:
		return map[string]any{"type": payloadType(typed)}
	}
}

func payloadType(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number, float64:
		return "number"
	default:
		return "unknown"
	}
}

func payloadProjectionTruncated(value any, depth int) bool {
	if depth >= maxPayloadProjectionDepth {
		return true
	}
	switch typed := value.(type) {
	case map[string]any:
		if len(typed) > 32 {
			return true
		}
		for _, item := range typed {
			if payloadProjectionTruncated(item, depth+1) {
				return true
			}
		}
	case []any:
		if len(typed) > maxPayloadProjectionItems {
			return true
		}
		for _, item := range typed {
			if payloadProjectionTruncated(item, depth+1) {
				return true
			}
		}
	}
	return false
}

func setKnownZeroTelemetry(run *agentaudit.Run) {
	zero := 0
	usage := `{"reported_model_calls":0,"missing_model_calls":0,"reported_totals":{}}`
	models := `[]`
	run.TelemetryStatus = agentaudit.TelemetryNotStarted
	run.ModelCallCount = &zero
	run.ToolCallCount = intPointer(0)
	run.DroppedModelCalls = intPointer(0)
	run.DroppedToolCalls = intPointer(0)
	run.ReportedModelCalls = intPointer(0)
	run.MissingModelCalls = intPointer(0)
	run.UsageJSON = &usage
	run.ModelCallsJSON = &models
	run.ToolCalls = nil
}

func clearTelemetry(run *agentaudit.Run, status agentaudit.TelemetryStatus) {
	run.TelemetryStatus = status
	run.AgentStartedAt = nil
	run.AgentFinishedAt = nil
	run.AgentDurationMS = nil
	run.ModelCallCount = nil
	run.ToolCallCount = nil
	run.DroppedModelCalls = nil
	run.DroppedToolCalls = nil
	run.ReportedModelCalls = nil
	run.MissingModelCalls = nil
	run.PromptTokens = nil
	run.CompletionTokens = nil
	run.TotalTokens = nil
	run.CachedTokens = nil
	run.ReasoningTokens = nil
	run.UsageJSON = nil
	run.ModelCallsJSON = nil
	run.ToolCalls = nil
}

func projectTokenTotals(value *telemetry.TokenUsage, allowZero bool) (tokenTotals, bool, error) {
	if value == nil {
		return tokenTotals{}, false, nil
	}
	if value.PromptTokens < 0 || value.CompletionTokens < 0 || value.TotalTokens < 0 || value.CachedTokens < 0 || value.ReasoningTokens < 0 {
		return tokenTotals{}, false, errors.New("negative token usage")
	}
	result := tokenTotals{
		PromptTokens:     int64(value.PromptTokens),
		CompletionTokens: int64(value.CompletionTokens),
		TotalTokens:      int64(value.TotalTokens),
		CachedTokens:     int64(value.CachedTokens),
		ReasoningTokens:  int64(value.ReasoningTokens),
	}
	if tokensAreZero(result) && !allowZero {
		return result, false, nil
	}
	return result, true, nil
}

func addTokenTotals(total *tokenTotals, value tokenTotals) bool {
	if total == nil {
		return false
	}
	values := []struct {
		target *int64
		add    int64
	}{
		{&total.PromptTokens, value.PromptTokens},
		{&total.CompletionTokens, value.CompletionTokens},
		{&total.TotalTokens, value.TotalTokens},
		{&total.CachedTokens, value.CachedTokens},
		{&total.ReasoningTokens, value.ReasoningTokens},
	}
	for _, item := range values {
		if item.add < 0 || *item.target > math.MaxInt64-item.add {
			return false
		}
		*item.target += item.add
	}
	return true
}

func positiveTokenPresence(value tokenTotals) tokenPresence {
	return tokenPresence{
		PromptTokens:     value.PromptTokens > 0,
		CompletionTokens: value.CompletionTokens > 0,
		TotalTokens:      value.TotalTokens > 0,
		CachedTokens:     value.CachedTokens > 0,
		ReasoningTokens:  value.ReasoningTokens > 0,
	}
}

func intersectTokenPresence(left, right tokenPresence) tokenPresence {
	return tokenPresence{
		PromptTokens:     left.PromptTokens && right.PromptTokens,
		CompletionTokens: left.CompletionTokens && right.CompletionTokens,
		TotalTokens:      left.TotalTokens && right.TotalTokens,
		CachedTokens:     left.CachedTokens && right.CachedTokens,
		ReasoningTokens:  left.ReasoningTokens && right.ReasoningTokens,
	}
}

func projectPresentTokens(value tokenTotals, presence tokenPresence) tokenProjection {
	result := tokenProjection{}
	if presence.PromptTokens {
		result.PromptTokens = int64Pointer(value.PromptTokens)
	}
	if presence.CompletionTokens {
		result.CompletionTokens = int64Pointer(value.CompletionTokens)
	}
	if presence.TotalTokens {
		result.TotalTokens = int64Pointer(value.TotalTokens)
	}
	if presence.CachedTokens {
		result.CachedTokens = int64Pointer(value.CachedTokens)
	}
	if presence.ReasoningTokens {
		result.ReasoningTokens = int64Pointer(value.ReasoningTokens)
	}
	return result
}

func tokensAreZero(value tokenTotals) bool {
	return value == (tokenTotals{})
}

func projectError(value *telemetry.ErrorInfo) (*safeErrorProjection, error) {
	if value == nil {
		return nil, nil
	}
	category := strings.ToLower(strings.TrimSpace(value.Category))
	code := strings.ToLower(strings.TrimSpace(value.Code))
	typeName := strings.ToLower(strings.TrimSpace(value.Type))
	if !allowedErrorCategory(category) || !allowedErrorCode(code) || typeName != "" && !allowedErrorType(typeName) || value.HTTPStatus != 0 && (value.HTTPStatus < 100 || value.HTTPStatus > 599) {
		return nil, errors.New("invalid telemetry error metadata")
	}
	result := &safeErrorProjection{
		Category:  category,
		Code:      code,
		Type:      typeName,
		Retryable: value.Retryable,
	}
	if value.HTTPStatus >= 100 && value.HTTPStatus <= 599 {
		result.HTTPStatus = value.HTTPStatus
	}
	return result, nil
}

func allowedErrorCategory(value string) bool {
	switch value {
	case "canceled", "timeout", "limit", "network", "execution", "panic", "provider", "provider_auth", "provider_rate_limit", "provider_unavailable", "provider_request":
		return true
	default:
		return false
	}
}

func allowedErrorCode(value string) bool {
	switch value {
	case "context_canceled", "deadline_exceeded", "downstream_closed", "telemetry_unavailable", "tool_call_limit", "network_error", "network_timeout", "execution_error", "panic", "provider_request_error", "provider_auth", "provider_rate_limit", "provider_timeout", "provider_unavailable", "provider_request", "provider_error", "invalid_api_key", "authentication_error", "permission_denied", "insufficient_quota", "rate_limit_exceeded", "rate_limit", "model_overloaded", "overloaded", "overloaded_error", "server_error", "service_unavailable", "temporarily_unavailable", "invalid_request_error", "model_not_found", "context_length_exceeded", "content_filter":
		return true
	default:
		return false
	}
}

func allowedErrorType(value string) bool {
	switch value {
	case "authentication_error", "permission_error", "rate_limit_error", "overloaded_error", "server_error", "api_error", "invalid_request_error":
		return true
	default:
		return false
	}
}

func safeProvider(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "openai", "openai_compatible":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "[invalid]"
	}
}

func safeModel(value string) string {
	value = strings.TrimSpace(value)
	if value == "[invalid]" {
		return value
	}
	if value == "" || utf8.RuneCountInString(value) > maxPersistedIdentifierRunes || strings.Contains(value, "://") {
		return "[invalid]"
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '-' || char == '.' || char == ':' || char == '/' {
			continue
		}
		return "[invalid]"
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{"sk-", "sk_", "pk-", "pk_", "eyj", "ghp_", "github_pat_", "xoxb-", "xoxp-", "akia", "api_key", "api-key", "apikey", "secret", "password", "passwd", "credential", "bearer", "token"} {
		if strings.Contains(lower, marker) {
			return "[invalid]"
		}
	}
	return value
}

func safeToolName(value string) string {
	value = strings.TrimSpace(value)
	if value == "[invalid]" || value == "[unknown]" {
		return "[invalid]"
	}
	if value == "" || utf8.RuneCountInString(value) > maxPersistedIdentifierRunes || !safeIdentifierPattern.MatchString(value) {
		return "[invalid]"
	}
	if !knownAuditedTool(value) {
		return "[invalid]"
	}
	return value
}

func safeAlias(value, prefix string) string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, prefix) || !safeAliasPattern.MatchString(value) {
		return ""
	}
	return value
}

func safeFinishReason(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "stop", "length", "tool_calls", "function_call", "content_filter", "end_turn", "max_tokens", "error", "canceled", "cancelled", "other":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "other"
	}
}

func validModelStatus(value string) bool {
	switch value {
	case "success", "error", "timeout", "canceled", "panic":
		return true
	default:
		return false
	}
}

func validToolStatus(value string) bool { return validModelStatus(value) }

func validErrorForStatus(status string, value *safeErrorProjection) bool {
	if status == "success" {
		return value == nil
	}
	if value == nil {
		return false
	}
	switch status {
	case "canceled":
		return value.Category == "canceled"
	case "timeout":
		return value.Category == "timeout"
	case "panic":
		return value.Category == "panic"
	case "error":
		return value.Category != "canceled" && value.Category != "timeout" && value.Category != "panic"
	default:
		return false
	}
}

func validCount(value int) bool { return value >= 0 && value <= maxTelemetryCount }

func knownAuditedTool(value string) bool {
	switch value {
	case "get_checkin_status", "get_my_work_hours", "get_checkin_requirements", "clock_in", "clock_out", "get_my_checkin_history", "search_knowledge", "recommend_knowledge", "query_team_members", "get_checkin_stats", "list_report_periods", "get_report_materials", "draft_announcement", "list_pending_knowledge", "get_online_summary", "review_knowledge_item", "generate_invitation_code", "save_evaluation_draft":
		return true
	default:
		return false
	}
}

func isWriteOrUnknownTool(value string) bool {
	switch value {
	case "clock_in", "clock_out", "draft_announcement", "review_knowledge_item", "generate_invitation_code", "save_evaluation_draft":
		return true
	case "get_checkin_status", "get_my_work_hours", "get_checkin_requirements", "get_my_checkin_history", "search_knowledge", "recommend_knowledge", "query_team_members", "get_checkin_stats", "list_report_periods", "get_report_materials", "list_pending_knowledge", "get_online_summary":
		return false
	default:
		// Unknown tools are conservatively classified as writes until an
		// explicit policy entry proves they are read-only.
		return true
	}
}

func callWithinSnapshot(snapshot *telemetry.Snapshot, started time.Time, durationMS int64) bool {
	if snapshot == nil || started.IsZero() || durationMS < 0 {
		return false
	}
	// Remote timestamps lose Go's monotonic component during JSON transport and
	// may move slightly during a host clock correction. Keep a bounded wall-clock
	// relationship without deriving duration from those wall timestamps.
	const clockSkew = 5 * time.Minute
	if started.Before(snapshot.StartedAt.Add(-clockSkew)) || started.After(snapshot.FinishedAt.Add(clockSkew)) {
		return false
	}
	return durationMS <= agentaudit.MaxCallDurationMS
}

func safeOptionalIdentifier(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" || utf8.RuneCountInString(value) > maxRunes || !safeIdentifierPattern.MatchString(value) {
		return ""
	}
	return value
}

func optionalSafeIdentifier(value string, maxRunes int) *string {
	value = safeOptionalIdentifier(value, maxRunes)
	return optionalString(value)
}

func jsonInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	case float64:
		if math.Trunc(typed) != typed || typed < math.MinInt64 || typed > math.MaxInt64 {
			return 0, false
		}
		return int64(typed), true
	default:
		return 0, false
	}
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneTrimmedString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := strings.TrimSpace(*value)
	if copy == "" {
		return nil
	}
	return &copy
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func intPointer(value int) *int          { return &value }
func int64Pointer(value int64) *int64    { return &value }
func boolPointer(value bool) *bool       { return &value }
func stringPointer(value string) *string { return &value }
