package router

import (
	"basegraph.app/relay/internal/http/handler"
	"github.com/gin-gonic/gin"
)

func UserRouter(rg *gin.RouterGroup, h *handler.UserHandler) {
	rg.POST("", h.Create)
	rg.GET("/:id", h.GetByID)
}
