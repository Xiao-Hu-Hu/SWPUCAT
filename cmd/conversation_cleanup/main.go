package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"time"

	conversationapp "SWPUCAT/internal/application/conversation"
	"SWPUCAT/internal/infrastructure/config"
	"SWPUCAT/internal/infrastructure/database"
	"SWPUCAT/internal/infrastructure/repository"
)

func main() {
	execute := flag.Bool("execute", false, "permanently delete eligible conversations; omitted means dry-run")
	batchSize := flag.Int("batch-size", 100, "maximum conversations deleted per transaction (1-1000)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}
	if cfg.Agent.ConversationRetentionDays == 0 {
		log.Printf("conversation cleanup is disabled: set CSA_AGENT_CONVERSATION_RETENTION_DAYS to a positive value")
		return
	}
	if cfg.Agent.ConversationRetentionDays < 0 || cfg.Agent.ConversationRetentionDays > 3650 {
		log.Fatalf("conversation retention days must be between 1 and 3650")
	}
	if err := cfg.ValidateForMigration(); err != nil {
		log.Fatalf("invalid database configuration: %v", err)
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

	service := conversationapp.NewService(repository.NewConversationRepo(db))
	result, err := service.CleanupExpired(
		context.Background(), time.Now().UTC(), cfg.Agent.ConversationRetentionDays, *batchSize, *execute,
	)
	if err != nil {
		log.Fatalf("conversation cleanup failed: %v", err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		log.Fatalf("failed to encode cleanup result: %v", err)
	}
	fmt.Println(string(encoded))
}
