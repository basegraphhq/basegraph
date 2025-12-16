package router

import (
	"basegraph.app/relay/internal/http/handler"
	"github.com/gin-gonic/gin"
)

func AuthRouter(rg *gin.RouterGroup, h *handler.AuthHandler) {
	rg.GET("/login", h.Login)
	rg.GET("/callback", h.Callback)
	rg.POST("/logout", h.Logout)
	rg.GET("/me", h.Me)

	rg.GET("/url", h.GetAuthURL)
	rg.POST("/exchange", h.Exchange)
	rg.GET("/validate", h.ValidateSession)
	rg.POST("/logout-session", h.LogoutSession)
}
