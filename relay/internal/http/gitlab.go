package http

import (
	"log/slog"

	"github.com/gin-gonic/gin"
)

func GitlabCallback(c *gin.Context) {

	var req map[string]any

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	slog.Info("GitLab callback received", "request", req)

	c.JSON(200, gin.H{"status": "ok"})
}
