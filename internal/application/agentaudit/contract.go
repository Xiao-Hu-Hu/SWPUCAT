package agentauditapp

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/big"
	"reflect"
	"strings"

	"SWPUCAT/internal/agent/telemetry"
	"SWPUCAT/internal/domain/agentaudit"
)

// ValidatePersistenceContract is the final privacy and consistency gate for
// the JSON projections stored by AgentAuditRepo. Domain validation deliberately
// treats those fields as opaque JSON; this stricter validator proves that they
// are canonical values emitted by this package's closed projection schema.
// Repository implementations must call it even when their normal caller is
// Service so a future internal caller cannot bypass the privacy boundary.
func ValidatePersistenceContract(run *agentaudit.Run) error {
	if err := run.Validate(); err != nil {
		return err
	}

	switch run.TelemetryStatus {
	case agentaudit.TelemetryUnavailable, agentaudit.TelemetryInvalid:
		return nil
	case agentaudit.TelemetryNotStarted:
		if run.UsageJSON == nil || run.ModelCallsJSON == nil {
			return agentaudit.ErrInvalidRun
		}
		var usage usageProjection
		var models []safeModelCallProjection
		if !decodeCanonicalProjection(*run.UsageJSON, &usage) || !decodeCanonicalProjection(*run.ModelCallsJSON, &models) ||
			len(models) != 0 || !reflect.DeepEqual(usage, usageProjection{ReportedTotals: tokenProjection{}}) {
			return agentaudit.ErrInvalidRun
		}
		return nil
	case agentaudit.TelemetryCaptured:
		return validateCapturedPersistenceContract(run)
	default:
		return agentaudit.ErrInvalidRun
	}
}

func validateCapturedPersistenceContract(run *agentaudit.Run) error {
	if run.UsageJSON == nil || run.ModelCallsJSON == nil || run.AgentStartedAt == nil || run.AgentFinishedAt == nil ||
		run.ModelCallCount == nil || run.ToolCallCount == nil || run.DroppedModelCalls == nil || run.DroppedToolCalls == nil ||
		run.ReportedModelCalls == nil || run.MissingModelCalls == nil {
		return agentaudit.ErrInvalidRun
	}

	var usage usageProjection
	var models []safeModelCallProjection
	if !decodeCanonicalProjection(*run.UsageJSON, &usage) || !decodeCanonicalProjection(*run.ModelCallsJSON, &models) {
		return agentaudit.ErrInvalidRun
	}
	if len(models) > maxPersistedModelCalls || len(models)+*run.DroppedModelCalls != *run.ModelCallCount {
		return agentaudit.ErrInvalidRun
	}

	requestedTools := make(map[string]string)
	sequences := make(map[int64]struct{}, len(models)+len(run.ToolCalls))
	computedUsage, err := validatePersistedModelCalls(run, models, requestedTools, sequences)
	if err != nil || !reflect.DeepEqual(usage, computedUsage) || usage.ReportedModelCalls != *run.ReportedModelCalls || usage.MissingModelCalls != *run.MissingModelCalls {
		return agentaudit.ErrInvalidRun
	}
	if !canonicalTokensMatch(run, usage.CompleteTotals) {
		return agentaudit.ErrInvalidRun
	}
	if len(run.ToolCalls) > maxPersistedToolCalls || len(run.ToolCalls)+*run.DroppedToolCalls != *run.ToolCallCount {
		return agentaudit.ErrInvalidRun
	}
	previousToolSequence := int64(0)
	for i := range run.ToolCalls {
		if run.ToolCalls[i].Sequence <= previousToolSequence {
			return agentaudit.ErrInvalidToolCall
		}
		if err := validatePersistedToolCall(run, &run.ToolCalls[i], requestedTools, sequences); err != nil {
			return err
		}
		previousToolSequence = run.ToolCalls[i].Sequence
	}
	return nil
}

