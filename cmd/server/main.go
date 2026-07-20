package main

import (
	"SWPUCAT/internal/application/announcement"
	"SWPUCAT/internal/application/approval"
	"SWPUCAT/internal/application/checkin"
	"SWPUCAT/internal/application/dashboard"
	"SWPUCAT/internal/application/knowledge"
	"SWPUCAT/internal/application/user"
	"SWPUCAT/internal/infrastructure/auth"
	"SWPUCAT/internal/infrastructure/config"
	"SWPUCAT/internal/infrastructure/database"
	"SWPUCAT/internal/infrastructure/event"
	"SWPUCAT/internal/infrastructure/repository"
	httpInterface "SWPUCAT/internal/interfaces/http"
	"fmt"
	"log"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database
	db, err := database.NewGORM(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Initialize repositories
	userRepo := repository.NewUserRepo(db)
	annRepo := repository.NewAnnouncementRepo(db)
	checkinRepo := repository.NewCheckinRepo(db)
	knowledgeRepo := repository.NewKnowledgeRepo(db)
	approvalRepo := repository.NewApprovalRepo(db)

	// Initialize infrastructure services
	jwtSvc := auth.NewJWTService(&cfg.JWT)
	hasher := auth.NewBcryptHasher()
	publisher := event.NewNoOpPublisher()

	// Initialize application services
	userSvc := user.NewUserApplicationService(userRepo, hasher, jwtSvc, publisher)
	annSvc := announcement.NewAnnouncementService(annRepo, publisher)
	checkinSvc := checkin.NewCheckinService(checkinRepo, userRepo, publisher)
	knowledgeSvc := knowledge.NewKnowledgeService(knowledgeRepo, publisher)
	approvalSvc := approval.NewApprovalService(approvalRepo, knowledgeRepo, publisher)
	dashboardSvc := dashboard.NewDashboardService(userRepo, checkinRepo, annRepo, knowledgeRepo)

	// Initialize handlers
	authHandler := httpInterface.NewAuthHandler(userSvc)
	userHandler := httpInterface.NewUserHandler(userSvc)
	dashboardHandler := httpInterface.NewDashboardHandler(dashboardSvc)
	annHandler := httpInterface.NewAnnouncementHandler(annSvc)
	checkinHandler := httpInterface.NewCheckinHandler(checkinSvc)
	knowledgeHandler := httpInterface.NewKnowledgeHandler(knowledgeSvc)
	approvalHandler := httpInterface.NewApprovalHandler(approvalSvc)

	// Initialize router
	router := httpInterface.NewRouter(
		jwtSvc,
		authHandler,
		userHandler,
		dashboardHandler,
		annHandler,
		checkinHandler,
		knowledgeHandler,
		approvalHandler,
	)

	// Setup routes
	engine := router.Setup()

	// Start server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("Server starting on %s", addr)
	if err := engine.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
