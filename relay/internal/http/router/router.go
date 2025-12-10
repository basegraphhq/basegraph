package router

import (
	"log/slog"

	"basegraph.app/relay/internal/http/handler"
	"basegraph.app/relay/internal/http/handler/webhook"
	"basegraph.app/relay/internal/service"
	"github.com/gin-gonic/gin"
)

type RouterConfig struct {
	DashboardURL string
	IsProduction bool
}

func SetupRoutes(router *gin.Engine, services *service.Services, cfg RouterConfig) {
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	authHandler := handler.NewAuthHandler(services.Auth(), cfg.DashboardURL, cfg.IsProduction)
	AuthRouter(router.Group("/auth"), authHandler)

	v1 := router.Group("/api/v1")
	{
		userHandler := handler.NewUserHandler(services.Users())
		UserRouter(v1.Group("/users"), userHandler)

		orgHandler := handler.NewOrganizationHandler(services.Organizations())
		OrganizationRouter(v1.Group("/organizations"), orgHandler)

		gitlabHandler := handler.NewGitLabHandler(services.GitLab(), services.WebhookBaseURL())
		GitLabRouter(v1.Group("/integrations/gitlab"), gitlabHandler)
	}

	webhookHandler := webhook.NewGitLabWebhookHandler(services.IntegrationCredentials(), slog.Default())
	GitLabWebhookRouter(router.Group("/webhooks/gitlab"), webhookHandler)
}
