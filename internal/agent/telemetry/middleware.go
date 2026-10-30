package telemetry

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"

	"SWPUCAT/internal/agent/business"

	componentopenai "github.com/cloudwego/eino-ext/components/model/openai"
	aclopenai "github.com/cloudwego/eino-ext/libs/acl/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	goopenai "github.com/meguminnnnnnnnn/go-openai"
)

const maxToolRequestsPerModelCall = 32

type modelOutcome struct {
	status                string
	finishReason          string
	providerResponseID    string
	usage                 *TokenUsage
	toolRequests          []ToolRequest
	toolRequestsTruncated bool
	err                   *ErrorInfo
}

type streamToolSlot struct {
	callID  string
	name    string
	invalid bool
}

type streamOutcomeAccumulator struct {
	outcome      modelOutcome
	indexedSlots map[int]int
	toolSlots    []streamToolSlot
}

func newStreamOutcomeAccumulator() *streamOutcomeAccumulator {
	return &streamOutcomeAccumulator{
		outcome:      modelOutcome{status: "success"},
		indexedSlots: make(map[int]int),
		toolSlots:    make([]streamToolSlot, 0, maxToolRequestsPerModelCall),
	}
}

// Middleware is stateless. Request-specific data is resolved from ctx, so one
// instance can safely be shared by every Engine worker.
type Middleware struct {
	*adk.BaseChatModelAgentMiddleware
	provider  string
	modelName string
}

func NewMiddleware(provider, modelName string) *Middleware {
	return &Middleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		provider:                     safeProviderIdentifier(provider),
		modelName:                    safeModelIdentifier(modelName),
	}
}

func (m *Middleware) WrapModel(_ context.Context, base model.BaseChatModel, modelContext *adk.ModelContext) (model.BaseChatModel, error) {
	toolsOffered := 0
	offeredToolNames := make(map[string]struct{})
	if modelContext != nil {
		toolsOffered = len(modelContext.Tools)
		for _, info := range modelContext.Tools {
			if info != nil && safeToolIdentifier(info.Name) == info.Name {
				offeredToolNames[info.Name] = struct{}{}
			}
		}
	}
	return &observedModel{
		BaseChatModel:    base,
		provider:         m.provider,
		modelName:        m.modelName,
		toolsOffered:     toolsOffered,
		offeredToolNames: offeredToolNames,
	}, nil
}

func (m *Middleware) WrapInvokableToolCall(
	_ context.Context,
	endpoint adk.InvokableToolCallEndpoint,
	toolContext *adk.ToolContext,
) (adk.InvokableToolCallEndpoint, error) {
	name, callID := "", ""
	if toolContext != nil {
		name, callID = toolContext.Name, toolContext.CallID
	}
	return func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (result string, err error) {
		collector := FromContext(ctx)
		if collector == nil {
			return endpoint(ctx, argumentsInJSON, opts...)
		}
		span := collector.beginTool(name, callID, argumentsInJSON)
		defer func() {
			if recovered := recover(); recovered != nil {
				collector.finishTool(span, result, panicError(recovered), "panic")
				panic(recovered)
			}
			classified := classifyError(err)
			collector.finishTool(span, result, classified, statusForError(classified))
		}()
		return endpoint(ctx, argumentsInJSON, opts...)
	}, nil
}

type observedModel struct {
	model.BaseChatModel
	provider         string
	modelName        string
	toolsOffered     int
	offeredToolNames map[string]struct{}
}

