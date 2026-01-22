package router

import (
	"basegraph.app/relay/internal/http/handler"
	"basegraph.app/relay/internal/http/handler/webhook"
	"basegraph.app/relay/internal/mapper"
	"basegraph.app/relay/internal/service"
	"github.com/gin-gonic/gin"
)

type RouterConfig struct {
	DashboardURL    string
	IsProduction    bool
	TraceHeaderName string // header name for distributed tracing (e.g., "X-Trace-ID")
	AdminAPIKey     string // API key for admin endpoints
}

func SetupRoutes(router *gin.Engine, services *service.Services, cfg RouterConfig) {
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	invitationService := services.Invitations()

	authHandler := handler.NewAuthHandler(services.Auth(), invitationService, cfg.DashboardURL, cfg.IsProduction)
	AuthRouter(router.Group("/auth"), authHandler)

	// Invitation routes (public validation + admin management)
	invitationHandler := handler.NewInvitationHandler(invitationService, cfg.AdminAPIKey)
	InvitationRouter(router.Group("/invites"), router.Group("/admin/invites"), invitationHandler)

	v1 := router.Group("/api/v1")
	{
		userHandler := handler.NewUserHandler(services.Users())
		UserRouter(v1.Group("/users"), userHandler)

		orgHandler := handler.NewOrganizationHandler(services.Organizations())
		OrganizationRouter(v1.Group("/organizations"), orgHandler)

		gitlabHandler := handler.NewGitLabHandler(services.GitLab(), services.WebhookBaseURL())
		GitLabRouter(v1.Group("/integrations/gitlab"), gitlabHandler)
	}

	mapperRegistry := mapper.NewMapperRegistry()
	gitlabMapper := mapperRegistry.MustGet("gitlab")
	webhookHandler := webhook.NewGitLabWebhookHandler(services.IntegrationCredentials(), services.EventIngest(), gitlabMapper)
	GitLabWebhookRouter(router.Group("/webhooks/gitlab"), webhookHandler)
}
