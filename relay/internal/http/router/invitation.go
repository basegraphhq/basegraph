package router

import (
	"basegraph.app/relay/internal/http/handler"
	"github.com/gin-gonic/gin"
)

// InvitationRouter sets up invitation routes
// - /invites/validate is public (for dashboard to validate tokens)
// - /invites/accept requires authentication (X-Session-ID header)
// - /admin/invites/* routes require admin API key
func InvitationRouter(rg *gin.RouterGroup, adminRg *gin.RouterGroup, h *handler.InvitationHandler) {
	// Public endpoint - validate invitation token
	rg.GET("/validate", h.Validate)

	// Authenticated endpoint - accept invitation while already signed in
	// Session validation is done inside the handler (checks X-Session-ID header)
	rg.POST("/accept", h.Accept)

	// Admin endpoints - require API key
	admin := adminRg.Group("")
	admin.Use(h.RequireAdminAPIKey())
	{
		admin.POST("", h.Create)
		admin.GET("", h.List)
		admin.GET("/pending", h.ListPending)
		admin.POST("/revoke", h.Revoke)
	}
}