func (m *observedModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (response *schema.Message, err error) {
	collector := FromContext(ctx)
	if collector == nil {
		return m.BaseChatModel.Generate(ctx, input, opts...)
	}
	span := collector.beginModel(m.provider, m.modelName, len(input), m.toolsOffered)
	defer func() {
		if recovered := recover(); recovered != nil {
			collector.finishModel(span, modelOutcome{status: "panic", err: panicError(recovered)})
			panic(recovered)
		}
		collector.finishModel(span, modelOutcomeFromMessage(response, err, m.offeredToolNames))
	}()
	return m.BaseChatModel.Generate(ctx, input, opts...)
}

func (m *observedModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	collector := FromContext(ctx)
	if collector == nil {
		return m.BaseChatModel.Stream(ctx, input, opts...)
	}
	span := collector.beginModel(m.provider, m.modelName, len(input), m.toolsOffered)
	defer func() {
		if recovered := recover(); recovered != nil {
			collector.finishModel(span, modelOutcome{status: "panic", err: panicError(recovered)})
			panic(recovered)
		}
	}()
	source, err := m.BaseChatModel.Stream(ctx, input, opts...)
	if err != nil {
		collector.finishModel(span, modelOutcome{status: statusForError(classifyError(err)), err: classifyError(err)})
		return nil, err
	}
	if source == nil {
		err = errors.New("model stream returned a nil reader")
		classified := classifyError(err)
		collector.finishModel(span, modelOutcome{status: statusForError(classified), err: classified})
		return nil, err
	}
	result, writer := schema.Pipe[*schema.Message](1)
	go func() {
		accumulator := newStreamOutcomeAccumulator()
		finished := false
		finish := func() {
			if finished {
				return
			}
			collector.finishModel(span, accumulator.snapshot(m.offeredToolNames))
			finished = true
		}
		sourceClosed := false
		closeSource := func() bool {
			if sourceClosed {
				return false
			}
			sourceClosed = true
			return runWithoutPanic(source.Close) != nil
		}
		defer func() {
			var deliveryErr error
			if recovered := recover(); recovered != nil {
				accumulator.setPanic()
				deliveryErr = errors.New("model stream failed: execution panicked")
			}
			if closeSource() {
				accumulator.setPanic()
				deliveryErr = errors.New("model stream failed: execution panicked")
			}
			_ = runWithoutPanic(finish)
			if deliveryErr != nil {
				_ = runWithoutPanic(func() { writer.Send(nil, deliveryErr) })
			}
			_ = runWithoutPanic(writer.Close)
		}()
		for {
			message, receiveErr := source.Recv()
			if errors.Is(receiveErr, io.EOF) {
				return
			}
			if receiveErr != nil {
				accumulator.setError(receiveErr)
				deliveryErr := receiveErr
				if closeSource() {
					accumulator.setPanic()
					deliveryErr = errors.New("model stream failed: execution panicked")
				}
				finish()
				writer.Send(nil, deliveryErr)
				return
			}
			accumulator.observe(message, m.offeredToolNames)
			if writer.Send(message, nil) {
				accumulator.outcome.err = &ErrorInfo{Category: "canceled", Code: "downstream_closed", Message: "model stream consumer closed early"}
				accumulator.outcome.status = "canceled"
				return
			}
		}
	}()
	return result, nil
}

func (a *streamOutcomeAccumulator) observe(message *schema.Message, offeredToolNames map[string]struct{}) {
	if a == nil || message == nil {
		return
	}
	if message.ResponseMeta != nil {
		if message.ResponseMeta.FinishReason != "" {
			a.outcome.finishReason = safeFinishReason(message.ResponseMeta.FinishReason)
		}
		a.outcome.usage = mergeUsageMaximum(a.outcome.usage, normalizeUsage(message.ResponseMeta.Usage))
	}
	if responseID := boundedOpaqueID(aclopenai.GetRequestID(message)); responseID != "" {
		a.outcome.providerResponseID = responseID
	}
	toolCalls := message.ToolCalls
	if len(toolCalls) > maxToolRequestsPerModelCall {
		toolCalls = toolCalls[:maxToolRequestsPerModelCall]
		a.outcome.toolRequestsTruncated = true
	}
	for _, call := range toolCalls {
		slotIndex := -1
		if call.Index != nil {
			slotIndex = a.indexedSlots[*call.Index]
			if slotIndex == 0 {
				if len(a.toolSlots) >= maxToolRequestsPerModelCall {
					a.outcome.toolRequestsTruncated = true
					break
				}
				a.toolSlots = append(a.toolSlots, streamToolSlot{})
				slotIndex = len(a.toolSlots)
				a.indexedSlots[*call.Index] = slotIndex
			}
			slotIndex--
		} else {
			if len(a.toolSlots) >= maxToolRequestsPerModelCall {
				a.outcome.toolRequestsTruncated = true
				break
			}
			a.toolSlots = append(a.toolSlots, streamToolSlot{})
			slotIndex = len(a.toolSlots) - 1
		}
		slot := &a.toolSlots[slotIndex]
		if call.ID != "" {
			callID := boundedOpaqueID(call.ID)
			if callID == "" || slot.callID != "" && slot.callID != callID {
				slot.invalid = true
			} else {
				slot.callID = callID
			}
		}
		if call.Function.Name != "" {
			if _, known := offeredToolNames[call.Function.Name]; !known || slot.name != "" && slot.name != call.Function.Name {
				slot.invalid = true
			} else {
				slot.name = call.Function.Name
			}
		}
	}
}

func (a *streamOutcomeAccumulator) snapshot(_ map[string]struct{}) modelOutcome {
	if a == nil {
		return modelOutcome{status: "error", err: &ErrorInfo{Category: "execution", Code: "telemetry_unavailable"}}
	}
	for _, slot := range a.toolSlots {
		request := ToolRequest{Name: "[unknown]"}
		if !slot.invalid && slot.name != "" {
			request = ToolRequest{CallID: slot.callID, Name: slot.name}
		}
		a.outcome.toolRequests = append(a.outcome.toolRequests, request)
	}
	return a.outcome
}

func (a *streamOutcomeAccumulator) setError(err error) {
	if a == nil {
		return
	}
	a.outcome.err = classifyError(err)
	a.outcome.status = statusForError(a.outcome.err)
}

func (a *streamOutcomeAccumulator) setPanic() {
	if a == nil {
		return
	}
	a.outcome.status = "panic"
	a.outcome.err = panicError(nil)
}

func runWithoutPanic(action func()) (recovered any) {
	defer func() { recovered = recover() }()
	action()
	return nil
}

func modelOutcomeFromMessage(message *schema.Message, err error, offeredToolNames map[string]struct{}) modelOutcome {
	if err != nil {
		classified := classifyError(err)
		return modelOutcome{status: statusForError(classified), err: classified}
	}
	outcome := modelOutcome{status: "success"}
	mergeMessageOutcome(&outcome, message, offeredToolNames)
	return outcome
}

func mergeMessageOutcome(outcome *modelOutcome, message *schema.Message, offeredToolNames map[string]struct{}) {
	if outcome == nil || message == nil {
		return
	}
	if message.ResponseMeta != nil {
		if message.ResponseMeta.FinishReason != "" {
			outcome.finishReason = safeFinishReason(message.ResponseMeta.FinishReason)
		}
		if usage := normalizeUsage(message.ResponseMeta.Usage); usage != nil {
			outcome.usage = usage
		}
	}
	if responseID := boundedOpaqueID(aclopenai.GetRequestID(message)); responseID != "" {
		outcome.providerResponseID = responseID
	}
	if len(message.ToolCalls) == 0 {
		return
	}
	for index, call := range message.ToolCalls {
		if index >= maxToolRequestsPerModelCall || len(outcome.toolRequests) >= maxToolRequestsPerModelCall {
			outcome.toolRequestsTruncated = true
			break
		}
		_, knownTool := offeredToolNames[call.Function.Name]
		request := ToolRequest{
			Name: "[unknown]",
		}
		if knownTool {
			request.CallID = boundedOpaqueID(call.ID)
			request.Name = call.Function.Name
		}
		outcome.toolRequests = append(outcome.toolRequests, request)
	}
}

func normalizeUsage(usage *schema.TokenUsage) *TokenUsage {
	if usage == nil {
		return nil
	}
	result := &TokenUsage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
		CachedTokens:     usage.PromptTokenDetails.CachedTokens,
		ReasoningTokens:  usage.CompletionTokensDetails.ReasoningTokens,
	}
	maximumPerCall := int(^uint(0)>>1) / maxRecordedModelCalls
	for _, value := range []int{result.PromptTokens, result.CompletionTokens, result.TotalTokens, result.CachedTokens, result.ReasoningTokens} {
		if value < 0 || value > maximumPerCall {
			return nil
		}
	}
	// The OpenAI-compatible adapter materializes an all-zero usage object when
	// a provider omits usage. A real model request cannot consume zero prompt
	// and completion tokens, so normalize that ambiguous value to unknown.
	if *result == (TokenUsage{}) {
		return nil
	}
	return result
}

