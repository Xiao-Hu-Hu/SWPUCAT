package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"SWPUCAT/internal/application/reportmigration"
	"SWPUCAT/internal/infrastructure/config"
	"SWPUCAT/internal/infrastructure/database"
)

type legacyKnowledgeRow struct {
	ID           int64  `gorm:"column:id"`
	Type         string `gorm:"column:type"`
	Name         string `gorm:"column:name"`
	Description  string `gorm:"column:description"`
	FileKey      string `gorm:"column:file_key"`
	CategoryID   int64  `gorm:"column:category_id"`
	CategoryName string `gorm:"column:category_name"`
	UploaderID   int64  `gorm:"column:uploader_id"`
	UploaderName string `gorm:"column:uploader_name"`
}

func main() {
	categoryID := flag.Int64("category-id", 0, "only scan one legacy knowledge category")
	flag.Parse()
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

	query := db.Table("knowledge_items AS ki").
		Select("ki.id, ki.type, ki.name, ki.description, ki.file_key, ki.category_id, kc.name AS category_name, ki.uploader_id, ki.uploader_name").
		Joins("LEFT JOIN knowledge_categories AS kc ON kc.id = ki.category_id").
		Where("ki.deleted_at IS NULL AND ki.status <> ?", "deleted")
	if *categoryID > 0 {
		query = query.Where("ki.category_id = ?", *categoryID)
	}
	var rows []legacyKnowledgeRow
	if err := query.Order("ki.id ASC").Find(&rows).Error; err != nil {
		log.Fatalf("failed to scan knowledge items: %v", err)
	}
	items := make([]reportmigration.LegacyKnowledgeItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, reportmigration.LegacyKnowledgeItem{
			ID: row.ID, Type: row.Type, Name: row.Name, Description: row.Description,
			FileKey: row.FileKey, CategoryID: row.CategoryID, CategoryName: row.CategoryName,
			UploaderID: row.UploaderID, UploaderName: row.UploaderName,
		})
	}
	result := reportmigration.BuildDryRun(items)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		log.Fatalf("failed to encode dry-run result: %v", err)
	}
}
