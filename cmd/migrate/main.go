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
	if count > 0 || os.Getenv("CSA_MIGRATION_BASELINE") != "true" {
		return nil
	}

	var existing bool
	if err := db.QueryRow(`SELECT to_regclass('public.users') IS NOT NULL AND to_regclass('public.knowledge_items') IS NOT NULL`).Scan(&existing); err != nil {
		return err
	}
	if !existing {
		return fmt.Errorf("CSA_MIGRATION_BASELINE=true requires an existing users and knowledge_items schema")
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range migrations {
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2) ON CONFLICT (version) DO NOTHING`, item.version, item.checksum); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("baselined %d existing migrations; future migrations will run normally", len(migrations))
	return nil
}
