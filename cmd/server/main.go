package main

import (
	"SWPUCAT/internal/application/announcement"
	"SWPUCAT/internal/application/approval"
	"SWPUCAT/internal/application/checkin"
	"SWPUCAT/internal/application/dashboard"
	"SWPUCAT/internal/application/invitation"
	"SWPUCAT/internal/application/knowledge"
	"SWPUCAT/internal/application/user"
	"SWPUCAT/internal/infrastructure/auth"
	"SWPUCAT/internal/infrastructure/config"
	"SWPUCAT/internal/infrastructure/database"
	"SWPUCAT/internal/infrastructure/email"
	"SWPUCAT/internal/infrastructure/event"
	"SWPUCAT/internal/infrastructure/repository"
	"SWPUCAT/internal/infrastructure/storage"
	httpInterface "SWPUCAT/internal/interfaces/http"
	"context"
	"fmt"
	"log"
	"time"
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
	codeRepo := repository.NewVerificationCodeRepo(db)
	invitationRepo := repository.NewInvitationRepo(db)
	settingsRepo := repository.NewSettingsRepo(db)

	// Initialize infrastructure services
	jwtSvc := auth.NewJWTService(&cfg.JWT)
	hasher := auth.NewBcryptHasher()
	publisher := event.NewNoOpPublisher()
	localStorage, err := storage.NewLocalStorage(cfg.Storage.UploadDir)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	emailSvc := email.NewSMTPService(&cfg.Email)

	// Initialize application services
	userSvc := user.NewUserApplicationService(userRepo, hasher, jwtSvc, publisher, emailSvc, codeRepo, invitationRepo)
	annSvc := announcement.NewAnnouncementService(annRepo, publisher, userRepo)
	invitationSvc := invitation.NewInvitationService(invitationRepo, userRepo)
	checkinSvc := checkin.NewCheckinService(checkinRepo, userRepo, publisher, settingsRepo)
	knowledgeSvc := knowledge.NewKnowledgeService(knowledgeRepo, publisher)
	approvalSvc := approval.NewApprovalService(approvalRepo, knowledgeRepo, publisher)
	dashboardSvc := dashboard.NewDashboardService(userRepo, checkinRepo, annRepo, knowledgeRepo)

	// Initialize handlers
	authHandler := httpInterface.NewAuthHandler(userSvc)
	userHandler := httpInterface.NewUserHandler(userSvc, localStorage)
	dashboardHandler := httpInterface.NewDashboardHandler(dashboardSvc)
	annHandler := httpInterface.NewAnnouncementHandler(annSvc)
	checkinHandler := httpInterface.NewCheckinHandler(checkinSvc, annSvc)
	knowledgeHandler := httpInterface.NewKnowledgeHandler(knowledgeSvc, localStorage)
	approvalHandler := httpInterface.NewApprovalHandler(approvalSvc)
	invitationHandler := httpInterface.NewInvitationHandler(invitationSvc)

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
		invitationHandler,
	)

	// Setup routes
	engine := router.Setup()

	// Schedule midnight auto clock-out
	go func() {
		for {
			now := time.Now()
			midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
			time.Sleep(time.Until(midnight))
			yesterday := midnight.Add(-24 * time.Hour).Format("2006-01-02")
			if err := checkinSvc.AutoClockOut(context.Background(), yesterday); err != nil {
				log.Printf("AutoClockOut failed: %v", err)
			} else {
				log.Printf("AutoClockOut completed for %s", yesterday)
			}
		}
	}()

	// Start server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("Server starting on %s", addr)
	if err := engine.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
