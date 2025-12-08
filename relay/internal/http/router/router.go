package router

import (
	"basegraph.app/relay/internal/http/handler"
	"basegraph.app/relay/internal/service"
	"github.com/gin-gonic/gin"
)

func SetupRoutes(router *gin.Engine, services *service.Services) {
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	v1 := router.Group("/api/v1")
	{
		userHandler := handler.NewUserHandler(services.Users())
		UserRouter(v1.Group("/users"), userHandler)
	}
}
