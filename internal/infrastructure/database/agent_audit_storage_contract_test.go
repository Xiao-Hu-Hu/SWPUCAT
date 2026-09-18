package database

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAgentAuditJSONModelsPreserveLexicalPayload(t *testing.T) {
	tests := []struct {
		model any
		field string
	}{
		{model: AgentRunModel{}, field: "UsageJSON"},
		{model: AgentRunModel{}, field: "ModelCallsJSON"},
		{model: AgentToolCallModel{}, field: "ArgumentsJSON"},
		{model: AgentToolCallModel{}, field: "ResultJSON"},
	}

	for _, test := range tests {
		t.Run(reflect.TypeOf(test.model).Name()+"/"+test.field, func(t *testing.T) {
			field, ok := reflect.TypeOf(test.model).FieldByName(test.field)
			if !ok {
				t.Fatalf("model field %s is missing", test.field)
			}
			tag := field.Tag.Get("gorm")
			if !strings.Contains(tag, "type:json") || strings.Contains(tag, "type:jsonb") {
				t.Fatalf("gorm tag %q does not select lexically preserving PostgreSQL json", tag)
			}
		})
	}
}

func TestAgentAuditMigrationPreservesCapturedJSONBytes(t *testing.T) {
	migrationPath := filepath.Join("..", "..", "..", "migrations", "035_create_agent_audits.sql")
	contents, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(contents)

	for _, fragment := range []string{
		"usage_json JSON,",
		"model_calls_json JSON,",
		"arguments_json JSON NOT NULL",
		"result_json JSON NOT NULL",
		"arguments_captured_bytes = octet_length(arguments_json::text)",
		"result_captured_bytes = octet_length(result_json::text)",
		"json_typeof(usage_json)",
		"json_array_length(model_calls_json)",
		"FLOOR(EXTRACT(EPOCH FROM (finished_at - started_at)) * 1000)::BIGINT",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("migration is missing storage contract %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"usage_json JSONB",
		"model_calls_json JSONB",
		"arguments_json JSONB",
		"result_json JSONB",
		"jsonb_typeof(",
		"jsonb_array_length(",
	} {
		if strings.Contains(sql, forbidden) {
			t.Errorf("migration still contains normalizing JSONB contract %q", forbidden)
		}
	}
}
