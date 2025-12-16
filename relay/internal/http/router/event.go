package router

import (
	"basegraph.app/relay/internal/http/handler"
	"github.com/gin-gonic/gin"
)

func EventRouter(router *gin.RouterGroup, handler *handler.EventIngestHandler) {
	router.POST("/ingest", handler.Ingest)
}
