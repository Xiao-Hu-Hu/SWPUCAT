package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentTurnMigrationEnforcesIdempotencyAndFeedbackOwnershipShape(t *testing.T) {
	path := filepath.Join("..", "..", "..", "migrations", "037_add_agent_turn_idempotency_and_feedback.sql")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"client_turn_id VARCHAR(32)",
		"generation_id VARCHAR(32)",
		"request_id VARCHAR(32)",
		"uq_agent_messages_user_turn",
		"uq_agent_messages_assistant_turn",
		"CREATE TABLE IF NOT EXISTS agent_response_feedback",
		"REFERENCES agent_messages(id) ON DELETE CASCADE",
		"UNIQUE (user_id, message_id)",
		"request_id ~ '^[0-9a-f]{32}$'",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("migration is missing %q", fragment)
		}
	}
}
