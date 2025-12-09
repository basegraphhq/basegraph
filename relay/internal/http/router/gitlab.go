package router

import (
	"basegraph.app/relay/internal/http/handler"
	"github.com/gin-gonic/gin"
)

func GitLabRouter(router *gin.RouterGroup, handler *handler.GitLabHandler) {
	router.POST("/test-connection", handler.TestConnection)
}