func mergeUsageMaximum(current, candidate *TokenUsage) *TokenUsage {
	if candidate == nil {
		return current
	}
	if current == nil {
		return cloneUsage(candidate)
	}
	if candidate.PromptTokens > current.PromptTokens {
		current.PromptTokens = candidate.PromptTokens
	}
	if candidate.CompletionTokens > current.CompletionTokens {
		current.CompletionTokens = candidate.CompletionTokens
	}
	if candidate.TotalTokens > current.TotalTokens {
		current.TotalTokens = candidate.TotalTokens
	}
	if candidate.CachedTokens > current.CachedTokens {
		current.CachedTokens = candidate.CachedTokens
	}
	if candidate.ReasoningTokens > current.ReasoningTokens {
		current.ReasoningTokens = candidate.ReasoningTokens
	}
	return current
}

func boundedOpaqueID(value string) string {
	if value == "" || len(value) > maxOpaqueIDBytes {
		return ""
	}
	return value
}

func statusForError(err *ErrorInfo) string {
	if err == nil {
		return "success"
	}
	switch err.Category {
	case "canceled":
		return "canceled"
	case "timeout":
		return "timeout"
	case "panic":
		return "panic"
	default:
		return "error"
	}
}

func classifyError(err error) *ErrorInfo {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return &ErrorInfo{Category: "canceled", Code: "context_canceled", Message: "request canceled"}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &ErrorInfo{Category: "timeout", Code: "deadline_exceeded", Retryable: true, Message: "request deadline exceeded"}
	}
	if errors.Is(err, business.ErrToolCallLimit) {
		return &ErrorInfo{Category: "limit", Code: "tool_call_limit", Message: "agent tool call limit exceeded"}
	}
	var apiErr *componentopenai.APIError
	if errors.As(err, &apiErr) {
		status := safeHTTPStatus(apiErr.HTTPStatusCode)
		code := safeProviderErrorCode(apiErr.Code, status)
		return &ErrorInfo{
			Category:   providerErrorCategory(status),
			Code:       code,
			Type:       safeProviderErrorType(apiErr.Type),
			HTTPStatus: status,
			Retryable:  retryableProviderError(status, code),
			Message:    "model provider request failed",
		}
	}
	var requestErr *goopenai.RequestError
	var providerAPIErr *goopenai.APIError
	if errors.As(err, &providerAPIErr) {
		status := safeHTTPStatus(providerAPIErr.HTTPStatusCode)
		code := safeProviderErrorCode(providerAPIErr.Code, status)
		return &ErrorInfo{
			Category:   providerErrorCategory(status),
			Code:       code,
			Type:       safeProviderErrorType(providerAPIErr.Type),
			HTTPStatus: status,
			Retryable:  retryableProviderError(status, code),
			Message:    "model provider request failed",
		}
	}
	if errors.As(err, &requestErr) {
		status := safeHTTPStatus(requestErr.HTTPStatusCode)
		return &ErrorInfo{
			Category:   providerErrorCategory(status),
			Code:       "provider_request_error",
			HTTPStatus: status,
			Retryable:  status == 0 || retryableHTTPStatus(status),
			Message:    "model provider request failed",
		}
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) {
		category := "network"
		code := "network_error"
		if networkErr.Timeout() {
			category = "timeout"
			code = "network_timeout"
		}
		return &ErrorInfo{
			Category:  category,
			Code:      code,
			Retryable: true,
			Message:   "network request failed",
		}
	}
	return &ErrorInfo{
		Category: "execution",
		Code:     "execution_error",
		Message:  "execution failed",
	}
}

