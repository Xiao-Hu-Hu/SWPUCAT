package main

import (
	"SWPUCAT/internal/application/announcement"
	announcementdraftapp "SWPUCAT/internal/application/announcementdraft"
	"SWPUCAT/internal/application/approval"
	"SWPUCAT/internal/application/authorization"
	"SWPUCAT/internal/application/checkin"
	"SWPUCAT/internal/application/dashboard"
	evaluationapp "SWPUCAT/internal/application/evaluation"
	"SWPUCAT/internal/application/invitation"
	"SWPUCAT/internal/application/knowledge"
	notificationapp "SWPUCAT/internal/application/notification"
	reportapp "SWPUCAT/internal/application/report"
	teamqueryapp "SWPUCAT/internal/application/teamquery"
	"SWPUCAT/internal/application/user"
	"SWPUCAT/internal/infrastructure/agentclient"
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
	"net/http"
	"time"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if err := cfg.ValidateForServer(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
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
	reportRepo := repository.NewReportRepo(db)
	evaluationRepo := repository.NewEvaluationRepo(db)
	notificationRepo := repository.NewNotificationRepo(db)
	announcementDraftRepo := repository.NewAnnouncementDraftRepo(db)
	approvalRepo := repository.NewApprovalRepo(db)
	codeRepo := repository.NewVerificationCodeRepo(db)
	invitationRepo := repository.NewInvitationRepo(db)
	registrationRepo := repository.NewRegistrationRepo(db)
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
	userSvc := user.NewUserApplicationService(userRepo, hasher, jwtSvc, publisher, emailSvc, codeRepo, invitationRepo, registrationRepo)
	annSvc := announcement.NewAnnouncementService(annRepo, publisher, userRepo, emailSvc)
	announcementDraftSvc := announcementdraftapp.NewService(announcementDraftRepo, annSvc)
	invitationSvc := invitation.NewInvitationService(invitationRepo, userRepo)
	notificationSvc := notificationapp.NewService(notificationRepo, userRepo, reportRepo)
	checkinSvc := checkin.NewCheckinService(checkinRepo, userRepo, publisher, settingsRepo, notificationSvc)
	knowledgeSvc := knowledge.NewKnowledgeService(knowledgeRepo, publisher)
	reportSvc := reportapp.NewService(reportRepo)
	evaluationSvc := evaluationapp.NewService(evaluationRepo, notificationSvc)
	teamQuerySvc := teamqueryapp.NewService(checkinSvc, reportSvc)
	approvalSvc := approval.NewApprovalService(approvalRepo, knowledgeRepo, publisher)
	dashboardSvc := dashboard.NewDashboardService(userRepo, checkinRepo, annRepo, knowledgeRepo)
	authzSvc := authorization.NewService(userRepo)

	// Initialize handlers
	authHandler := httpInterface.NewAuthHandler(userSvc)
	userHandler := httpInterface.NewUserHandler(userSvc, localStorage)
	dashboardHandler := httpInterface.NewDashboardHandler(dashboardSvc)
	annHandler := httpInterface.NewAnnouncementHandler(annSvc)
	checkinHandler := httpInterface.NewCheckinHandler(checkinSvc, annSvc)
	knowledgeHandler := httpInterface.NewKnowledgeHandler(knowledgeSvc, localStorage)
	approvalHandler := httpInterface.NewApprovalHandler(approvalSvc)
	invitationHandler := httpInterface.NewInvitationHandler(invitationSvc)
	agentClient, err := agentclient.NewClient(cfg.Agent.BaseURL, cfg.Agent.InternalSecret, cfg.Agent.LLMTimeout+15*time.Second)
	if err != nil {
		log.Fatalf("Failed to initialize agent client: %v", err)
	}
	agentHandler := httpInterface.NewAgentHandler(agentClient)
	reportHandler := httpInterface.NewReportHandler(reportSvc)
	evaluationHandler := httpInterface.NewEvaluationHandler(evaluationSvc)
	notificationHandler := httpInterface.NewNotificationHandler(notificationSvc)
	teamQueryHandler := httpInterface.NewTeamQueryHandler(teamQuerySvc)
	announcementDraftHandler := httpInterface.NewAnnouncementDraftHandler(announcementDraftSvc)

	// Initialize router
	router := httpInterface.NewRouter(
		jwtSvc,
		authzSvc,
		authHandler,
		userHandler,
		dashboardHandler,
		annHandler,
		checkinHandler,
		knowledgeHandler,
		approvalHandler,
		invitationHandler,
		agentHandler,
		reportHandler,
		evaluationHandler,
		notificationHandler,
		teamQueryHandler,
		announcementDraftHandler,
	)

	// Setup routes
	engine := router.Setup()

	// Schedule midnight auto clock-out
	go func() {
		// Catch up the previous date after a restart. The repository's durable
		// claim makes this safe when multiple instances start together.
		now := time.Now()
		yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
		if err := checkinSvc.AutoClockOut(context.Background(), yesterday); err != nil {
			log.Printf("AutoClockOut catch-up failed for %s: %v", yesterday, err)
		}
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

	// Run database-backed report due reminders. The unique notification key
	// makes startup catch-up and hourly retries safe without a broker.
	go func() {
		run := func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := notificationSvc.RunReportDueReminders(ctx, time.Now(), 48*time.Hour); err != nil {
				log.Printf("report due reminder run failed: %v", err)
			}
		}
		run()
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			run()
		}
	}()

	// Start server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      engine,
		ReadTimeout:  15 * time.Minute,
		WriteTimeout: 15 * time.Minute,
	}
	log.Printf("Server starting on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Failed to start server: %v", err)
	}
}
