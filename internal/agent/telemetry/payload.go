package telemetry

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type payloadKind int

const (
	payloadArguments payloadKind = iota
	payloadResult

	maxRawPayloadBytes = 1 << 20
	maxArgumentBytes   = 2 << 10
	maxResultBytes     = 4 << 10
	maxProjectionItems = 20
)

var (
	bearerPattern    = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/=-]+`)
	emailPattern     = regexp.MustCompile(`(?i)\b[a-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+\b`)
	phonePattern     = regexp.MustCompile(`\b1[3-9][0-9]{9}\b`)
	secretPattern    = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|refresh[_-]?token|password|passwd|secret|credential|signature)\s*[:=]\s*[^\s,;]+`)
	periodKeyPattern = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)
)

func captureToolPayload(toolName string, kind payloadKind, raw string) PayloadCapture {
	originalBytes := len(raw)
	if originalBytes > maxRawPayloadBytes {
		return marshalCapture(map[string]any{
			"kind":  "oversize",
			"bytes": originalBytes,
		}, originalBytes, true, true, payloadLimit(kind))
	}
	if strings.TrimSpace(raw) == "" {
		value := any(nil)
		if kind == payloadArguments {
			value = map[string]any{}
		}
		return marshalCapture(value, originalBytes, false, false, payloadLimit(kind))
	}
	decoded, ok := decodeJSON(raw)
	if !ok {
		return marshalCapture(map[string]any{
			"kind":  "opaque",
			"bytes": originalBytes,
		}, originalBytes, true, false, payloadLimit(kind))
	}
	var projected any
	if kind == payloadArguments {
		projected = projectArguments(toolName, decoded)
	} else {
		projected = projectResult(toolName, decoded)
	}
	projected = sanitizeProjection(projected)
	return marshalCapture(projected, originalBytes, true, false, payloadLimit(kind))
}

func payloadLimit(kind payloadKind) int {
	if kind == payloadArguments {
		return maxArgumentBytes
	}
	return maxResultBytes
}

func decodeJSON(raw string) (any, bool) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, false
	}
	return value, true
}

func marshalCapture(value any, originalBytes int, redacted, truncated bool, limit int) PayloadCapture {
	data, err := json.Marshal(value)
	if err != nil {
		data = []byte(`{"kind":"unavailable"}`)
		redacted = true
	}
	if len(data) > limit {
		truncated = true
		previewBudget := limit / 2
		if previewBudget < 64 {
			previewBudget = 64
		}
		preview := truncateUTF8Bytes(string(data), previewBudget)
		for {
			wrapped, marshalErr := json.Marshal(map[string]any{
				"kind":           "bounded_projection",
				"original_bytes": originalBytes,
				"preview":        preview,
			})
			if marshalErr == nil && len(wrapped) <= limit {
				data = wrapped
				break
			}
			if preview == "" {
				data = []byte(`{"kind":"bounded_projection"}`)
				break
			}
			preview = truncateUTF8Bytes(preview, len(preview)/2)
		}
	}
	return PayloadCapture{
		JSON:          string(data),
		OriginalBytes: originalBytes,
		CapturedBytes: len(data),
		Redacted:      redacted,
		Truncated:     truncated,
	}
}

