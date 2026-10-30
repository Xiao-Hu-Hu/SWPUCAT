// Package agentaudit contains the append-only execution records used to
// correlate a public Agent request with its bounded model and tool telemetry.
// It intentionally contains no prompt, answer, authorization header, raw tool
// payload, or provider error message.
package agentaudit

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxRequestIDBytes       = 32
	MaxSessionIDRunes       = 128
	MaxActorRoleRunes       = 16
	MaxFailureSourceRunes   = 32
	MaxFailureCategoryRunes = 32
	MaxFailureCodeRunes     = 64
	MaxErrorTypeRunes       = 64
	MaxToolNameRunes        = 128
	MaxCallIDRunes          = 64
	MaxBusinessFieldRunes   = 32
	MaxUsageJSONBytes       = 16 << 10
	MaxModelCallsJSONBytes  = 1 << 20
	MaxArgumentsJSONBytes   = 2 << 10
	MaxResultJSONBytes      = 4 << 10
	MaxTelemetryCount       = 1_000_000
	MaxPersistedModelCalls  = 64
	MaxPersistedToolCalls   = 128
	MaxCallDurationMS       = int64((24 * time.Hour) / time.Millisecond)
)

var (
	ErrInvalidRun                    = errors.New("invalid agent audit run")
	ErrInvalidToolCall               = errors.New("invalid agent audit tool call")
	ErrConversationOwnershipMismatch = errors.New("agent audit conversation ownership mismatch")
)

type RunStatus string

const (
	RunStatusSuccess     RunStatus = "success"
	RunStatusRejected    RunStatus = "rejected"
	RunStatusBusy        RunStatus = "busy"
	RunStatusUnavailable RunStatus = "unavailable"
	RunStatusTimeout     RunStatus = "timeout"
	RunStatusCanceled    RunStatus = "canceled"
	RunStatusError       RunStatus = "error"
)

func (s RunStatus) IsValid() bool {
	switch s {
	case RunStatusSuccess, RunStatusRejected, RunStatusBusy, RunStatusUnavailable, RunStatusTimeout, RunStatusCanceled, RunStatusError:
		return true
	default:
		return false
	}
}

type TelemetryStatus string

const (
	TelemetryStatusNotStarted  TelemetryStatus = "not_started"
	TelemetryStatusCaptured    TelemetryStatus = "captured"
	TelemetryStatusUnavailable TelemetryStatus = "unavailable"
	TelemetryStatusInvalid     TelemetryStatus = "invalid"
)

// Short aliases keep the domain vocabulary readable at projection call sites.
const (
	TelemetryNotStarted  = TelemetryStatusNotStarted
	TelemetryCaptured    = TelemetryStatusCaptured
	TelemetryUnavailable = TelemetryStatusUnavailable
	TelemetryInvalid     = TelemetryStatusInvalid
)

func (s TelemetryStatus) IsValid() bool {
	switch s {
	case TelemetryStatusNotStarted, TelemetryStatusCaptured, TelemetryStatusUnavailable, TelemetryStatusInvalid:
		return true
	default:
		return false
	}
}

type ToolCallStatus string

const (
	ToolCallStatusSuccess  ToolCallStatus = "success"
	ToolCallStatusError    ToolCallStatus = "error"
	ToolCallStatusTimeout  ToolCallStatus = "timeout"
	ToolCallStatusCanceled ToolCallStatus = "canceled"
	ToolCallStatusPanic    ToolCallStatus = "panic"
)

func (s ToolCallStatus) IsValid() bool {
	switch s {
	case ToolCallStatusSuccess, ToolCallStatusError, ToolCallStatusTimeout, ToolCallStatusCanceled, ToolCallStatusPanic:
		return true
	default:
		return false
	}
}

// Run is the aggregate persisted for one trusted request identifier. Nullable
// counters distinguish missing/invalid telemetry from a captured zero-call
// execution. Each canonical token dimension is nullable because upstream
// adapters cannot distinguish an omitted token field from an integer zero.
type Run struct {
	ID             int64
	RequestID      string
	UserID         *int64
	ConversationID *int64
	SessionID      *string
	ActorRole      *string

	Status           RunStatus
	TelemetryStatus  TelemetryStatus
	HTTPStatus       *int
	FailureSource    *string
	FailureCategory  *string
	FailureCode      *string
	FailureRetryable *bool

	StartedAt  time.Time
	FinishedAt time.Time
	DurationMS int64

	AgentStartedAt  *time.Time
	AgentFinishedAt *time.Time
	AgentDurationMS *int64

	ConversationPersisted *bool

	ModelCallCount     *int
	ToolCallCount      *int
	DroppedModelCalls  *int
	DroppedToolCalls   *int
	ReportedModelCalls *int
	MissingModelCalls  *int

	PromptTokens     *int64
	CompletionTokens *int64
	TotalTokens      *int64
	CachedTokens     *int64
	ReasoningTokens  *int64

	UsageJSON      *string
	ModelCallsJSON *string
	ToolCalls      []ToolCall
	CreatedAt      time.Time
}

