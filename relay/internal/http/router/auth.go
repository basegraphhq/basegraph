package router

import (
	"basegraph.app/relay/internal/http/handler"
	"github.com/gin-gonic/gin"
)

func AuthRouter(rg *gin.RouterGroup, h *handler.UserHandler) {
	rg.POST("/sync", h.Sync)
}