func safeHTTPStatus(status int) int {
	if status < 100 || status > 599 {
		return 0
	}
	return status
}

func providerErrorCategory(status int) string {
	switch {
	case status == 401 || status == 403:
		return "provider_auth"
	case status == 429:
		return "provider_rate_limit"
	case status == 408:
		return "timeout"
	case status >= 500:
		return "provider_unavailable"
	case status >= 400:
		return "provider_request"
	default:
		return "provider"
	}
}

func retryableHTTPStatus(status int) bool {
	return status == 408 || status == 409 || status == 429 || status >= 500
}

func retryableProviderError(status int, code string) bool {
	switch code {
	case "insufficient_quota", "invalid_api_key", "authentication_error", "permission_denied",
		"invalid_request_error", "model_not_found", "context_length_exceeded", "content_filter":
		return false
	default:
		return retryableHTTPStatus(status)
	}
}

func providerErrorCode(status int) string {
	switch {
	case status == 401 || status == 403:
		return "provider_auth"
	case status == 429:
		return "provider_rate_limit"
	case status == 408:
		return "provider_timeout"
	case status >= 500:
		return "provider_unavailable"
	case status >= 400:
		return "provider_request"
	default:
		return "provider_error"
	}
}

func safeProviderErrorCode(value any, status int) string {
	text, ok := value.(string)
	if !ok {
		return providerErrorCode(status)
	}
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "invalid_api_key", "authentication_error", "permission_denied", "insufficient_quota",
		"rate_limit_exceeded", "rate_limit", "model_overloaded", "overloaded", "overloaded_error",
		"server_error", "service_unavailable", "temporarily_unavailable", "invalid_request_error",
		"model_not_found", "context_length_exceeded", "content_filter":
		return strings.ToLower(strings.TrimSpace(text))
	default:
		return providerErrorCode(status)
	}
}