// ToolCall contains only the telemetry package's bounded projections. Error
// Message and Param are deliberately absent from this domain type so they
// cannot accidentally be persisted later.
type ToolCall struct {
	ID       int64
	RunID    int64
	Sequence int64
	CallID   *string
	Name     string
	IsWrite  bool
	Status   string

	StartedAt  time.Time
	DurationMS int64

	ArgumentsJSON          string
	ArgumentsOriginalBytes int64
	ArgumentsCapturedBytes int64
	ArgumentsRedacted      bool
	ArgumentsTruncated     bool

	ResultJSON          string
	ResultOriginalBytes int64
	ResultCapturedBytes int64
	ResultRedacted      bool
	ResultTruncated     bool

	ErrorCategory   *string
	ErrorCode       *string
	ErrorType       *string
	ErrorHTTPStatus *int
	ErrorRetryable  *bool

	BusinessObjectType   *string
	BusinessObjectID     *int64
	BusinessObjectStatus *string
	CreatedAt            time.Time
}

// RunMetadata contains the trusted request identity and outer outcome. The
// application layer attaches its already privacy-filtered telemetry before it
// calls Validate and the repository repeats that validation at the write
// boundary. The domain package never imports the Agent telemetry package.
type RunMetadata struct {
	RequestID      string
	UserID         *int64
	ConversationID *int64
	SessionID      *string
	ActorRole      *string

	Status           RunStatus
	TelemetryStatus  TelemetryStatus
	HTTPStatus       *int
	FailureSource    *string
	FailureCategory  *string
	FailureCode      *string
	FailureRetryable *bool

	StartedAt  time.Time
	FinishedAt time.Time

	ConversationPersisted *bool
	CreatedAt             time.Time
}

func NewRun(input RunMetadata) (*Run, error) {
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	run := &Run{
		RequestID:             strings.TrimSpace(input.RequestID),
		UserID:                cloneInt64(input.UserID),
		ConversationID:        cloneInt64(input.ConversationID),
		SessionID:             normalizeString(input.SessionID),
		ActorRole:             normalizeString(input.ActorRole),
		Status:                input.Status,
		TelemetryStatus:       input.TelemetryStatus,
		HTTPStatus:            cloneInt(input.HTTPStatus),
		FailureSource:         normalizeString(input.FailureSource),
		FailureCategory:       normalizeString(input.FailureCategory),
		FailureCode:           normalizeString(input.FailureCode),
		FailureRetryable:      cloneBool(input.FailureRetryable),
		StartedAt:             input.StartedAt,
		FinishedAt:            input.FinishedAt,
		ConversationPersisted: cloneBool(input.ConversationPersisted),
		ToolCalls:             make([]ToolCall, 0),
		CreatedAt:             createdAt,
	}
	if !run.StartedAt.IsZero() && !run.FinishedAt.Before(run.StartedAt) {
		run.DurationMS = run.FinishedAt.Sub(run.StartedAt).Milliseconds()
	}
	if err := run.validateMetadata(); err != nil {
		return nil, err
	}
	return run, nil
}

func (r *Run) Validate() error {
	if r == nil || !r.TelemetryStatus.IsValid() || r.CreatedAt.IsZero() {
		return ErrInvalidRun
	}
	if err := r.validateMetadata(); err != nil {
		return err
	}
	if !validTelemetry(r) {
		return ErrInvalidRun
	}
	for i := range r.ToolCalls {
		if err := r.ToolCalls[i].Validate(); err != nil {
			return err
		}
		if r.ToolCalls[i].RunID != 0 && (r.ID == 0 || r.ToolCalls[i].RunID != r.ID) {
			return ErrInvalidToolCall
		}
	}
	return nil
}