func validatePersistedModelCalls(run *agentaudit.Run, models []safeModelCallProjection, requestedTools map[string]string, sequences map[int64]struct{}) (usageProjection, error) {
	computed := usageProjection{ReportedTotals: tokenProjection{}}
	computed.MissingModelCalls = *run.DroppedModelCalls
	var totals tokenTotals
	var presence tokenPresence
	presenceInitialized := false
	previousSequence := int64(0)
	snapshot := &telemetry.Snapshot{StartedAt: *run.AgentStartedAt, FinishedAt: *run.AgentFinishedAt}

	for i := range models {
		call := &models[i]
		if call.Sequence == 0 || call.Sequence > math.MaxInt64 || int64(call.Sequence) <= previousSequence ||
			!callWithinSnapshot(snapshot, call.StartedAt, call.DurationMS) || !validCount(call.InputMessages) || !validCount(call.ToolsOffered) ||
			safeProvider(call.Provider) != call.Provider || safeModel(call.Model) != call.Model || !validModelStatus(call.Status) ||
			safeFinishReason(call.FinishReason) != call.FinishReason || safeAlias(call.ProviderResponseAlias, "response-") != call.ProviderResponseAlias ||
			len(call.ToolRequests) > maxPersistedToolRequests || !validPersistedError(call.Status, call.Error) {
			return usageProjection{}, agentaudit.ErrInvalidRun
		}
		sequence := int64(call.Sequence)
		if _, exists := sequences[sequence]; exists {
			return usageProjection{}, agentaudit.ErrInvalidRun
		}
		sequences[sequence] = struct{}{}
		previousSequence = sequence

		for _, request := range call.ToolRequests {
			if safeAlias(request.CallID, "call-") != request.CallID || safeToolName(request.Name) != request.Name {
				return usageProjection{}, agentaudit.ErrInvalidRun
			}
			if request.CallID == "" {
				continue
			}
			if existing, found := requestedTools[request.CallID]; found && existing != request.Name {
				return usageProjection{}, agentaudit.ErrInvalidRun
			}
			requestedTools[request.CallID] = request.Name
		}

		if call.Usage == nil {
			computed.MissingModelCalls++
			continue
		}
		values, currentPresence, ok := tokenTotalsFromProjection(*call.Usage)
		if !ok || tokensAreZero(values) || !addTokenTotals(&totals, values) {
			return usageProjection{}, agentaudit.ErrInvalidRun
		}
		computed.ReportedModelCalls++
		if !presenceInitialized {
			presence = currentPresence
			presenceInitialized = true
		} else {
			presence = intersectTokenPresence(presence, currentPresence)
		}
	}

	if computed.ReportedModelCalls+computed.MissingModelCalls != *run.ModelCallCount ||
		computed.ReportedModelCalls != *run.ReportedModelCalls || computed.MissingModelCalls != *run.MissingModelCalls {
		return usageProjection{}, agentaudit.ErrInvalidRun
	}
	if presenceInitialized {
		computed.ReportedTotals = projectPresentTokens(totals, presence)
	}
	if *run.ModelCallCount > 0 && computed.ReportedModelCalls > 0 && computed.MissingModelCalls == 0 {
		complete := computed.ReportedTotals
		computed.CompleteTotals = &complete
	}
	return computed, nil
}

func validatePersistedToolCall(run *agentaudit.Run, call *agentaudit.ToolCall, requestedTools map[string]string, sequences map[int64]struct{}) error {
	if safeToolName(call.Name) != call.Name || call.IsWrite != isWriteOrUnknownTool(call.Name) ||
		!callWithinSnapshot(&telemetry.Snapshot{StartedAt: *run.AgentStartedAt, FinishedAt: *run.AgentFinishedAt}, call.StartedAt, call.DurationMS) ||
		!validPersistedToolError(call) {
		return agentaudit.ErrInvalidToolCall
	}
	if call.Name == "[invalid]" && (!call.ArgumentsRedacted || !call.ResultRedacted) {
		return agentaudit.ErrInvalidToolCall
	}
	if _, exists := sequences[call.Sequence]; exists {
		return agentaudit.ErrInvalidToolCall
	}
	sequences[call.Sequence] = struct{}{}
	if call.CallID != nil {
		requestedName, found := requestedTools[*call.CallID]
		if !found || requestedName != call.Name {
			return agentaudit.ErrInvalidToolCall
		}
	}
	if !validPersistedPayload(call.Name, persistedPayloadArguments, call.ArgumentsJSON, maxArgumentProjectionBytes) ||
		!validPersistedPayload(call.Name, persistedPayloadResult, call.ResultJSON, maxResultProjectionBytes) {
		return agentaudit.ErrInvalidToolCall
	}

	expected := *call
	expected.BusinessObjectType = nil
	expected.BusinessObjectID = nil
	expected.BusinessObjectStatus = nil
	projectBusinessObject(&expected)
	if !sameStringPointer(call.BusinessObjectType, expected.BusinessObjectType) ||
		!sameInt64Pointer(call.BusinessObjectID, expected.BusinessObjectID) ||
		!sameStringPointer(call.BusinessObjectStatus, expected.BusinessObjectStatus) {
		return agentaudit.ErrInvalidToolCall
	}
	return nil
}

