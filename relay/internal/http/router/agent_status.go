package router

import (
	"basegraph.co/relay/internal/http/handler"
	"github.com/gin-gonic/gin"
)

func AgentStatusRouter(rg *gin.RouterGroup, h *handler.AgentStatusHandler) {
	rg.GET("/orgs/:org_id/workspaces/:workspace_id/stream", h.Stream)
}
