package http

import (
	"SWPUCAT/internal/infrastructure/auth"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

type Router struct {
	engine            *gin.Engine
	jwtSvc            *auth.JWTService
	authHandler       *AuthHandler
	userHandler       *UserHandler
	dashboardHandler  *DashboardHandler
	annHandler        *AnnouncementHandler
	checkinHandler    *CheckinHandler
	knowledgeHandler  *KnowledgeHandler
	approvalHandler   *ApprovalHandler
	invitationHandler *InvitationHandler
}

func NewRouter(
	jwtSvc *auth.JWTService,
	authHandler *AuthHandler,
	userHandler *UserHandler,
	dashboardHandler *DashboardHandler,
	annHandler *AnnouncementHandler,
	checkinHandler *CheckinHandler,
	knowledgeHandler *KnowledgeHandler,
	approvalHandler *ApprovalHandler,
	invitationHandler *InvitationHandler,
) *Router {
	engine := gin.Default()

	engine.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	return &Router{
		engine:            engine,
		jwtSvc:            jwtSvc,
		authHandler:       authHandler,
		userHandler:       userHandler,
		dashboardHandler:  dashboardHandler,
		annHandler:        annHandler,
		checkinHandler:    checkinHandler,
		knowledgeHandler:  knowledgeHandler,
		approvalHandler:   approvalHandler,
		invitationHandler: invitationHandler,
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
		protected.Use(JWTAuthMiddleware(r.jwtSvc))
		{
			// User
			protected.GET("/profile", r.userHandler.GetProfile)
			protected.PUT("/profile", r.userHandler.UpdateProfile)
			protected.PUT("/profile/password", r.userHandler.ChangePassword)
			protected.POST("/profile/avatar", r.userHandler.UploadAvatar)
			protected.GET("/members", r.userHandler.ListMembers)
			protected.POST("/members/:id/transfer-captain", r.userHandler.TransferCaptain)
			protected.DELETE("/members/:id", r.userHandler.RemoveMember)

			// Dashboard
			protected.GET("/dashboard", r.dashboardHandler.GetDashboard)

			// Announcements
			protected.GET("/announcements", r.annHandler.List)
			protected.POST("/announcements", r.annHandler.Create)
			protected.PUT("/announcements/:id", r.annHandler.Update)
			protected.DELETE("/announcements/:id", r.annHandler.Delete)

			// Checkin
			protected.POST("/checkin/clock-in", r.checkinHandler.ClockIn)
			protected.POST("/checkin/clock-out", r.checkinHandler.ClockOut)
			protected.GET("/checkin/records", r.checkinHandler.GetRecords)
			protected.GET("/checkin/status", r.checkinHandler.GetStatus)
			protected.GET("/checkin/stats", r.checkinHandler.GetStats)
			protected.GET("/checkin/online", r.checkinHandler.GetOnlineMembers)

			// Knowledge
			protected.GET("/knowledge/items", r.knowledgeHandler.ListItems)
			protected.GET("/knowledge/items/pending", r.knowledgeHandler.ListPendingItems)
			protected.GET("/knowledge/items/my", r.knowledgeHandler.ListUserItems)
			protected.GET("/knowledge/items/:id", r.knowledgeHandler.GetItem)
			protected.POST("/knowledge/links", r.knowledgeHandler.CreateLink)
			protected.POST("/knowledge/files", r.knowledgeHandler.UploadFile)
			protected.DELETE("/knowledge/items/:id", r.knowledgeHandler.DeleteItem)
			protected.PUT("/knowledge/items/:id/approve", r.knowledgeHandler.ApproveItem)
			protected.PUT("/knowledge/items/:id/reject", r.knowledgeHandler.RejectItem)
			protected.GET("/knowledge/download/:id", r.knowledgeHandler.DownloadFile)
			protected.GET("/knowledge/categories", r.knowledgeHandler.ListCategories)
			protected.POST("/knowledge/categories", r.knowledgeHandler.CreateCategory)
			protected.DELETE("/knowledge/categories/:id", r.knowledgeHandler.DeleteCategory)

			// Approvals
			protected.GET("/approvals", r.approvalHandler.ListPending)
			protected.POST("/approvals", r.approvalHandler.Submit)
			protected.POST("/approvals/:id/approve", r.approvalHandler.Approve)
			protected.POST("/approvals/:id/reject", r.approvalHandler.Reject)

			// Invitation codes (super_admin and captain only)
			protected.POST("/invitations/generate", r.invitationHandler.GenerateCode)
			protected.GET("/invitations/my", r.invitationHandler.GetMyCodes)
		}
	}

	return r.engine
}
