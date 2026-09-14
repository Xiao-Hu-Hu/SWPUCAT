package main

import "testing"

func TestMigrationsThroughStopsAtLegacyCutoff(t *testing.T) {
	items := []migration{
		{version: "001_create_users_table.sql"},
		{version: legacyBaselineThrough},
		{version: "016_add_user_status_and_auth_version.sql"},
	}

	result, err := migrationsThrough(items, legacyBaselineThrough)
	if err != nil {
		t.Fatalf("migrationsThrough returned an error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 legacy migrations, got %d", len(result))
	}
	if result[len(result)-1].version != legacyBaselineThrough {
		t.Fatalf("expected cutoff %q, got %q", legacyBaselineThrough, result[len(result)-1].version)
	}
}

func TestMigrationsThroughRejectsMissingCutoff(t *testing.T) {
	items := []migration{{version: "001_create_users_table.sql"}}

	if _, err := migrationsThrough(items, legacyBaselineThrough); err == nil {
		t.Fatal("expected a missing cutoff error")
	}
}