func projectArguments(toolName string, decoded any) any {
	object, _ := decoded.(map[string]any)
	switch toolName {
	case "get_checkin_status", "get_my_work_hours", "get_checkin_requirements", "get_online_summary", "clock_in", "clock_out":
		return map[string]any{}
	case "get_my_checkin_history":
		result := make(map[string]any)
		projectArgumentNumber(result, object, "limit", 0, 100, true)
		return result
	case "search_knowledge":
		return map[string]any{"query_length": trimmedStringLength(object["query"])}
	case "recommend_knowledge":
		return map[string]any{"need_length": trimmedStringLength(object["need"])}
	case "query_team_members":
		result := make(map[string]any)
		projectArgumentPresence(result, object, "period_id")
		projectArgumentNumber(result, object, "min_hours", 0, 10000, false)
		projectArgumentNumber(result, object, "limit", 0, 200, true)
		projectArgumentEnum(result, object, "period", "today", "week", "month")
		projectArgumentEnum(result, object, "report_status", "missing", "draft", "submitted", "reviewed")
		return result
	case "get_checkin_stats":
		result := make(map[string]any)
		projectArgumentEnum(result, object, "period", "today", "week", "month")
		return result
	case "list_report_periods":
		result := make(map[string]any)
		projectArgumentNumber(result, object, "limit", 0, 24, true)
		projectArgumentPeriodKey(result, object, "period_key")
		projectArgumentEnum(result, object, "status", "open", "closed", "archived")
		return result
	case "get_report_materials":
		result := make(map[string]any)
		projectArgumentPresence(result, object, "period_id")
		projectArgumentNumber(result, object, "offset", 0, 1000000, true)
		projectArgumentNumber(result, object, "limit", 0, 8, true)
		projectArgumentEnum(result, object, "scope", "mine", "team")
		return result
	case "draft_announcement":
		return map[string]any{
			"title_length":       trimmedStringLength(object["title"]),
			"content_length":     trimmedStringLength(object["content"]),
			"source_note_length": trimmedStringLength(object["source_note"]),
			"pinned":             argumentBool(object, "pinned"),
			"recipient_count":    arrayLength(object["recipient_ids"]),
		}
	case "list_pending_knowledge":
		result := make(map[string]any)
		projectArgumentNumber(result, object, "limit", 0, 50, true)
		return result
	case "review_knowledge_item":
		result := map[string]any{"reason_length": trimmedStringLength(object["reason"])}
		projectArgumentPresence(result, object, "item_id")
		projectArgumentEnum(result, object, "decision", "approve", "reject")
		return result
	case "generate_invitation_code":
		result := make(map[string]any)
		projectArgumentEnum(result, object, "type", "member", "captain")
		return result
	case "save_evaluation_draft":
		result := map[string]any{
			"title_length":   trimmedStringLength(object["title"]),
			"summary_length": trimmedStringLength(object["summary_markdown"]),
		}
		projectArgumentEnum(result, object, "mode", "create", "update")
		projectArgumentPresence(result, object, "draft_id")
		projectArgumentPresence(result, object, "snapshot_id")
		projectArgumentEnum(result, object, "audience", "captain", "team", "private")
		return result
	default:
		return payloadShape(decoded, 0)
	}
}

func projectArgumentPresence(result map[string]any, object map[string]any, field string) {
	if _, ok := object[field]; ok {
		result[field+"_provided"] = true
	}
}

func projectArgumentNumber(result map[string]any, object map[string]any, field string, minimum, maximum float64, integer bool) {
	value, ok := object[field]
	if !ok {
		return
	}
	normalized, number, ok := normalizeNumber(value, integer)
	if !ok || number < minimum || number > maximum {
		result[field] = "invalid"
		return
	}
	result[field] = normalized
}

func normalizeNumber(value any, integer bool) (any, float64, bool) {
	if integer {
		var normalized int64
		switch typed := value.(type) {
		case json.Number:
			parsed, err := typed.Int64()
			if err != nil {
				return nil, 0, false
			}
			normalized = parsed
		case float64:
			if math.IsNaN(typed) || math.IsInf(typed, 0) || math.Trunc(typed) != typed || typed < math.MinInt64 || typed > math.MaxInt64 {
				return nil, 0, false
			}
			normalized = int64(typed)
		default:
			return nil, 0, false
		}
		return normalized, float64(normalized), true
	}

	var number float64
	var err error
	switch typed := value.(type) {
	case json.Number:
		number, err = typed.Float64()
	case float64:
		number = typed
	default:
		return nil, 0, false
	}
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return nil, 0, false
	}
	return number, number, true
}

func projectArgumentEnum(result map[string]any, object map[string]any, field string, allowed ...string) {
	value, ok := object[field]
	if !ok {
		return
	}
	text, ok := value.(string)
	if !ok {
		result[field] = "invalid"
		return
	}
	text = strings.TrimSpace(text)
	for _, candidate := range allowed {
		if text == candidate || text == "" {
			result[field] = text
			return
		}
	}
	result[field] = "invalid"
}

func projectArgumentPeriodKey(result map[string]any, object map[string]any, field string) {
	value, ok := object[field]
	if !ok {
		return
	}
	text, ok := value.(string)
	text = strings.TrimSpace(text)
	if ok && (text == "" || periodKeyPattern.MatchString(text)) {
		result[field] = text
		return
	}
	result[field] = "invalid"
}

func trimmedStringLength(value any) int {
	text, _ := value.(string)
	return utf8.RuneCountInString(strings.TrimSpace(text))
}

func argumentBool(object map[string]any, field string) any {
	value, ok := object[field]
	if !ok {
		return nil
	}
	if boolean, ok := value.(bool); ok {
		return boolean
	}
	return "invalid"
}

