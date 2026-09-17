package telemetry

import (
	"context"
	"crypto/sha256"
	"sort"
	"strconv"
	"sync"
	"time"
)

const (
	maxRecordedModelCalls = 64
	maxRecordedToolCalls  = 128
	maxOpaqueIDBytes      = 512
)

type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	CachedTokens     int `json:"cached_tokens"`
	ReasoningTokens  int `json:"reasoning_tokens"`
}

type UsageSummary struct {
	ReportedModelCalls int         `json:"reported_model_calls"`
	MissingModelCalls  int         `json:"missing_model_calls"`
	ReportedTotals     TokenUsage  `json:"reported_totals"`
	CompleteTotals     *TokenUsage `json:"complete_totals,omitempty"`
}

type ErrorInfo struct {
	Category   string `json:"category"`
	Code       string `json:"code,omitempty"`
	Type       string `json:"type,omitempty"`
	Param      string `json:"param,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
	Retryable  bool   `json:"retryable"`
	Message    string `json:"message,omitempty"`
}

// PayloadCapture is a bounded, valid-JSON projection. JSON never contains the
// complete raw tool payload; OriginalBytes is retained only for diagnostics.
type PayloadCapture struct {
	JSON          string `json:"json"`
	OriginalBytes int    `json:"original_bytes"`
	CapturedBytes int    `json:"captured_bytes"`
	Redacted      bool   `json:"redacted"`
	Truncated     bool   `json:"truncated"`
}

type ToolRequest struct {
	// CallID is a request-local pseudonym, never the provider's raw call ID.
	CallID string `json:"call_id,omitempty"`
	Name   string `json:"name"`
}

type ModelCall struct {
	Sequence      uint64    `json:"sequence"`
	Provider      string    `json:"provider,omitempty"`
	Model         string    `json:"model,omitempty"`
	Status        string    `json:"status"`
	StartedAt     time.Time `json:"started_at"`
	DurationMS    int64     `json:"duration_ms"`
	InputMessages int       `json:"input_messages"`
	ToolsOffered  int       `json:"tools_offered"`
	FinishReason  string    `json:"finish_reason,omitempty"`
	// ProviderResponseAlias is request-local and only indicates identity within
	// this agent run. It is never the provider's raw response identifier.
	ProviderResponseAlias string        `json:"provider_response_alias,omitempty"`
	Usage                 *TokenUsage   `json:"usage,omitempty"`
	ToolRequests          []ToolRequest `json:"tool_requests,omitempty"`
	ToolRequestsTruncated bool          `json:"tool_requests_truncated,omitempty"`
	Error                 *ErrorInfo    `json:"error,omitempty"`
}

type ToolCall struct {
	Sequence uint64 `json:"sequence"`
	// CallID is a request-local pseudonym, never the provider's raw call ID.
	CallID     string         `json:"call_id,omitempty"`
	Name       string         `json:"name"`
	Status     string         `json:"status"`
	StartedAt  time.Time      `json:"started_at"`
	DurationMS int64          `json:"duration_ms"`
	Arguments  PayloadCapture `json:"arguments"`
	Result     PayloadCapture `json:"result"`
	Error      *ErrorInfo     `json:"error,omitempty"`
}

type Snapshot struct {
	StartedAt         time.Time    `json:"started_at"`
	FinishedAt        time.Time    `json:"finished_at"`
	DurationMS        int64        `json:"duration_ms"`
	ModelCallCount    int          `json:"model_call_count"`
	ToolCallCount     int          `json:"tool_call_count"`
	DroppedModelCalls int          `json:"dropped_model_calls,omitempty"`
	DroppedToolCalls  int          `json:"dropped_tool_calls,omitempty"`
	Usage             UsageSummary `json:"usage"`
	ModelCalls        []ModelCall  `json:"model_calls"`
	ToolCalls         []ToolCall   `json:"tool_calls"`
}

type Collector struct {
	mu sync.Mutex

	now       func() time.Time
	startedAt time.Time
	sequence  uint64

	modelCalls          []ModelCall
	toolCalls           []ToolCall
	reservedModelCalls  int
	reservedToolCalls   int
	droppedModelCalls   int
	droppedToolCalls    int
	callIDAliases       map[[sha256.Size]byte]string
	responseIDAliases   map[[sha256.Size]byte]string
	nextCallIDAlias     uint64
	nextResponseIDAlias uint64
}

type modelSpan struct {
	sequence      uint64
	provider      string
	model         string
	startedAt     time.Time
	inputMessages int
	toolsOffered  int
}

type toolSpan struct {
	sequence  uint64
	name      string
	callID    string
	startedAt time.Time
	arguments PayloadCapture
}

func NewCollector() *Collector {
	return newCollector(time.Now)
}

func newCollector(now func() time.Time) *Collector {
	if now == nil {
		now = time.Now
	}
	startedAt := now()
	return &Collector{now: now, startedAt: startedAt}
}

func (c *Collector) beginModel(provider, modelName string, inputMessages, toolsOffered int) *modelSpan {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.reservedModelCalls >= maxRecordedModelCalls {
		c.droppedModelCalls++
		return nil
	}
	c.reservedModelCalls++
	c.sequence++
	return &modelSpan{
		sequence:      c.sequence,
		provider:      safeProviderIdentifier(provider),
		model:         safeModelIdentifier(modelName),
		startedAt:     c.now(),
		inputMessages: inputMessages,
		toolsOffered:  toolsOffered,
	}
}

func (c *Collector) finishModel(span *modelSpan, outcome modelOutcome) {
	if c == nil || span == nil {
		return
	}
	finishedAt := c.now()
	call := ModelCall{
		Sequence:              span.sequence,
		Provider:              span.provider,
		Model:                 span.model,
		Status:                outcome.status,
		StartedAt:             span.startedAt,
		DurationMS:            durationMillis(span.startedAt, finishedAt),
		InputMessages:         span.inputMessages,
		ToolsOffered:          span.toolsOffered,
		FinishReason:          truncateRunes(outcome.finishReason, 128),
		Usage:                 cloneUsage(outcome.usage),
		ToolRequests:          append([]ToolRequest(nil), outcome.toolRequests...),
		ToolRequestsTruncated: outcome.toolRequestsTruncated,
		Error:                 cloneError(outcome.err),
	}
	c.mu.Lock()
	call.ProviderResponseAlias = c.aliasResponseIDLocked(outcome.providerResponseID)
	for index := range call.ToolRequests {
		call.ToolRequests[index].CallID = c.aliasCallIDLocked(call.ToolRequests[index].CallID)
	}
	c.modelCalls = append(c.modelCalls, call)
	c.mu.Unlock()
}

func (c *Collector) beginTool(name, callID, arguments string) *toolSpan {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	if c.reservedToolCalls >= maxRecordedToolCalls {
		c.droppedToolCalls++
		c.mu.Unlock()
		return nil
	}
	c.reservedToolCalls++
	c.sequence++
	sequence := c.sequence
	safeName := safeToolIdentifier(name)
	safeCallID := c.aliasCallIDLocked(callID)
	startedAt := c.now()
	c.mu.Unlock()
	argumentsCapture := captureToolPayload(name, payloadArguments, arguments)
	return &toolSpan{
		sequence:  sequence,
		name:      safeName,
		callID:    safeCallID,
		startedAt: startedAt,
		arguments: argumentsCapture,
	}
}

func (c *Collector) aliasCallIDLocked(raw string) string {
	if raw == "" || len(raw) > maxOpaqueIDBytes {
		return ""
	}
	if c.callIDAliases == nil {
		c.callIDAliases = make(map[[sha256.Size]byte]string)
	}
	digest := sha256.Sum256(append([]byte("call\x00"), []byte(raw)...))
	if alias, ok := c.callIDAliases[digest]; ok {
		return alias
	}
	c.nextCallIDAlias++
	alias := "call-" + strconv.FormatUint(c.nextCallIDAlias, 10)
	c.callIDAliases[digest] = alias
	return alias
}

func (c *Collector) aliasResponseIDLocked(raw string) string {
	if raw == "" || len(raw) > maxOpaqueIDBytes {
		return ""
	}
	if c.responseIDAliases == nil {
		c.responseIDAliases = make(map[[sha256.Size]byte]string)
	}
	digest := sha256.Sum256(append([]byte("response\x00"), []byte(raw)...))
	if alias, ok := c.responseIDAliases[digest]; ok {
		return alias
	}
	c.nextResponseIDAlias++
	alias := "response-" + strconv.FormatUint(c.nextResponseIDAlias, 10)
	c.responseIDAliases[digest] = alias
	return alias
}

func (c *Collector) finishTool(span *toolSpan, result string, err *ErrorInfo, status string) {
	if c == nil || span == nil {
		return
	}
	finishedAt := c.now()
	call := ToolCall{
		Sequence:   span.sequence,
		CallID:     span.callID,
		Name:       span.name,
		Status:     status,
		StartedAt:  span.startedAt,
		DurationMS: durationMillis(span.startedAt, finishedAt),
		Arguments:  span.arguments,
		Result:     captureToolPayload(span.name, payloadResult, result),
		Error:      cloneError(err),
	}
	c.mu.Lock()
	c.toolCalls = append(c.toolCalls, call)
	c.mu.Unlock()
}

func (c *Collector) Snapshot() Snapshot {
	if c == nil {
		return Snapshot{ModelCalls: []ModelCall{}, ToolCalls: []ToolCall{}}
	}
	c.mu.Lock()
	finishedAt := c.now()
	models := cloneModelCalls(c.modelCalls)
	tools := cloneToolCalls(c.toolCalls)
	droppedModels := c.droppedModelCalls
	droppedTools := c.droppedToolCalls
	reservedModels := c.reservedModelCalls
	reservedTools := c.reservedToolCalls
	startedAt := c.startedAt
	c.mu.Unlock()
	sort.SliceStable(models, func(i, j int) bool { return models[i].Sequence < models[j].Sequence })
	sort.SliceStable(tools, func(i, j int) bool { return tools[i].Sequence < tools[j].Sequence })
	inFlightModels := reservedModels - len(models)
	if inFlightModels < 0 {
		inFlightModels = 0
	}
	usage := summarizeUsage(models, droppedModels+inFlightModels)
	return Snapshot{
		StartedAt:         startedAt,
		FinishedAt:        finishedAt,
		DurationMS:        durationMillis(startedAt, finishedAt),
		ModelCallCount:    reservedModels + droppedModels,
		ToolCallCount:     reservedTools + droppedTools,
		DroppedModelCalls: droppedModels,
		DroppedToolCalls:  droppedTools,
		Usage:             usage,
		ModelCalls:        models,
		ToolCalls:         tools,
	}
}

func summarizeUsage(calls []ModelCall, dropped int) UsageSummary {
	result := UsageSummary{MissingModelCalls: dropped}
	for _, call := range calls {
		if call.Usage == nil {
			result.MissingModelCalls++
			continue
		}
		result.ReportedModelCalls++
		result.ReportedTotals.PromptTokens = saturatingAdd(result.ReportedTotals.PromptTokens, call.Usage.PromptTokens)
		result.ReportedTotals.CompletionTokens = saturatingAdd(result.ReportedTotals.CompletionTokens, call.Usage.CompletionTokens)
		result.ReportedTotals.TotalTokens = saturatingAdd(result.ReportedTotals.TotalTokens, call.Usage.TotalTokens)
		result.ReportedTotals.CachedTokens = saturatingAdd(result.ReportedTotals.CachedTokens, call.Usage.CachedTokens)
		result.ReportedTotals.ReasoningTokens = saturatingAdd(result.ReportedTotals.ReasoningTokens, call.Usage.ReasoningTokens)
	}
	if len(calls) > 0 && result.MissingModelCalls == 0 {
		complete := result.ReportedTotals
		result.CompleteTotals = &complete
	}
	return result
}

func saturatingAdd(left, right int) int {
	maximum := int(^uint(0) >> 1)
	if right > 0 && left > maximum-right {
		return maximum
	}
	return left + right
}

func cloneModelCalls(source []ModelCall) []ModelCall {
	result := make([]ModelCall, len(source))
	for i := range source {
		result[i] = source[i]
		result[i].Usage = cloneUsage(source[i].Usage)
		result[i].Error = cloneError(source[i].Error)
		result[i].ToolRequests = append([]ToolRequest(nil), source[i].ToolRequests...)
	}
	return result
}

func cloneToolCalls(source []ToolCall) []ToolCall {
	result := make([]ToolCall, len(source))
	copy(result, source)
	for i := range result {
		result[i].Error = cloneError(source[i].Error)
	}
	return result
}

func cloneUsage(value *TokenUsage) *TokenUsage {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneError(value *ErrorInfo) *ErrorInfo {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func durationMillis(start, end time.Time) int64 {
	if start.IsZero() || end.Before(start) {
		return 0
	}
	return end.Sub(start).Milliseconds()
}

type collectorContextKey struct{}

func WithCollector(ctx context.Context, collector *Collector) context.Context {
	if collector == nil {
		return ctx
	}
	return context.WithValue(ctx, collectorContextKey{}, collector)
}

func FromContext(ctx context.Context) *Collector {
	if ctx == nil {
		return nil
	}
	collector, _ := ctx.Value(collectorContextKey{}).(*Collector)
	return collector
}