func validPersistedError(status string, value *safeErrorProjection) bool {
	if !validErrorForStatus(status, value) {
		return false
	}
	if value == nil {
		return true
	}
	projected, err := projectError(&telemetry.ErrorInfo{
		Category: value.Category, Code: value.Code, Type: value.Type,
		HTTPStatus: value.HTTPStatus, Retryable: value.Retryable,
	})
	return err == nil && reflect.DeepEqual(projected, value)
}

func validPersistedToolError(call *agentaudit.ToolCall) bool {
	var value *safeErrorProjection
	if call.ErrorCategory != nil || call.ErrorCode != nil || call.ErrorType != nil || call.ErrorHTTPStatus != nil || call.ErrorRetryable != nil {
		if call.ErrorCategory == nil || call.ErrorCode == nil || call.ErrorRetryable == nil {
			return false
		}
		value = &safeErrorProjection{Category: *call.ErrorCategory, Code: *call.ErrorCode, Retryable: *call.ErrorRetryable}
		if call.ErrorType != nil {
			value.Type = *call.ErrorType
		}
		if call.ErrorHTTPStatus != nil {
			value.HTTPStatus = *call.ErrorHTTPStatus
		}
	}
	return validPersistedError(call.Status, value)
}

func validPersistedPayload(toolName string, kind persistedPayloadKind, raw string, limit int) bool {
	if toolName == "[invalid]" {
		return validCanonicalPayloadShape(raw)
	}
	if !knownAuditedTool(toolName) {
		return false
	}
	projected, _, _, err := projectPayload(toolName, raw, limit, true, kind)
	return err == nil && validJSONWithoutDuplicateKeys(raw) && jsonSemanticallyEqual(raw, projected)
}

func validCanonicalPayloadShape(raw string) bool {
	var object map[string]any
	if !decodeCanonicalUntypedObject(raw, &object) {
		return false
	}
	typeName, ok := object["type"].(string)
	if !ok {
		return false
	}
	switch typeName {
	case "object":
		count, valid := jsonInt64(object["key_count"])
		truncated, boolean := object["truncated"].(bool)
		return len(object) == 3 && valid && count >= 0 && boolean && truncated == (count > 32)
	case "array":
		count, valid := jsonInt64(object["count"])
		items, array := object["item_types"].([]any)
		if len(object) != 3 || !valid || count < 0 || !array || len(items) > 6 || count == 0 && len(items) != 0 || count > 0 && (len(items) == 0 || int64(len(items)) > count) {
			return false
		}
		allowed := map[string]bool{"null": true, "object": true, "array": true, "string": true, "boolean": true, "number": true}
		previous := ""
		for _, item := range items {
			text, stringValue := item.(string)
			if !stringValue || !allowed[text] || previous != "" && text <= previous {
				return false
			}
			previous = text
		}
		return true
	case "null", "string", "boolean", "number":
		return len(object) == 1
	default:
		return false
	}
}

func tokenTotalsFromProjection(value tokenProjection) (tokenTotals, tokenPresence, bool) {
	result := tokenTotals{}
	presence := tokenPresence{}
	values := []struct {
		source  *int64
		target  *int64
		present *bool
	}{
		{value.PromptTokens, &result.PromptTokens, &presence.PromptTokens},
		{value.CompletionTokens, &result.CompletionTokens, &presence.CompletionTokens},
		{value.TotalTokens, &result.TotalTokens, &presence.TotalTokens},
		{value.CachedTokens, &result.CachedTokens, &presence.CachedTokens},
		{value.ReasoningTokens, &result.ReasoningTokens, &presence.ReasoningTokens},
	}
	for _, item := range values {
		if item.source == nil {
			continue
		}
		if *item.source <= 0 {
			return tokenTotals{}, tokenPresence{}, false
		}
		*item.target = *item.source
		*item.present = true
	}
	return result, presence, true
}