func projectResult(toolName string, decoded any) any {
	object, _ := decoded.(map[string]any)
	switch toolName {
	case "get_checkin_status":
		return map[string]any{"status": enumOrOther(object["status"], "idle", "clocked_in", "clocked_out")}
	case "clock_in", "clock_out", "review_knowledge_item", "generate_invitation_code", "save_evaluation_draft":
		if object["status"] == "confirmation_required" {
			return projectActionProposal(object)
		}
		if toolName != "clock_in" && toolName != "clock_out" {
			return payloadShape(decoded, 0)
		}
		return map[string]any{
			"type": enumOrOther(object["type"], "in", "out"),
			"date": dateOrOther(object["date"]),
		}
	case "get_my_work_hours":
		return projectWorkHours(object)
	case "get_my_checkin_history":
		return map[string]any{
			"limit":        numberOrNil(object["limit"]),
			"record_count": arrayLength(object["records"]),
			"type_counts":  countEnumField(object["records"], "type", "in", "out"),
		}
	case "get_checkin_requirements":
		return map[string]any{"requirements": projectRequirements(object["requirements"])}
	case "search_knowledge":
		return map[string]any{
			"source_count":    arrayLength(object["sources"]),
			"source_ids":      collectNumberField(object["sources"], "id"),
			"match_modes":     collectUniqueEnums(object["sources"], "match_mode", "keyword", "vector", "hybrid"),
			"truncated":       boolOrNil(object["truncated"]),
			"omitted_sources": numberOrNil(object["omitted_sources"]),
		}
	case "recommend_knowledge":
		return map[string]any{
			"recommendation_count": arrayLength(decoded),
			"item_ids":             collectNumberField(decoded, "item_id"),
		}
	case "query_team_members":
		return map[string]any{
			"period":          enumOrOther(object["period"], "today", "week", "month"),
			"period_id":       numberOrNil(object["period_id"]),
			"report_status":   enumOrOther(object["report_status"], "", "missing", "draft", "submitted", "reviewed", "archived"),
			"limit":           numberOrNil(object["limit"]),
			"member_count":    arrayLength(object["members"]),
			"report_states":   countEnumField(object["members"], "report_state", "missing", "draft", "submitted", "reviewed", "archived"),
			"attendance":      countEnumField(object["members"], "attendance_state", "absent", "normal", "attention", "unknown", "anomaly"),
			"anomaly_members": countNonEmptyArrayField(object["members"], "anomalies"),
		}
	case "get_checkin_stats":
		return map[string]any{
			"member_count":    arrayLength(decoded),
			"status_counts":   countEnumField(decoded, "status", "absent", "normal", "attention", "unknown", "anomaly"),
			"anomaly_members": countNonEmptyArrayField(decoded, "anomalies"),
		}
	case "list_report_periods":
		return map[string]any{
			"period_key":    periodKeyOrOther(object["period_key"]),
			"status":        enumOrOther(object["status"], "", "open", "closed", "archived"),
			"total_periods": numberOrNil(object["total_periods"]),
			"truncated":     boolOrNil(object["truncated"]),
			"periods":       projectReportPeriods(object["periods"]),
		}
	case "get_report_materials":
		return map[string]any{
			"period_id":       numberOrNil(object["period_id"]),
			"scope":           enumOrOther(object["scope"], "mine", "team"),
			"offset":          numberOrNil(object["offset"]),
			"limit":           numberOrNil(object["limit"]),
			"total_reports":   numberOrNil(object["total_reports"]),
			"next_offset":     numberOrNil(object["next_offset"]),
			"skipped_reports": numberOrNil(object["skipped_reports"]),
			"materials":       projectReportMaterials(object["materials"]),
		}
	case "draft_announcement":
		if object["status"] == "confirmation_required" {
			return projectActionProposal(object)
		}
		return map[string]any{
			"id":              numberOrNil(object["id"]),
			"status":          enumOrOther(object["status"], "draft", "confirmed", "published", "discarded"),
			"pinned":          boolOrNil(object["pinned"]),
			"recipient_count": arrayLength(object["recipient_ids"]),
			"email_requested": boolOrNil(object["email_requested"]),
			"announcement_id": numberOrNil(object["announcement_id"]),
		}
	case "list_pending_knowledge":
		return map[string]any{
			"total":      numberOrNil(object["total"]),
			"limit":      numberOrNil(object["limit"]),
			"item_count": arrayLength(object["items"]),
			"truncated":  boolOrNil(object["truncated"]),
		}
	case "get_online_summary":
		return map[string]any{
			"as_of_date":    dateOrOther(object["as_of_date"]),
			"total_members": numberOrNil(object["total_members"]),
			"online_count":  numberOrNil(object["online_count"]),
			"self_online":   boolOrNil(object["self_online"]),
			"name_count":    arrayLength(object["online_members"]),
			"truncated":     boolOrNil(object["truncated"]),
		}
	default:
		return payloadShape(decoded, 0)
	}
}

