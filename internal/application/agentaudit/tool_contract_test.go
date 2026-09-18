package agentauditapp

import (
	"context"
	"strings"
	"testing"
	"time"

	"SWPUCAT/internal/agent/telemetry"
	"SWPUCAT/internal/domain/agentaudit"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
)

func TestKnownToolTelemetryProjectionsSatisfyPersistenceContract(t *testing.T) {
	tests := []struct {
		name          string
		arguments     string
		result        string
		wantArguments string
		wantResult    string
	}{
		{
			name:          "get_checkin_status",
			arguments:     `{"private_note":"SECRET_CHECKIN_STATUS_ARGUMENT_913"}`,
			result:        `{"status":"clocked_in","raw_secret":"SECRET_CHECKIN_STATUS_RESULT_913"}`,
			wantArguments: `{}`,
			wantResult:    `{"status":"clocked_in"}`,
		},
		{
			name:      "get_my_work_hours",
			arguments: `{"private_note":"SECRET_WORK_HOURS_ARGUMENT_913"}`,
			result: `{
				"as_of_date":"2026-09-18",
				"week":{"starts_on":"2026-09-14","ends_on":"2026-09-20","total_minutes":600,"total_hours":10,"record_days":5,"attendance_state":"normal","raw_secret":"SECRET_WEEK_913"},
				"month":{"starts_on":"2026-09-01","ends_on":"2026-09-30","total_minutes":1470,"total_hours":24.5,"record_days":12,"attendance_state":"attention","raw_secret":"SECRET_MONTH_913"},
				"weekly_requirement":{"grade":2,"required_minutes":480,"required_hours":8,"is_met":true,"shortfall_minutes":0,"shortfall_hours":0,"status":"met","raw_secret":"SECRET_REQUIREMENT_913"},
				"nickname":"SECRET_WORK_HOURS_RESULT_913"
			}`,
			wantArguments: `{}`,
			wantResult:    `{"as_of_date":"2026-09-18","month":{"attendance_state":"attention","ends_on":"2026-09-30","record_days":12,"starts_on":"2026-09-01","total_hours":24.5,"total_minutes":1470},"week":{"attendance_state":"normal","ends_on":"2026-09-20","record_days":5,"starts_on":"2026-09-14","total_hours":10,"total_minutes":600},"weekly_requirement":{"grade":2,"is_met":true,"required_hours":8,"required_minutes":480,"shortfall_hours":0,"shortfall_minutes":0,"status":"met"}}`,
		},
		{
			name:          "get_checkin_requirements",
			arguments:     `{"private_note":"SECRET_REQUIREMENTS_ARGUMENT_913"}`,
			result:        `{"requirements":[{"grade":1,"minutes":480,"raw_secret":"SECRET_REQUIREMENT_ONE_913"},{"grade":2,"minutes":360,"raw_secret":"SECRET_REQUIREMENT_TWO_913"}],"raw_secret":"SECRET_REQUIREMENTS_RESULT_913"}`,
			wantArguments: `{}`,
			wantResult:    `{"requirements":[{"grade":1,"minutes":480},{"grade":2,"minutes":360}]}`,
		},
		{
			name:          "clock_in",
			arguments:     `{"private_note":"SECRET_CLOCK_IN_ARGUMENT_913"}`,
			result:        `{"id":91,"type":"in","date":"2026-09-18","time":"08:30:00","raw_secret":"SECRET_CLOCK_IN_RESULT_913"}`,
			wantArguments: `{}`,
			wantResult:    `{"date":"2026-09-18","type":"in"}`,
		},
		{
			name:          "clock_out",
			arguments:     `{"private_note":"SECRET_CLOCK_OUT_ARGUMENT_913"}`,
			result:        `{"id":92,"type":"out","date":"2026-09-18","time":"17:30:00","raw_secret":"SECRET_CLOCK_OUT_RESULT_913"}`,
			wantArguments: `{}`,
			wantResult:    `{"date":"2026-09-18","type":"out"}`,
		},
		{
			name:          "get_my_checkin_history",
			arguments:     `{"limit":2,"private_note":"SECRET_HISTORY_ARGUMENT_913"}`,
			result:        `{"limit":2,"records":[{"type":"in","raw_secret":"SECRET_HISTORY_IN_913"},{"type":"out","raw_secret":"SECRET_HISTORY_OUT_913"},{"type":"SECRET_HISTORY_TYPE_913"}],"raw_secret":"SECRET_HISTORY_RESULT_913"}`,
			wantArguments: `{"limit":2}`,
			wantResult:    `{"limit":2,"record_count":3,"type_counts":{"in":1,"other":1,"out":1}}`,
		},
		{
			name:      "search_knowledge",
			arguments: `{"query":"SECRET_QUERY_913","private_note":"SECRET_SEARCH_ARGUMENT_913"}`,
			result: `{
				"sources":[
					{"id":12,"match_mode":"keyword","name":"SECRET_SOURCE_ONE_913"},
					{"id":13,"match_mode":"vector","description":"SECRET_SOURCE_TWO_913"},
					{"id":14,"match_mode":"hybrid","excerpt":"SECRET_SOURCE_THREE_913"}
				],
				"truncated":true,
				"omitted_sources":1,
				"raw_secret":"SECRET_SEARCH_RESULT_913"
			}`,
			wantArguments: `{"query_length":16}`,
			wantResult:    `{"match_modes":["hybrid","keyword","vector"],"omitted_sources":1,"source_count":3,"source_ids":[12,13,14],"truncated":true}`,
		},
		{
			name:          "recommend_knowledge",
			arguments:     `{"need":"SECRET_NEED_913","private_note":"SECRET_RECOMMEND_ARGUMENT_913"}`,
			result:        `[{"item_id":21,"reason":"SECRET_RECOMMEND_ONE_913"},{"item_id":22,"reason":"SECRET_RECOMMEND_TWO_913"}]`,
			wantArguments: `{"need_length":15}`,
			wantResult:    `{"item_ids":[21,22],"recommendation_count":2}`,
		},
		{
			name:          "query_team_members",
			arguments:     `{"period_id":7,"min_hours":3.5,"limit":25,"period":"week","report_status":"submitted","private_note":"SECRET_TEAM_ARGUMENT_913"}`,
			result:        `{"period":"week","period_id":7,"report_status":"submitted","limit":25,"members":[{"report_state":"submitted","attendance_state":"normal","nickname":"SECRET_MEMBER_ONE_913","anomalies":[]},{"report_state":"reviewed","attendance_state":"anomaly","nickname":"SECRET_MEMBER_TWO_913","anomalies":["SECRET_ANOMALY_913"]}],"raw_secret":"SECRET_TEAM_RESULT_913"}`,
			wantArguments: `{"limit":25,"min_hours":3.5,"period":"week","period_id_provided":true,"report_status":"submitted"}`,
			wantResult:    `{"anomaly_members":1,"attendance":{"anomaly":1,"normal":1},"limit":25,"member_count":2,"period":"week","period_id":7,"report_states":{"reviewed":1,"submitted":1},"report_status":"submitted"}`,
		},
		{
			name:          "get_checkin_stats",
			arguments:     `{"period":"month","private_note":"SECRET_STATS_ARGUMENT_913"}`,
			result:        `[{"status":"absent"},{"status":"normal"},{"status":"attention"},{"status":"unknown"},{"status":"anomaly","anomalies":["SECRET_STATS_ANOMALY_913"]},{"status":"SECRET_STATS_STATE_913"}]`,
			wantArguments: `{"period":"month"}`,
			wantResult:    `{"anomaly_members":1,"member_count":6,"status_counts":{"absent":1,"anomaly":1,"attention":1,"normal":1,"other":1,"unknown":1}}`,
		},
		{
			name:          "list_report_periods",
			arguments:     `{"limit":12,"period_key":"2026-09","status":"open","private_note":"SECRET_PERIODS_ARGUMENT_913"}`,
			result:        `{"period_key":"2026-09","status":"open","total_periods":2,"truncated":false,"periods":[{"id":31,"period_key":"2026-09","status":"open","member_count":8,"raw_secret":"SECRET_PERIOD_ONE_913"},{"id":30,"period_key":"2026-08","status":"closed","member_count":7,"raw_secret":"SECRET_PERIOD_TWO_913"}],"raw_secret":"SECRET_PERIODS_RESULT_913"}`,
			wantArguments: `{"limit":12,"period_key":"2026-09","status":"open"}`,
			wantResult:    `{"period_key":"2026-09","periods":[{"id":31,"member_count":8,"period_key":"2026-09","status":"open"},{"id":30,"member_count":7,"period_key":"2026-08","status":"closed"}],"status":"open","total_periods":2,"truncated":false}`,
		},
		{
			name:          "get_report_materials",
			arguments:     `{"period_id":31,"offset":2,"limit":8,"scope":"team","private_note":"SECRET_MATERIALS_ARGUMENT_913"}`,
			result:        `{"period_id":31,"scope":"team","offset":2,"limit":8,"total_reports":10,"next_offset":10,"skipped_reports":1,"materials":[{"report_id":41,"version":3,"status":"submitted","parse_status":"parsed","truncated":false,"body_markdown":"SECRET_REPORT_ONE_913"},{"report_id":42,"version":1,"status":"reviewed","parse_status":"ready","truncated":true,"body_markdown":"SECRET_REPORT_TWO_913"}],"raw_secret":"SECRET_MATERIALS_RESULT_913"}`,
			wantArguments: `{"limit":8,"offset":2,"period_id_provided":true,"scope":"team"}`,
			wantResult:    `{"limit":8,"materials":[{"parse_status":"parsed","report_id":41,"status":"submitted","truncated":false,"version":3},{"parse_status":"ready","report_id":42,"status":"reviewed","truncated":true,"version":1}],"next_offset":10,"offset":2,"period_id":31,"scope":"team","skipped_reports":1,"total_reports":10}`,
		},
		{
			name:          "draft_announcement",
			arguments:     `{"title":"SECRET_TITLE_913","content":"SECRET_BODY_913","source_note":"SECRET_NOTE_913","pinned":true,"recipient_ids":[901,902],"private_note":"SECRET_ANNOUNCEMENT_ARGUMENT_913"}`,
			result:        `{"id":77,"status":"draft","pinned":true,"recipient_ids":[901,902],"email_requested":true,"announcement_id":101,"content":"SECRET_ANNOUNCEMENT_BODY_913","raw_secret":"SECRET_ANNOUNCEMENT_RESULT_913"}`,
			wantArguments: `{"content_length":15,"pinned":true,"recipient_count":2,"source_note_length":15,"title_length":16}`,
			wantResult:    `{"announcement_id":101,"email_requested":"[REDACTED]","id":77,"pinned":true,"recipient_count":2,"status":"draft"}`,
		},
	}

	if len(tests) != 13 {
		t.Fatalf("known tool fixture count = %d, want 13", len(tests))
	}
	seen := make(map[string]struct{}, len(tests))
	for _, test := range tests {
		if !knownAuditedTool(test.name) {
			t.Fatalf("fixture uses unknown audited tool %q", test.name)
		}
		if _, duplicate := seen[test.name]; duplicate {
			t.Fatalf("duplicate audited tool fixture %q", test.name)
		}
		seen[test.name] = struct{}{}

		t.Run(test.name, func(t *testing.T) {
			collector := telemetry.NewCollector()
			endpoint := func(_ context.Context, arguments string, _ ...tool.Option) (string, error) {
				if arguments != test.arguments {
					t.Fatalf("endpoint arguments = %q, want original payload", arguments)
				}
				return test.result, nil
			}
			wrapped, err := telemetry.NewMiddleware("openai_compatible", "contract-model").WrapInvokableToolCall(
				context.Background(),
				endpoint,
				&adk.ToolContext{Name: test.name},
			)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := wrapped(telemetry.WithCollector(context.Background(), collector), test.arguments); err != nil {
				t.Fatal(err)
			}

			snapshot := collector.Snapshot()
			if len(snapshot.ToolCalls) != 1 || snapshot.ToolCalls[0].Name != test.name || snapshot.ToolCalls[0].Status != "success" {
				t.Fatalf("unexpected telemetry tool call: %#v", snapshot.ToolCalls)
			}
			captured := snapshot.ToolCalls[0]
			if captured.Arguments.JSON != test.wantArguments {
				t.Fatalf("telemetry arguments = %s, want %s", captured.Arguments.JSON, test.wantArguments)
			}
			if captured.Result.JSON != test.wantResult {
				t.Fatalf("telemetry result = %s, want %s", captured.Result.JSON, test.wantResult)
			}

			req := validRecordRequest(agentaudit.TelemetryCaptured)
			req.StartedAt = snapshot.StartedAt.Add(-time.Second)
			req.FinishedAt = snapshot.FinishedAt.Add(time.Second)
			req.Snapshot = &snapshot
			run, err := NewService(&fakeAuditRepo{}).Record(context.Background(), req)
			if err != nil {
				t.Fatalf("persist telemetry projection: %v", err)
			}
			if err := ValidatePersistenceContract(run); err != nil {
				t.Fatalf("strict persistence contract rejected telemetry projection: %v", err)
			}
			if len(run.ToolCalls) != 1 {
				t.Fatalf("persisted tool calls = %d, want 1", len(run.ToolCalls))
			}
			persisted := run.ToolCalls[0]
			if persisted.ArgumentsJSON != captured.Arguments.JSON || persisted.ResultJSON != captured.Result.JSON {
				t.Fatalf("secondary projection is not closed: captured=(%s, %s) persisted=(%s, %s)",
					captured.Arguments.JSON, captured.Result.JSON, persisted.ArgumentsJSON, persisted.ResultJSON)
			}

			serialized := persisted.ArgumentsJSON + persisted.ResultJSON
			for _, forbidden := range []string{
				"SECRET_",
				`"private_note"`,
				`"raw_secret"`,
				`"nickname"`,
				`"body_markdown"`,
				`"content"`,
				`"source_note"`,
				`"query"`,
				`"need"`,
				`"recipient_ids"`,
				`"records"`,
				`"sources"`,
				`"members"`,
				`"anomalies"`,
			} {
				if strings.Contains(serialized, forbidden) {
					t.Fatalf("persisted payload leaked raw sensitive field/value %q: %s", forbidden, serialized)
				}
			}
		})
	}

	for _, name := range []string{
		"get_checkin_status",
		"get_my_work_hours",
		"get_checkin_requirements",
		"clock_in",
		"clock_out",
		"get_my_checkin_history",
		"search_knowledge",
		"recommend_knowledge",
		"query_team_members",
		"get_checkin_stats",
		"list_report_periods",
		"get_report_materials",
		"draft_announcement",
	} {
		if _, ok := seen[name]; !ok {
			t.Errorf("missing audited tool fixture %q", name)
		}
	}
}
