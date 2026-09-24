package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"SWPUCAT/internal/domain/checkin"
	"SWPUCAT/internal/infrastructure/database"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const checkinPostgresTestDSNEnv = "SWPUCAT_TEST_POSTGRES_DSN"

// TestCheckinRepoPostgresMakeupDoesNotBlockClockOut is opt-in like the agent
// audit test: it writes to, and then removes its own rows from, a PostgreSQL 16
// database migrated through 040_add_checkin_makeup_minutes.sql. Point the DSN
// only at a disposable database whose name starts with "swpucat_verify_".
//
// It guards the regression where a makeup record created while the member is
// clocked in became the newest row and made the next clock-out return
// ErrNotClockedIn.
func TestCheckinRepoPostgresMakeupDoesNotBlockClockOut(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv(checkinPostgresTestDSNEnv))
	if dsn == "" {
		t.Skipf("set %s to a disposable PostgreSQL 16 database migrated through 040", checkinPostgresTestDSNEnv)
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open PostgreSQL test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get PostgreSQL connection: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		t.Fatalf("ping PostgreSQL test database: %v", err)
	}

	var databaseName string
	if err := db.WithContext(ctx).Raw(`SELECT current_database()`).Row().Scan(&databaseName); err != nil {
		t.Fatalf("inspect PostgreSQL test database: %v", err)
	}
	if !strings.HasPrefix(databaseName, "swpucat_verify_") {
		t.Fatalf("refusing to write to database %q; test database name must start with swpucat_verify_", databaseName)
	}

	suffix, err := randomHex(6)
	if err != nil {
		t.Fatalf("random suffix: %v", err)
	}
	user := database.UserModel{
		Username:     "verify_" + suffix,
		PasswordHash: "x",
		Nickname:     "verify",
		Role:         "member",
		Status:       "active",
	}
	if err := db.WithContext(ctx).Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		// Remove the rows this test created (ignore order/absence of cascades).
		_ = db.WithContext(context.Background()).Where("user_id = ?", user.ID).Delete(&database.CheckinRecordModel{}).Error
		_ = db.WithContext(context.Background()).Where("user_id = ?", user.ID).Delete(&database.CheckinIdempotencyModel{}).Error
		_ = db.WithContext(context.Background()).Delete(&database.UserModel{}, user.ID).Error
	})

	repo := NewCheckinRepo(db)
	date := time.Now().Format("2006-01-02")

	in := &checkin.CheckinRecord{UserID: user.ID, Type: checkin.CheckinTypeIn, Date: date, Time: "09:00:00"}
	if _, err := repo.CreateTransition(ctx, in, ""); err != nil {
		t.Fatalf("clock in: %v", err)
	}

	makeup := &checkin.CheckinRecord{UserID: user.ID, Type: checkin.CheckinTypeMakeup, Date: date, Minutes: 60, Time: ""}
	if err := repo.CreateMakeup(ctx, makeup); err != nil {
		t.Fatalf("create makeup while clocked in: %v", err)
	}

	out := &checkin.CheckinRecord{UserID: user.ID, Type: checkin.CheckinTypeOut, Date: date, Time: "18:00:00"}
	created, err := repo.CreateTransition(ctx, out, "")
	if err != nil {
		t.Fatalf("clock out after makeup must succeed, got: %v", err)
	}
	if !created {
		t.Fatalf("clock out should create a new record")
	}
}

func randomHex(nBytes int) (string, error) {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
