package telemetry

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestToolPayloadSafeProjections(t *testing.T) {
	tests := []struct {
		name      string
		tool      string
		kind      payloadKind
		raw       string
		forbidden []string
		required  []string
	}{
		{
			name:      "announcement arguments",
			tool:      "draft_announcement",
			kind:      payloadArguments,
			raw:       `{"title":"SECRET_TITLE_913","content":"SECRET_BODY_913","source_note":"SECRET_NOTE_913","pinned":true,"recipient_ids":[91,92]}`,
			forbidden: []string{"SECRET_TITLE_913", "SECRET_BODY_913", "SECRET_NOTE_913", "91", "92"},
			required:  []string{`"title_length":16`, `"content_length":15`, `"recipient_count":2`},
		},
		{
			name:      "report materials result",
			tool:      "get_report_materials",
			kind:      payloadResult,
			raw:       `{"period_id":7,"scope":"team","total_reports":1,"materials":[{"report_id":88,"version":3,"author_id":42,"author_name":"SECRET_MEMBER_913","title":"SECRET_TITLE_913","body_markdown":"SECRET_REPORT_BODY_913","parsed_json":{"secret":"SECRET_PARSED_913"},"status":"submitted","parse_status":"parsed"}]}`,
			forbidden: []string{"SECRET_MEMBER_913", "SECRET_TITLE_913", "SECRET_REPORT_BODY_913", "SECRET_PARSED_913", "author_id"},
			required:  []string{`"period_id":7`, `"report_id":88`, `"version":3`},
		},
		{
			name:      "team members result",
			tool:      "query_team_members",
			kind:      payloadResult,
			raw:       `{"period":"week","members":[{"user_id":9,"nickname":"SECRET_MEMBER_913","report_state":"missing","attendance_state":"anomaly","anomalies":["SECRET_ANOMALY_913"]}]}`,
			forbidden: []string{"SECRET_MEMBER_913", "SECRET_ANOMALY_913", "user_id", "nickname"},
			required:  []string{`"member_count":1`, `"missing":1`, `"anomaly_members":1`},
		},
		{
			name:      "knowledge result",
			tool:      "search_knowledge",
			kind:      payloadResult,
			raw:       `{"query":"SECRET_QUERY_913","sources":[{"id":12,"name":"SECRET_NAME_913","description":"SECRET_DESCRIPTION_913","excerpt":"SECRET_EXCERPT_913","url":"https://example.invalid/SECRET_URL_913","match_mode":"hybrid"}]}`,
			forbidden: []string{"SECRET_QUERY_913", "SECRET_NAME_913", "SECRET_DESCRIPTION_913", "SECRET_EXCERPT_913", "SECRET_URL_913"},
			required:  []string{`"source_count":1`, `"source_ids":[12]`, `"hybrid"`},
		},
		{
			name:      "personal hours result",
			tool:      "get_my_work_hours",
			kind:      payloadResult,
			raw:       `{"user_id":77,"nickname":"SECRET_MEMBER_913","as_of_date":"2026-09-17","week":{"total_minutes":600,"anomalies":["SECRET_ANOMALY_913"]},"month":{"total_minutes":1200},"weekly_requirement":{"grade":2,"required_minutes":480,"is_met":true}}`,
			forbidden: []string{"SECRET_MEMBER_913", "SECRET_ANOMALY_913", "user_id", "nickname"},
			required:  []string{`"as_of_date":"2026-09-17"`, `"total_minutes":600`, `"is_met":true`},
		},
		{
			name:      "invalid enum argument",
			tool:      "get_checkin_stats",
			kind:      payloadArguments,
			raw:       `{"period":"SECRET_PERIOD_913"}`,
			forbidden: []string{"SECRET_PERIOD_913"},
			required:  []string{`"period":"invalid"`},
		},
		{
			name:      "invalid numeric argument",
			tool:      "get_my_checkin_history",
			kind:      payloadArguments,
			raw:       `{"limit":"SECRET_LIMIT_913"}`,
			forbidden: []string{"SECRET_LIMIT_913"},
			required:  []string{`"limit":"invalid"`},
		},
		{
			name:      "numeric phone argument",
			tool:      "get_my_checkin_history",
			kind:      payloadArguments,
			raw:       `{"limit":13800138000}`,
			forbidden: []string{"13800138000"},
			required:  []string{`"limit":"invalid"`},
		},
		{
			name:      "numeric lexical smuggling argument",
			tool:      "get_my_checkin_history",
			kind:      payloadArguments,
			raw:       `{"limit":1.00000000000000000013800138000}`,
			forbidden: []string{"13800138000"},
			required:  []string{`"limit":"invalid"`},
		},
		{
			name:      "period id argument records presence only",
			tool:      "get_report_materials",
			kind:      payloadArguments,
			raw:       `{"period_id":13800138000,"offset":0,"limit":8,"scope":"team"}`,
			forbidden: []string{"13800138000"},
			required:  []string{`"period_id_provided":true`, `"offset":0`, `"limit":8`},
		},
		{
			name:      "invalid boolean argument",
			tool:      "draft_announcement",
			kind:      payloadArguments,
			raw:       `{"title":"x","content":"y","pinned":"SECRET_PINNED_913"}`,
			forbidden: []string{"SECRET_PINNED_913"},
			required:  []string{`"pinned":"invalid"`},
		},
		{
			name:      "invalid period key argument",
			tool:      "list_report_periods",
			kind:      payloadArguments,
			raw:       `{"period_key":"SECRET_PERIOD_KEY_913"}`,
			forbidden: []string{"SECRET_PERIOD_KEY_913"},
			required:  []string{`"period_key":"invalid"`},
		},
		{
			name:      "real clock in result",
			tool:      "clock_in",
			kind:      payloadResult,
			raw:       `{"id":91,"type":"in","date":"2026-09-18","time":"08:30:00"}`,
			forbidden: []string{`"id"`, `"time"`, "08:30:00"},
			required:  []string{`"type":"in"`, `"date":"2026-09-18"`},
		},
		{
			name:      "invalid status result",
			tool:      "get_checkin_status",
			kind:      payloadResult,
			raw:       `{"status":"SECRET_BODY_913"}`,
			forbidden: []string{"SECRET_BODY_913"},
			required:  []string{`"status":"other"`},
		},
		{
			name:      "invalid knowledge result enums and ids",
			tool:      "search_knowledge",
			kind:      payloadResult,
			raw:       `{"sources":[{"id":"SECRET_ID_913","match_mode":"SECRET_MODE_913"}]}`,
			forbidden: []string{"SECRET_ID_913", "SECRET_MODE_913"},
			required:  []string{`"source_ids":[]`, `"match_modes":[]`},
		},
		{
			name:      "invalid team member states",
			tool:      "query_team_members",
			kind:      payloadResult,
			raw:       `{"members":[{"report_state":"SECRET_REPORT_STATE_913","attendance_state":"SECRET_ATTENDANCE_STATE_913"}]}`,
			forbidden: []string{"SECRET_REPORT_STATE_913", "SECRET_ATTENDANCE_STATE_913"},
			required:  []string{`"report_states":{"other":1}`, `"attendance":{"other":1}`},
		},
		{
			name:      "invalid result date",
			tool:      "clock_out",
			kind:      payloadResult,
			raw:       `{"type":"out","date":"2026-99-99"}`,
			forbidden: []string{"2026-99-99"},
			required:  []string{`"date":"other"`, `"type":"out"`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capture := captureToolPayload(test.tool, test.kind, test.raw)
			if !json.Valid([]byte(capture.JSON)) {
				t.Fatalf("capture is not valid JSON: %q", capture.JSON)
			}
			for _, forbidden := range test.forbidden {
				if strings.Contains(capture.JSON, forbidden) {
					t.Fatalf("capture leaked %q: %s", forbidden, capture.JSON)
				}
			}
			for _, required := range test.required {
				if !strings.Contains(capture.JSON, required) {
					t.Fatalf("capture missing %q: %s", required, capture.JSON)
				}
			}
			if capture.OriginalBytes != len(test.raw) || capture.CapturedBytes != len(capture.JSON) || !capture.Redacted {
				t.Fatalf("unexpected capture metadata: %#v", capture)
			}
		})
	}
}

