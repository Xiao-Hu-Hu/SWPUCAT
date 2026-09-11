package http

import (
	"SWPUCAT/internal/application/authorization"
	"SWPUCAT/internal/infrastructure/auth"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

type Router struct {
	engine                   *gin.Engine
	jwtSvc                   *auth.JWTService
	authzSvc                 *authorization.Service
	authHandler              *AuthHandler
	userHandler              *UserHandler
	dashboardHandler         *DashboardHandler
	annHandler               *AnnouncementHandler
	checkinHandler           *CheckinHandler
	knowledgeHandler         *KnowledgeHandler
	approvalHandler          *ApprovalHandler
	invitationHandler        *InvitationHandler
	agentHandler             *AgentHandler
	reportHandler            *ReportHandler
	evaluationHandler        *EvaluationHandler
	notificationHandler      *NotificationHandler
	teamQueryHandler         *TeamQueryHandler
	announcementDraftHandler *AnnouncementDraftHandler
}

func NewRouter(
	jwtSvc *auth.JWTService,
	authzSvc *authorization.Service,
	authHandler *AuthHandler,
	userHandler *UserHandler,
	dashboardHandler *DashboardHandler,
	annHandler *AnnouncementHandler,
	checkinHandler *CheckinHandler,
	knowledgeHandler *KnowledgeHandler,
	approvalHandler *ApprovalHandler,
	invitationHandler *InvitationHandler,
	agentHandler *AgentHandler,
	reportHandler *ReportHandler,
	evaluationHandler *EvaluationHandler,
	notificationHandler *NotificationHandler,
	teamQueryHandler *TeamQueryHandler,
	announcementDraftHandler *AnnouncementDraftHandler,
) *Router {
	engine := gin.Default()

	engine.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "Idempotency-Key"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	return &Router{
		engine:                   engine,
		jwtSvc:                   jwtSvc,
		authzSvc:                 authzSvc,
		authHandler:              authHandler,
		userHandler:              userHandler,
		dashboardHandler:         dashboardHandler,
		annHandler:               annHandler,
		checkinHandler:           checkinHandler,
		knowledgeHandler:         knowledgeHandler,
		approvalHandler:          approvalHandler,
		invitationHandler:        invitationHandler,
		agentHandler:             agentHandler,
		reportHandler:            reportHandler,
		evaluationHandler:        evaluationHandler,
		notificationHandler:      notificationHandler,
		teamQueryHandler:         teamQueryHandler,
		announcementDraftHandler: announcementDraftHandler,
	}
}

func (r *Router) Setup() *gin.Engine {
	api := r.engine.Group("/api")
	{
		// Public routes
		auth := api.Group("/auth")
		{
			auth.POST("/register", r.authHandler.Register)
			auth.POST("/login", r.authHandler.Login)
			auth.POST("/refresh", r.authHandler.RefreshToken)
			auth.POST("/send-code", r.authHandler.SendVerificationCode)
		}

		// Public avatar access
		api.GET("/avatar/*path", r.userHandler.GetAvatar)

		// Protected routes
		protected := api.Group("")
		protected.Use(JWTAuthMiddleware(r.jwtSvc, r.authzSvc))
		{
			// User
			protected.GET("/profile", r.userHandler.GetProfile)
			protected.PUT("/profile", r.userHandler.UpdateProfile)
			protected.PUT("/profile/password", r.userHandler.ChangePassword)
			protected.PUT("/profile/tech-direction", r.userHandler.UpdateTechDirection)
			protected.POST("/profile/avatar", r.userHandler.UploadAvatar)
			protected.GET("/members", r.userHandler.ListMembers)
			protected.POST("/members/:id/transfer-captain", RBACMiddleware("captain"), r.userHandler.TransferCaptain)
			protected.DELETE("/members/:id", RBACMiddleware("captain", "super_admin"), r.userHandler.RemoveMember)

			// Dashboard
			protected.GET("/dashboard", r.dashboardHandler.GetDashboard)

			// Announcements
			protected.GET("/announcements", r.annHandler.List)
			protected.POST("/announcements", RBACMiddleware("captain", "super_admin"), r.annHandler.Create)
			protected.PUT("/announcements/:id", RBACMiddleware("captain", "super_admin"), r.annHandler.Update)
			protected.DELETE("/announcements/:id", RBACMiddleware("captain", "super_admin"), r.annHandler.Delete)
			protected.POST("/announcements/:id/notify", RBACMiddleware("captain", "super_admin"), r.annHandler.Notify)

			// Agent-generated announcement drafts. The Agent may create a draft;
			// only a manager can edit, confirm, publish, or discard it.
			protected.POST("/announcement-drafts", RBACMiddleware("captain", "super_admin"), r.announcementDraftHandler.Create)
			protected.GET("/announcement-drafts/:id", RBACMiddleware("captain", "super_admin"), r.announcementDraftHandler.Get)
			protected.PUT("/announcement-drafts/:id", RBACMiddleware("captain", "super_admin"), r.announcementDraftHandler.Update)
			protected.POST("/announcement-drafts/:id/confirm", RBACMiddleware("captain", "super_admin"), r.announcementDraftHandler.Confirm)
			protected.POST("/announcement-drafts/:id/publish", RBACMiddleware("captain", "super_admin"), r.announcementDraftHandler.Publish)
			protected.POST("/announcement-drafts/:id/discard", RBACMiddleware("captain", "super_admin"), r.announcementDraftHandler.Discard)

			// Checkin
			protected.POST("/checkin/clock-in", r.checkinHandler.ClockIn)
			protected.POST("/checkin/clock-out", r.checkinHandler.ClockOut)
			protected.GET("/checkin/records", r.checkinHandler.GetRecords)
			protected.GET("/checkin/status", r.checkinHandler.GetStatus)
			protected.GET("/checkin/stats", r.checkinHandler.GetStats)
			protected.GET("/checkin/online", r.checkinHandler.GetOnlineMembers)
			protected.GET("/checkin/today-records", r.checkinHandler.GetAllTodayRecords)

			// Knowledge
			protected.GET("/knowledge/items", r.knowledgeHandler.ListItems)
			protected.GET("/knowledge/items/pending", RBACMiddleware("captain", "super_admin"), r.knowledgeHandler.ListPendingItems)
			protected.GET("/knowledge/items/my", r.knowledgeHandler.ListUserItems)
			protected.GET("/knowledge/items/:id", r.knowledgeHandler.GetItem)
			protected.GET("/knowledge/items/:id/insight", r.knowledgeHandler.GetInsight)
			protected.POST("/knowledge/items/:id/insights", RBACMiddleware("captain", "super_admin"), r.knowledgeHandler.CreateInsight)
			protected.GET("/knowledge/recommendations", r.knowledgeHandler.Recommend)
			protected.POST("/knowledge/recommendations/:id/feedback", r.knowledgeHandler.RecommendationFeedback)
			protected.GET("/knowledge/insights/due", RBACMiddleware("captain", "super_admin"), r.knowledgeHandler.ListDueInsights)
			protected.POST("/knowledge/insights/:id/publish", RBACMiddleware("captain", "super_admin"), r.knowledgeHandler.PublishInsight)
			protected.POST("/knowledge/insights/:id/invalidate", RBACMiddleware("captain", "super_admin"), r.knowledgeHandler.InvalidateInsight)
			protected.POST("/knowledge/links", r.knowledgeHandler.CreateLink)
			protected.POST("/knowledge/files", r.knowledgeHandler.UploadFile)
			protected.DELETE("/knowledge/items/:id", r.knowledgeHandler.DeleteItem)
			protected.PUT("/knowledge/items/:id/approve", RBACMiddleware("captain", "super_admin"), r.knowledgeHandler.ApproveItem)
			protected.PUT("/knowledge/items/:id/reject", RBACMiddleware("captain", "super_admin"), r.knowledgeHandler.RejectItem)
			protected.GET("/knowledge/download/:id", r.knowledgeHandler.DownloadFile)
			protected.GET("/knowledge/categories", r.knowledgeHandler.ListCategories)
			protected.POST("/knowledge/categories", RBACMiddleware("captain", "super_admin"), r.knowledgeHandler.CreateCategory)
			protected.DELETE("/knowledge/categories/:id", RBACMiddleware("captain", "super_admin"), r.knowledgeHandler.DeleteCategory)

			// Approvals
			protected.GET("/approvals", RBACMiddleware("captain", "super_admin"), r.approvalHandler.ListPending)
			protected.POST("/approvals", r.approvalHandler.Submit)
			protected.POST("/approvals/:id/approve", RBACMiddleware("captain", "super_admin"), r.approvalHandler.Approve)
			protected.POST("/approvals/:id/reject", RBACMiddleware("captain", "super_admin"), r.approvalHandler.Reject)

			// Invitation codes (super_admin and captain only)
			protected.POST("/invitations/generate", RBACMiddleware("captain", "super_admin"), r.invitationHandler.GenerateCode)
			protected.GET("/invitations/my", r.invitationHandler.GetMyCodes)

			// Checkin Management (captain/super_admin only)
			protected.POST("/checkin/makeup", RBACMiddleware("captain", "super_admin"), r.checkinHandler.Makeup)
			protected.GET("/checkin/requirements", r.checkinHandler.GetRequirements)
			protected.POST("/checkin/requirements", RBACMiddleware("captain", "super_admin"), r.checkinHandler.SetRequirements)
			protected.POST("/checkin/report", RBACMiddleware("captain", "super_admin"), r.checkinHandler.PublishReport)

			// Agent assistant. JWT middleware above resolves the user and role;
			// the handler forwards that identity to the private Agent service.
			protected.POST("/agent/chat", r.agentHandler.Chat)

			// Monthly reports. Drafts and versions remain private to their author
			// unless a captain/super admin is reviewing them.
			protected.GET("/report-periods", r.reportHandler.ListPeriods)
			protected.POST("/report-periods", RBACMiddleware("captain", "super_admin"), r.reportHandler.CreatePeriod)
			protected.POST("/report-periods/:id/members", RBACMiddleware("captain", "super_admin"), r.reportHandler.AddPeriodMember)
			protected.POST("/reports", r.reportHandler.CreateReport)
			protected.GET("/reports", r.reportHandler.ListReports)
			protected.GET("/reports/:id", r.reportHandler.GetReport)
			protected.POST("/reports/:id/versions", r.reportHandler.CreateVersion)
			protected.GET("/reports/:id/versions/:version", r.reportHandler.GetVersion)
			protected.POST("/reports/:id/versions/:version/submit", r.reportHandler.SubmitVersion)
			protected.POST("/reports/:id/versions/:version/review", RBACMiddleware("captain", "super_admin"), r.reportHandler.ReviewVersion)
			protected.POST("/reports/:id/versions/:version/assets", r.reportHandler.AttachAsset)
			protected.POST("/reports/:id/versions/:version/check", r.reportHandler.CheckQuality)
			protected.POST("/reports/:id/versions/:version/quality/:finding/decision", r.reportHandler.DecideQuality)

			// Captain evaluations are always bound to an immutable evidence
			// snapshot. Only managers can edit, review, or publish a draft.
			protected.POST("/evaluations/drafts", RBACMiddleware("captain", "super_admin"), r.evaluationHandler.CreateDraft)
			protected.GET("/evaluations/:id", RBACMiddleware("captain", "super_admin"), r.evaluationHandler.GetDraft)
			protected.PUT("/evaluations/:id/draft", RBACMiddleware("captain", "super_admin"), r.evaluationHandler.UpdateDraft)
			protected.POST("/evaluations/:id/review", RBACMiddleware("captain", "super_admin"), r.evaluationHandler.ReviewDraft)
			protected.POST("/evaluations/:id/publish", RBACMiddleware("captain", "super_admin"), r.evaluationHandler.PublishDraft)
			protected.PUT("/evaluations/:id/members/:user_id", RBACMiddleware("captain", "super_admin"), r.evaluationHandler.SaveMemberComment)

			// In-app notifications and per-user reminder preferences.
			protected.GET("/notifications", r.notificationHandler.List)
			protected.POST("/notifications/:id/read", r.notificationHandler.MarkRead)
			protected.GET("/notifications/preferences", r.notificationHandler.GetPreferences)
			protected.PUT("/notifications/preferences", r.notificationHandler.UpdatePreferences)

			// Manager-only structured team query; the backend performs the
			// deterministic filtering and never executes model-generated SQL.
			protected.GET("/analysis/team", RBACMiddleware("captain", "super_admin"), r.teamQueryHandler.Query)
		}
	}

	return r.engine
}
