package main

import (
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	g := gin.Default()

	if os.Getenv("RAILWAY_ENVIRONMENT_NAME") == "production" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	g.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "Hello, World!",
		})
	})
	slog.Info("Relay is running on port 8080")
	g.Run(":8080")
}