// ValidateForCreate rejects aggregates that already carry database-assigned
// identities. CreateRun is append-only, so accepting a caller-supplied parent
// or child identity would allow the input aggregate to influence primary and
// foreign keys instead of letting the database assign them.
func (r *Run) ValidateForCreate() error {
	if r == nil || r.ID != 0 || r.ConversationPersisted != nil && r.ConversationID == nil {
		return ErrInvalidRun
	}
	for i := range r.ToolCalls {
		if r.ToolCalls[i].ID != 0 || r.ToolCalls[i].RunID != 0 {
			return ErrInvalidToolCall
		}
	}
	return r.Validate()
}

func (r *Run) validateMetadata() error {
	if r == nil || !validRequestID(r.RequestID) || !r.Status.IsValid() || r.CreatedAt.IsZero() {
		return ErrInvalidRun
	}
	if r.UserID != nil && *r.UserID <= 0 || r.ConversationID != nil && *r.ConversationID <= 0 {
		return ErrInvalidRun
	}
	if r.SessionID != nil {
		if r.UserID == nil || !validSessionID(*r.SessionID) {
			return ErrInvalidRun
		}
	}
	if r.ConversationID != nil && (r.UserID == nil || r.SessionID == nil) {
		return ErrInvalidRun
	}
	if r.ActorRole != nil && (r.UserID == nil || !validActorRole(*r.ActorRole)) {
		return ErrInvalidRun
	}
	if r.HTTPStatus != nil && (*r.HTTPStatus < 100 || *r.HTTPStatus > 599) {
		return ErrInvalidRun
	}
	if !validFailure(r) {
		return ErrInvalidRun
	}
	if r.StartedAt.IsZero() || r.FinishedAt.IsZero() || r.FinishedAt.Before(r.StartedAt) || r.DurationMS < 0 || !matchesPersistedDuration(r.ID != 0, r.DurationMS, r.StartedAt, r.FinishedAt) {
		return ErrInvalidRun
	}
	return nil
}

// matchesPersistedDuration accounts for PostgreSQL's microsecond timestamp
// precision. Rounding the two endpoints independently can move their truncated
// millisecond delta by one after a record is loaded back from the database.
// Keeping the tolerance here and in the table constraint makes a valid
// aggregate stable across the persistence round trip without admitting wider
// timing inconsistencies.
func matchesPersistedDuration(persisted bool, durationMS int64, startedAt, finishedAt time.Time) bool {
	const postgresTimestampToleranceMS int64 = 1

	wallClockMS := finishedAt.Sub(startedAt).Milliseconds()
	if durationMS == wallClockMS {
		return true
	}
	if !persisted {
		return false
	}
	return durationMS >= wallClockMS-postgresTimestampToleranceMS && durationMS <= wallClockMS+postgresTimestampToleranceMS
}

func (c *ToolCall) Validate() error {
	if c == nil || c.Sequence <= 0 || !validToolName(c.Name) || !ToolCallStatus(c.Status).IsValid() || c.StartedAt.IsZero() || c.DurationMS < 0 || c.DurationMS > MaxCallDurationMS {
		return ErrInvalidToolCall
	}
	if c.CallID != nil && !validCallID(*c.CallID) {
		return ErrInvalidToolCall
	}
	if !validPayload(c.ArgumentsJSON, c.ArgumentsOriginalBytes, c.ArgumentsCapturedBytes, MaxArgumentsJSONBytes) ||
		!validPayload(c.ResultJSON, c.ResultOriginalBytes, c.ResultCapturedBytes, MaxResultJSONBytes) {
		return ErrInvalidToolCall
	}
	if c.ErrorHTTPStatus != nil && (*c.ErrorHTTPStatus < 100 || *c.ErrorHTTPStatus > 599) {
		return ErrInvalidToolCall
	}
	errorPresent := c.ErrorCategory != nil || c.ErrorCode != nil || c.ErrorType != nil || c.ErrorHTTPStatus != nil || c.ErrorRetryable != nil
	if c.Status == string(ToolCallStatusSuccess) {
		if errorPresent {
			return ErrInvalidToolCall
		}
	} else if c.ErrorCategory == nil || c.ErrorCode == nil || c.ErrorRetryable == nil {
		return ErrInvalidToolCall
	}
	if c.ErrorCategory != nil && !validIdentifier(*c.ErrorCategory, MaxFailureCategoryRunes) {
		return ErrInvalidToolCall
	}
	if c.ErrorCode != nil && !validIdentifier(*c.ErrorCode, MaxFailureCodeRunes) {
		return ErrInvalidToolCall
	}
	if c.ErrorType != nil && !validIdentifier(*c.ErrorType, MaxErrorTypeRunes) {
		return ErrInvalidToolCall
	}
	if c.BusinessObjectID != nil && (*c.BusinessObjectID <= 0 || c.BusinessObjectType == nil) {
		return ErrInvalidToolCall
	}
	if c.BusinessObjectStatus != nil && c.BusinessObjectType == nil {
		return ErrInvalidToolCall
	}
	if !c.IsWrite && (c.BusinessObjectType != nil || c.BusinessObjectID != nil || c.BusinessObjectStatus != nil) {
		return ErrInvalidToolCall
	}
	for _, field := range []*string{c.BusinessObjectType, c.BusinessObjectStatus} {
		if field != nil && !validIdentifier(*field, MaxBusinessFieldRunes) {
			return ErrInvalidToolCall
		}
	}
	return nil
}