func projectActionProposal(object map[string]any) map[string]any {
	return map[string]any{
		"status": enumOrOther(object["status"], "confirmation_required"),
		"tool": enumOrOther(object["tool"], "clock_in", "clock_out", "draft_announcement", "review_knowledge_item",
			"generate_invitation_code", "save_evaluation_draft"),
	}
}

func projectWorkHours(object map[string]any) any {
	return map[string]any{
		"as_of_date":         dateOrOther(object["as_of_date"]),
		"week":               projectPeriodStats(object["week"]),
		"month":              projectPeriodStats(object["month"]),
		"weekly_requirement": projectWeeklyRequirement(object["weekly_requirement"]),
	}
}

func projectPeriodStats(value any) map[string]any {
	object, _ := value.(map[string]any)
	return map[string]any{
		"starts_on":        dateOrOther(object["starts_on"]),
		"ends_on":          dateOrOther(object["ends_on"]),
		"total_minutes":    numberOrNil(object["total_minutes"]),
		"total_hours":      numberOrNil(object["total_hours"]),
		"record_days":      numberOrNil(object["record_days"]),
		"attendance_state": enumOrOther(object["attendance_state"], "absent", "normal", "attention", "unknown", "anomaly"),
	}
}

func projectWeeklyRequirement(value any) map[string]any {
	object, _ := value.(map[string]any)
	return map[string]any{
		"grade":             numberOrNil(object["grade"]),
		"required_minutes":  numberOrNil(object["required_minutes"]),
		"required_hours":    numberOrNil(object["required_hours"]),
		"is_met":            boolOrNil(object["is_met"]),
		"shortfall_minutes": numberOrNil(object["shortfall_minutes"]),
		"shortfall_hours":   numberOrNil(object["shortfall_hours"]),
		"status":            enumOrOther(object["status"], "unknown", "below_target", "met"),
	}
}

func projectRequirements(value any) []any {
	items, _ := value.([]any)
	if len(items) > maxProjectionItems {
		items = items[:maxProjectionItems]
	}
	result := make([]any, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		result = append(result, map[string]any{
			"grade":   numberOrNil(object["grade"]),
			"minutes": numberOrNil(object["minutes"]),
		})
	}
	return result
}

func projectReportPeriods(value any) []any {
	items, _ := value.([]any)
	if len(items) > maxProjectionItems {
		items = items[:maxProjectionItems]
	}
	result := make([]any, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		result = append(result, map[string]any{
			"id":           numberOrNil(object["id"]),
			"period_key":   periodKeyOrOther(object["period_key"]),
			"status":       enumOrOther(object["status"], "open", "closed", "archived"),
			"member_count": numberOrNil(object["member_count"]),
		})
	}
	return result
}

func projectReportMaterials(value any) []any {
	items, _ := value.([]any)
	if len(items) > maxProjectionItems {
		items = items[:maxProjectionItems]
	}
	result := make([]any, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		result = append(result, map[string]any{
			"report_id":    numberOrNil(object["report_id"]),
			"version":      numberOrNil(object["version"]),
			"status":       enumOrOther(object["status"], "draft", "submitted", "reviewed", "rejected", "invalidated", "archived"),
			"parse_status": enumOrOther(object["parse_status"], "pending", "processing", "ready", "failed", "parsed"),
			"truncated":    boolOrNil(object["truncated"]),
		})
	}
	return result
}

func collectNumberField(value any, field string) []any {
	items, _ := value.([]any)
	if len(items) > maxProjectionItems {
		items = items[:maxProjectionItems]
	}
	result := make([]any, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if number := numberOrNil(object[field]); number != nil {
			result = append(result, number)
		}
	}
	return result
}

func collectUniqueEnums(value any, field string, allowed ...string) []string {
	items, _ := value.([]any)
	if len(items) > maxProjectionItems {
		items = items[:maxProjectionItems]
	}
	seen := make(map[string]struct{})
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		text, ok := enumValue(object[field], allowed...)
		if !ok || text == "" {
			continue
		}
		seen[text] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for item := range seen {
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func countEnumField(value any, field string, allowed ...string) map[string]int {
	items, _ := value.([]any)
	if len(items) > maxProjectionItems {
		items = items[:maxProjectionItems]
	}
	result := make(map[string]int)
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		text, ok := enumValue(object[field], allowed...)
		if !ok || text == "" {
			if _, exists := object[field]; exists {
				result["other"]++
			}
			continue
		}
		result[text]++
	}
	return result
}

func countNonEmptyArrayField(value any, field string) int {
	items, _ := value.([]any)
	if len(items) > maxProjectionItems {
		items = items[:maxProjectionItems]
	}
	count := 0
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if values, ok := object[field].([]any); ok && len(values) > 0 {
			count++
		}
	}
	return count
}