func canonicalTokensMatch(run *agentaudit.Run, complete *tokenProjection) bool {
	if complete == nil {
		return run.PromptTokens == nil && run.CompletionTokens == nil && run.TotalTokens == nil && run.CachedTokens == nil && run.ReasoningTokens == nil
	}
	return sameInt64Pointer(run.PromptTokens, complete.PromptTokens) &&
		sameInt64Pointer(run.CompletionTokens, complete.CompletionTokens) &&
		sameInt64Pointer(run.TotalTokens, complete.TotalTokens) &&
		sameInt64Pointer(run.CachedTokens, complete.CachedTokens) &&
		sameInt64Pointer(run.ReasoningTokens, complete.ReasoningTokens)
}

func decodeCanonicalProjection(raw string, target any) bool {
	if !validJSONWithoutDuplicateKeys(raw) {
		return false
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return false
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return false
	}
	canonical, err := json.Marshal(target)
	return err == nil && jsonSemanticallyEqual(raw, string(canonical))
}

func decodeCanonicalUntypedObject(raw string, target *map[string]any) bool {
	if !validJSONWithoutDuplicateKeys(raw) {
		return false
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil || *target == nil {
		return false
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return false
	}
	return true
}

func validJSONWithoutDuplicateKeys(raw string) bool {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if !walkUniqueJSONValue(decoder) {
		return false
	}
	var extra json.RawMessage
	return errors.Is(decoder.Decode(&extra), io.EOF)
}

func walkUniqueJSONValue(decoder *json.Decoder) bool {
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return true
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, keyErr := decoder.Token()
			key, stringKey := keyToken.(string)
			if keyErr != nil || !stringKey {
				return false
			}
			if _, duplicate := seen[key]; duplicate {
				return false
			}
			seen[key] = struct{}{}
			if !walkUniqueJSONValue(decoder) {
				return false
			}
		}
		closing, closeErr := decoder.Token()
		return closeErr == nil && closing == json.Delim('}')
	case '[':
		for decoder.More() {
			if !walkUniqueJSONValue(decoder) {
				return false
			}
		}
		closing, closeErr := decoder.Token()
		return closeErr == nil && closing == json.Delim(']')
	default:
		return false
	}
}

func jsonSemanticallyEqual(left, right string) bool {
	var leftValue any
	var rightValue any
	if !decodeJSONValue(left, &leftValue) || !decodeJSONValue(right, &rightValue) {
		return false
	}
	return equalJSONValue(leftValue, rightValue)
}

func decodeJSONValue(raw string, target *any) bool {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return false
	}
	var extra json.RawMessage
	return errors.Is(decoder.Decode(&extra), io.EOF)
}

func equalJSONValue(left, right any) bool {
	switch typed := left.(type) {
	case map[string]any:
		other, ok := right.(map[string]any)
		if !ok || len(typed) != len(other) {
			return false
		}
		for key, value := range typed {
			otherValue, exists := other[key]
			if !exists || !equalJSONValue(value, otherValue) {
				return false
			}
		}
		return true
	case []any:
		other, ok := right.([]any)
		if !ok || len(typed) != len(other) {
			return false
		}
		for i := range typed {
			if !equalJSONValue(typed[i], other[i]) {
				return false
			}
		}
		return true
	case json.Number:
		other, ok := right.(json.Number)
		if !ok {
			return false
		}
		leftNumber, leftOK := new(big.Rat).SetString(string(typed))
		rightNumber, rightOK := new(big.Rat).SetString(string(other))
		return leftOK && rightOK && leftNumber.Cmp(rightNumber) == 0
	default:
		return reflect.DeepEqual(left, right)
	}
}

func sameInt64Pointer(left, right *int64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func sameStringPointer(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}