func validFailure(run *Run) bool {
	if run.FailureSource != nil && !validFailureSource(*run.FailureSource) {
		return false
	}
	if run.FailureCategory != nil && !validIdentifier(*run.FailureCategory, MaxFailureCategoryRunes) {
		return false
	}
	if run.FailureCode != nil && !validIdentifier(*run.FailureCode, MaxFailureCodeRunes) {
		return false
	}
	any := run.FailureSource != nil || run.FailureCategory != nil || run.FailureCode != nil || run.FailureRetryable != nil
	if run.Status == RunStatusSuccess {
		return !any
	}
	return run.FailureSource != nil && run.FailureCategory != nil && run.FailureCode != nil && run.FailureRetryable != nil
}

func validFailureSource(value string) bool {
	switch value {
	case "main", "agent", "transport", "model", "tool":
		return true
	default:
		return false
	}
}

func validTelemetry(run *Run) bool {
	counts := []*int{
		run.ModelCallCount, run.ToolCallCount, run.DroppedModelCalls,
		run.DroppedToolCalls, run.ReportedModelCalls, run.MissingModelCalls,
	}
	tokens := []*int64{run.PromptTokens, run.CompletionTokens, run.TotalTokens, run.CachedTokens, run.ReasoningTokens}
	allCountsNil, allCountsPresent := allIntsNil(counts), allIntsPresent(counts)
	allTokensNil := allInt64sNil(tokens)
	for _, value := range tokens {
		if value != nil && *value <= 0 {
			return false
		}
	}

	switch run.TelemetryStatus {
	case TelemetryStatusUnavailable, TelemetryStatusInvalid:
		return allCountsNil && allTokensNil && run.AgentStartedAt == nil && run.AgentFinishedAt == nil && run.AgentDurationMS == nil && run.UsageJSON == nil && run.ModelCallsJSON == nil && len(run.ToolCalls) == 0
	case TelemetryStatusNotStarted:
		if !allCountsPresent || !allTokensNil || !allIntsEqual(counts, 0) || run.AgentStartedAt != nil || run.AgentFinishedAt != nil || run.AgentDurationMS != nil || len(run.ToolCalls) != 0 {
			return false
		}
		return validJSONObject(run.UsageJSON, MaxUsageJSONBytes) && validEmptyJSONArray(run.ModelCallsJSON, MaxModelCallsJSONBytes)
	case TelemetryStatusCaptured:
		const clockSkew = 5 * time.Minute
		if !allCountsPresent || run.AgentStartedAt == nil || run.AgentFinishedAt == nil || run.AgentDurationMS == nil || run.AgentStartedAt.IsZero() || run.AgentFinishedAt.IsZero() || *run.AgentDurationMS < 0 || *run.AgentDurationMS > MaxCallDurationMS ||
			run.AgentStartedAt.Before(run.StartedAt.Add(-clockSkew)) || run.AgentStartedAt.After(run.FinishedAt.Add(clockSkew)) ||
			run.AgentFinishedAt.Before(run.StartedAt.Add(-clockSkew)) || run.AgentFinishedAt.After(run.FinishedAt.Add(clockSkew)) {
			return false
		}
		for _, value := range counts {
			if *value < 0 || *value > MaxTelemetryCount {
				return false
			}
		}
		if *run.DroppedModelCalls > *run.ModelCallCount || *run.DroppedToolCalls > *run.ToolCallCount || *run.ReportedModelCalls+*run.MissingModelCalls != *run.ModelCallCount {
			return false
		}
		modelCalls, ok := jsonArrayLength(run.ModelCallsJSON, MaxModelCallsJSONBytes)
		if !ok || modelCalls > MaxPersistedModelCalls || len(run.ToolCalls) > MaxPersistedToolCalls || modelCalls+*run.DroppedModelCalls != *run.ModelCallCount || len(run.ToolCalls)+*run.DroppedToolCalls != *run.ToolCallCount || !validJSONObject(run.UsageJSON, MaxUsageJSONBytes) {
			return false
		}
		seen := make(map[int64]struct{}, len(run.ToolCalls))
		for i := range run.ToolCalls {
			if _, exists := seen[run.ToolCalls[i].Sequence]; exists {
				return false
			}
			seen[run.ToolCalls[i].Sequence] = struct{}{}
		}
		completeUsage := *run.ReportedModelCalls > 0 && *run.MissingModelCalls == 0
		return allTokensNil || completeUsage
	default:
		return false
	}
}

