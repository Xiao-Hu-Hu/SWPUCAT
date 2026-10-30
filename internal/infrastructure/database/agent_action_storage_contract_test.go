package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentActionMigrationStoresOneWayClaimsAndStructuredMessageActions(t *testing.T) {
	path := filepath.Join("..", "..", "..", "migrations", "038_add_agent_action_confirmations.sql")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"actions_json JSONB NOT NULL DEFAULT '[]'::jsonb",
		"jsonb_typeof(actions_json) = 'array'",
		"CREATE TABLE IF NOT EXISTS agent_action_confirmations",
		"token_hash CHAR(64) PRIMARY KEY",
		"parameters_digest CHAR(64) NOT NULL",
		"status IN ('executing', 'succeeded', 'failed')",
		"status = 'executing' AND completed_at IS NULL",
		"status IN ('succeeded', 'failed') AND completed_at IS NOT NULL",
		"REFERENCES users(id) ON DELETE CASCADE",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("migration is missing %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"confirmation_token varchar",
		"raw_token varchar",
		"authorization varchar",
		"access_token varchar",
	} {
		if strings.Contains(strings.ToLower(sql), forbidden) {
			t.Errorf("migration must not persist sensitive field %q", forbidden)
		}
	}
}
