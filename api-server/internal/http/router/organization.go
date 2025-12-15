package router

import (
	"basegraph.app/api-server/internal/http/handler"
	"github.com/gin-gonic/gin"
)

func OrganizationRouter(rg *gin.RouterGroup, h *handler.OrganizationHandler) {
	rg.POST("", h.Create)
}