func validPayload(value string, originalBytes, capturedBytes int64, limit int) bool {
	return originalBytes >= 0 && capturedBytes >= 0 && capturedBytes == int64(len(value)) && len(value) <= limit && utf8.ValidString(value) && json.Valid([]byte(value))
}

func validJSONObject(value *string, limit int) bool {
	if value == nil || len(*value) == 0 || len(*value) > limit || !json.Valid([]byte(*value)) {
		return false
	}
	trimmed := strings.TrimSpace(*value)
	return strings.HasPrefix(trimmed, "{")
}

func validEmptyJSONArray(value *string, limit int) bool {
	length, ok := jsonArrayLength(value, limit)
	return ok && length == 0
}

func jsonArrayLength(value *string, limit int) (int, bool) {
	if value == nil || len(*value) == 0 || len(*value) > limit || !json.Valid([]byte(*value)) {
		return 0, false
	}
	var items []json.RawMessage
	if err := json.Unmarshal([]byte(*value), &items); err != nil || items == nil {
		return 0, false
	}
	return len(items), true
}

func validRequestID(value string) bool {
	if len(value) != MaxRequestIDBytes {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func validSessionID(value string) bool {
	if value == "" || utf8.RuneCountInString(value) > MaxSessionIDRunes {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func validToolName(value string) bool {
	return value == "[invalid]" || validIdentifier(value, MaxToolNameRunes)
}

func validCallID(value string) bool {
	const prefix = "call-"
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	digits := value[len(prefix):]
	if len(digits) < 1 || len(digits) > 20 || digits[0] < '1' || digits[0] > '9' {
		return false
	}
	for i := 1; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return false
		}
	}
	return true
}

func validActorRole(value string) bool {
	switch value {
	case "member", "captain", "super_admin":
		return true
	default:
		return false
	}
}

func validIdentifier(value string, maxRunes int) bool {
	if value == "" || utf8.RuneCountInString(value) > maxRunes {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '-' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func allIntsNil(values []*int) bool {
	for _, value := range values {
		if value != nil {
			return false
		}
	}
	return true
}

func allIntsPresent(values []*int) bool {
	for _, value := range values {
		if value == nil {
			return false
		}
	}
	return true
}

func allIntsEqual(values []*int, want int) bool {
	for _, value := range values {
		if value == nil || *value != want {
			return false
		}
	}
	return true
}

func allInt64sNil(values []*int64) bool {
	for _, value := range values {
		if value != nil {
			return false
		}
	}
	return true
}

func normalizeString(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil
	}
	return &normalized
}

func normalizeJSON(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	return &normalized
}

func cloneToolCalls(source []ToolCall) []ToolCall {
	result := make([]ToolCall, len(source))
	for i := range source {
		result[i] = source[i]
		result[i].CallID = normalizeString(source[i].CallID)
		result[i].ErrorCategory = normalizeString(source[i].ErrorCategory)
		result[i].ErrorCode = normalizeString(source[i].ErrorCode)
		result[i].ErrorType = normalizeString(source[i].ErrorType)
		result[i].ErrorHTTPStatus = cloneInt(source[i].ErrorHTTPStatus)
		result[i].ErrorRetryable = cloneBool(source[i].ErrorRetryable)
		result[i].BusinessObjectType = normalizeString(source[i].BusinessObjectType)
		result[i].BusinessObjectID = cloneInt64(source[i].BusinessObjectID)
		result[i].BusinessObjectStatus = normalizeString(source[i].BusinessObjectStatus)
	}
	return result
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneInt64(value *int64) *int64 {
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

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