func numberOrNil(value any) any {
	if number, _, ok := normalizeNumber(value, true); ok {
		return number
	}
	if number, _, ok := normalizeNumber(value, false); ok {
		return number
	}
	return nil
}

func boolOrNil(value any) any {
	if boolean, ok := value.(bool); ok {
		return boolean
	}
	return nil
}

func enumValue(value any, allowed ...string) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	for _, candidate := range allowed {
		if text == candidate {
			return text, true
		}
	}
	return "", false
}

func enumOrOther(value any, allowed ...string) any {
	if value == nil {
		return nil
	}
	if text, ok := enumValue(value, allowed...); ok {
		return text
	}
	return "other"
}

func dateOrOther(value any) any {
	text, ok := value.(string)
	if ok {
		if _, err := time.Parse("2006-01-02", text); err == nil {
			return text
		}
	}
	if value == nil {
		return nil
	}
	return "other"
}

func periodKeyOrOther(value any) any {
	text, ok := value.(string)
	if ok && (text == "" || periodKeyPattern.MatchString(text)) {
		return text
	}
	if value == nil {
		return nil
	}
	return "other"
}

func stringLength(value any) int {
	text, _ := value.(string)
	return utf8.RuneCountInString(text)
}

func arrayLength(value any) int {
	items, _ := value.([]any)
	return len(items)
}

func payloadShape(value any, depth int) any {
	if depth >= 6 {
		return map[string]any{"type": "depth_limited"}
	}
	switch typed := value.(type) {
	case map[string]any:
		return map[string]any{"type": "object", "key_count": len(typed), "truncated": len(typed) > 32}
	case []any:
		itemTypes := make(map[string]struct{})
		limit := len(typed)
		if limit > maxProjectionItems {
			limit = maxProjectionItems
		}
		for _, item := range typed[:limit] {
			itemTypes[shapeType(item)] = struct{}{}
		}
		types := make([]string, 0, len(itemTypes))
		for itemType := range itemTypes {
			types = append(types, itemType)
		}
		sort.Strings(types)
		return map[string]any{"type": "array", "count": len(typed), "item_types": types}
	default:
		return map[string]any{"type": shapeType(typed)}
	}
}

func shapeType(value any) string {
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

func sanitizeProjection(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if isSensitiveKey(key) {
				result[key] = "[REDACTED]"
				continue
			}
			result[key] = sanitizeProjection(item)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i := range typed {
			result[i] = sanitizeProjection(typed[i])
		}
		return result
	case []string:
		result := make([]string, len(typed))
		for i := range typed {
			result[i] = truncateRunes(sanitizeFreeText(typed[i]), 128)
		}
		return result
	case string:
		return truncateRunes(sanitizeFreeText(typed), 256)
	default:
		return typed
	}
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(key)
	normalized = strings.NewReplacer("_", "", "-", "", ".", "", " ", "").Replace(normalized)
	for _, sensitive := range []string{
		"authorization", "cookie", "setcookie", "token", "accesstoken", "refreshtoken",
		"apikey", "secret", "password", "passwd", "passphrase", "credential", "signature", "privatekey",
		"email", "phone", "studentid", "filekey",
	} {
		if normalized == sensitive || strings.Contains(normalized, sensitive) {
			return true
		}
	}
	return false
}

func sanitizeFreeText(value string) string {
	value = strings.ToValidUTF8(value, "�")
	value = bearerPattern.ReplaceAllString(value, "Bearer [REDACTED]")
	value = emailPattern.ReplaceAllString(value, "[REDACTED_EMAIL]")
	value = phonePattern.ReplaceAllString(value, "[REDACTED_PHONE]")
	value = secretPattern.ReplaceAllString(value, "$1=[REDACTED]")
	return value
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit == 1 {
		return "…"
	}
	return string(runes[:limit-1]) + "…"
}

func truncateUTF8Bytes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	data := []byte(value)
	if len(data) <= limit {
		return value
	}
	data = data[:limit]
	for len(data) > 0 && !utf8.Valid(data) {
		data = data[:len(data)-1]
	}
	return string(bytes.TrimSpace(data))
}
