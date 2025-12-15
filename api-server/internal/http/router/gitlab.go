package router

import (
	"basegraph.app/api-server/internal/http/handler"
	"basegraph.app/api-server/internal/http/handler/webhook"
	"github.com/gin-gonic/gin"
)

func GitLabRouter(router *gin.RouterGroup, handler *handler.GitLabHandler) {
	router.POST("/projects", handler.ListProjects)
	router.POST("/setup", handler.SetupIntegration)
	router.GET("/status", handler.GetStatus)
	router.POST("/refresh", handler.RefreshIntegration)
}

func GitLabWebhookRouter(router *gin.RouterGroup, handler *webhook.GitLabWebhookHandler) {
	router.POST("/:integration_id", handler.HandleEvent)
}