func TestToolPayloadMalformedOversizeAndUTF8(t *testing.T) {
	tests := []struct {
		name          string
		kind          payloadKind
		raw           string
		wantFragment  string
		wantTruncated bool
	}{
		{name: "empty arguments", kind: payloadArguments, raw: "  ", wantFragment: `{}`},
		{name: "empty result", kind: payloadResult, raw: "", wantFragment: `null`},
		{name: "trailing whitespace", kind: payloadArguments, raw: "{\"limit\":2}\r\n\t", wantFragment: `"limit":2`},
		{name: "multiple JSON values", kind: payloadArguments, raw: `{} {}`, wantFragment: `"opaque"`},
		{name: "trailing garbage", kind: payloadArguments, raw: `{} garbage`, wantFragment: `"opaque"`},
		{name: "truncated JSON", kind: payloadArguments, raw: `{"limit":`, wantFragment: `"opaque"`},
		{name: "invalid UTF-8", kind: payloadResult, raw: string([]byte{'{', 0xff, '}'}), wantFragment: `"opaque"`},
		{name: "oversize", kind: payloadArguments, raw: strings.Repeat("x", maxRawPayloadBytes+1), wantFragment: `"oversize"`, wantTruncated: true},
		{name: "oversize whitespace", kind: payloadArguments, raw: strings.Repeat(" ", maxRawPayloadBytes+1), wantFragment: `"oversize"`, wantTruncated: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capture := captureToolPayload("get_my_checkin_history", test.kind, test.raw)
			if !json.Valid([]byte(capture.JSON)) || !utf8.ValidString(capture.JSON) {
				t.Fatalf("capture must be valid UTF-8 JSON: %q", capture.JSON)
			}
			if !strings.Contains(capture.JSON, test.wantFragment) {
				t.Fatalf("capture %q missing %q", capture.JSON, test.wantFragment)
			}
			if capture.Truncated != test.wantTruncated {
				t.Fatalf("truncated = %v, want %v", capture.Truncated, test.wantTruncated)
			}
			if capture.CapturedBytes != len(capture.JSON) || capture.CapturedBytes > payloadLimit(test.kind) {
				t.Fatalf("capture size metadata invalid: %#v", capture)
			}
		})
	}

	if _, ok := decodeJSON(`{"ok":true} trailing`); ok {
		t.Fatal("decodeJSON accepted trailing non-whitespace data")
	}
	if _, ok := decodeJSON("{\"ok\":true}\n\t"); !ok {
		t.Fatal("decodeJSON rejected trailing whitespace")
	}

	large := map[string]any{"正文": strings.Repeat("中文内容", maxResultBytes)}
	bounded := marshalCapture(large, maxResultBytes*4, true, false, maxResultBytes)
	if !bounded.Truncated || bounded.CapturedBytes > maxResultBytes || !json.Valid([]byte(bounded.JSON)) || !utf8.ValidString(bounded.JSON) {
		t.Fatalf("bounded projection invalid: %#v", bounded)
	}

	shape := captureToolPayload("future_unknown_tool", payloadResult, `{"PRIVATE_REPORT_BODY_913":"SECRET_UNKNOWN_913","nested":{"body":"SECRET_BODY_913"}}`)
	if strings.Contains(shape.JSON, "PRIVATE_REPORT_BODY_913") || strings.Contains(shape.JSON, "SECRET_UNKNOWN_913") || strings.Contains(shape.JSON, "SECRET_BODY_913") || !strings.Contains(shape.JSON, `"type":"object"`) || strings.Contains(shape.JSON, `"keys"`) {
		t.Fatalf("unknown tool shape leaked values: %s", shape.JSON)
	}
}