func safeProviderErrorType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "authentication_error", "permission_error", "rate_limit_error", "overloaded_error",
		"server_error", "api_error", "invalid_request_error":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func panicError(recovered any) *ErrorInfo {
	return &ErrorInfo{
		Category: "panic",
		Code:     "panic",
		Message:  "execution panicked",
	}
}

func safeFinishReason(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "stop", "length", "tool_calls", "function_call", "content_filter", "end_turn", "max_tokens", "error", "canceled", "cancelled":
		return strings.ToLower(strings.TrimSpace(value))
	case "":
		return ""
	default:
		return "other"
	}
}

func safeToolIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return "[invalid]"
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '-' || char == '.' {
			continue
		}
		return "[invalid]"
	}
	return value
}

func safeProviderIdentifier(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "openai", "openai_compatible":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "[invalid]"
	}
}

func safeModelIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 || strings.Contains(value, "://") {
		return "[invalid]"
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') ||
			char == '_' || char == '-' || char == '.' || char == ':' || char == '/' {
			continue
		}
		return "[invalid]"
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{
		"sk-", "sk_", "pk-", "pk_", "eyj", "ghp_", "github_pat_", "xoxb-", "xoxp-", "akia",
		"api_key", "api-key", "apikey", "secret", "password", "passwd", "credential", "bearer", "token",
	} {
		if strings.Contains(lower, marker) {
			return "[invalid]"
		}
	}
	return value
}
