package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"SWPUCAT/internal/infrastructure/config"
	"SWPUCAT/internal/infrastructure/database"
)

const migrationTableSQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version VARCHAR(255) PRIMARY KEY,
    checksum VARCHAR(64) NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);`

const legacyBaselineThrough = "015_add_tech_direction_to_users.sql"

var legacyRequiredRelations = []string{
	"users",
	"announcements",
	"checkin_records",
	"knowledge_categories",
	"knowledge_items",
	"approvals",
	"settings",
	"verification_codes",
	"invitation_codes",
}

var legacyRequiredColumns = []struct {
	table  string
	column string
}{
	{table: "users", column: "student_id"},
	{table: "users", column: "email"},
	{table: "users", column: "avatar"},
	{table: "users", column: "tech_direction"},
	{table: "knowledge_items", column: "rejected"},
	{table: "knowledge_items", column: "reject_reason"},
	{table: "knowledge_items", column: "reviewer_id"},
	{table: "knowledge_items", column: "reviewer_name"},
	{table: "knowledge_items", column: "description"},
}

type migration struct {
	version  string
	path     string
	checksum string
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}
	if err := cfg.ValidateForMigration(); err != nil {
		log.Fatalf("invalid migration configuration: %v", err)
	}

	db, err := database.NewGORM(&cfg.Database)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("failed to get database connection: %v", err)
	}
	defer sqlDB.Close()

	if _, err := sqlDB.Exec(`SELECT pg_advisory_lock(hashtextextended('swpucat:schema-migrations', 0))`); err != nil {
		log.Fatalf("failed to acquire migration lock: %v", err)
	}
	defer func() {
		if _, err := sqlDB.Exec(`SELECT pg_advisory_unlock(hashtextextended('swpucat:schema-migrations', 0))`); err != nil {
			log.Printf("failed to release migration lock: %v", err)
		}
	}()

	if _, err := sqlDB.Exec(migrationTableSQL); err != nil {
		log.Fatalf("failed to initialize schema_migrations: %v", err)
	}

	migrationsDir := os.Getenv("CSA_MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "./migrations"
	}
	migrations, err := loadMigrations(migrationsDir)
	if err != nil {
		log.Fatalf("failed to load migrations: %v", err)
	}

	if err := maybeBaseline(sqlDB, migrations); err != nil {
		log.Fatalf("migration baseline check failed: %v", err)
	}

	for _, item := range migrations {
		applied, checksum, err := migrationState(sqlDB, item.version)
		if err != nil {
			log.Fatalf("failed to read migration %s: %v", item.version, err)
		}
		if applied {
			if checksum != item.checksum {
				log.Fatalf("migration %s was modified after it was applied", item.version)
			}
			log.Printf("migration %s already applied", item.version)
			continue
		}

		sqlText, err := os.ReadFile(item.path)
		if err != nil {
			log.Fatalf("failed to read migration %s: %v", item.version, err)
		}
		tx, err := sqlDB.Begin()
		if err != nil {
			log.Fatalf("failed to begin migration %s: %v", item.version, err)
		}
		if _, err := tx.Exec(string(sqlText)); err != nil {
			_ = tx.Rollback()
			log.Fatalf("migration %s failed: %v", item.version, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)`, item.version, item.checksum); err != nil {
			_ = tx.Rollback()
			log.Fatalf("failed to record migration %s: %v", item.version, err)
		}
		if err := tx.Commit(); err != nil {
			log.Fatalf("failed to commit migration %s: %v", item.version, err)
		}
		log.Printf("applied migration %s", item.version)
	}
}

func loadMigrations(dir string) ([]migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	items := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		items = append(items, migration{
			version:  entry.Name(),
			path:     path,
			checksum: checksum(contents),
		})
	}
	if len(items) == 0 {
		return nil, errors.New("no .sql migration files found")
	}
	sort.Slice(items, func(i, j int) bool { return items[i].version < items[j].version })
	return items, nil
}

func checksum(contents []byte) string {
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}

func migrationState(db *sql.DB, version string) (bool, string, error) {
	var stored string
	err := db.QueryRow(`SELECT checksum FROM schema_migrations WHERE version = $1`, version).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, stored, nil
}

func maybeBaseline(db *sql.DB, migrations []migration) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	existing, err := hasExistingApplicationSchema(db)
	if err != nil {
		return err
	}
	if !existing {
		if os.Getenv("CSA_MIGRATION_BASELINE") == "true" {
			return fmt.Errorf("CSA_MIGRATION_BASELINE=true requires an existing legacy application schema")
		}
		return nil
	}
	if os.Getenv("CSA_MIGRATION_BASELINE") != "true" {
		return fmt.Errorf("database contains an existing application schema without migration history; back it up and set CSA_MIGRATION_BASELINE=true to verify and baseline legacy migrations through %s", legacyBaselineThrough)
	}
	if err := validateLegacySchema(db); err != nil {
		return err
	}
	legacyMigrations, err := migrationsThrough(migrations, legacyBaselineThrough)
	if err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range legacyMigrations {
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2) ON CONFLICT (version) DO NOTHING`, item.version, item.checksum); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("baselined %d legacy migrations through %s; later migrations will run normally", len(legacyMigrations), legacyBaselineThrough)
	return nil
}

func hasExistingApplicationSchema(db *sql.DB) (bool, error) {
	for _, relation := range legacyRequiredRelations {
		var exists bool
		if err := db.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, "public."+relation).Scan(&exists); err != nil {
			return false, err
		}
		if exists {
			return true, nil
		}
	}
	return false, nil
}

func validateLegacySchema(db *sql.DB) error {
	missing := make([]string, 0)
	for _, relation := range legacyRequiredRelations {
		var exists bool
		if err := db.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, "public."+relation).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			missing = append(missing, "table "+relation)
		}
	}
	for _, requirement := range legacyRequiredColumns {
		var exists bool
		if err := db.QueryRow(`
			SELECT EXISTS (
				SELECT 1
				FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2
			)`, requirement.table, requirement.column).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			missing = append(missing, "column "+requirement.table+"."+requirement.column)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("legacy schema cannot be baselined through %s; missing %s", legacyBaselineThrough, strings.Join(missing, ", "))
	}
	return nil
}

func migrationsThrough(migrations []migration, cutoff string) ([]migration, error) {
	result := make([]migration, 0, len(migrations))
	found := false
	for _, item := range migrations {
		result = append(result, item)
		if item.version == cutoff {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("legacy baseline cutoff migration %s was not found", cutoff)
	}
	return result, nil
}
