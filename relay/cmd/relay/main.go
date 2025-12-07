package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"basegraph.app/relay/common/logger"
	"basegraph.app/relay/core/config"
	"basegraph.app/relay/core/db"
	"github.com/gin-gonic/gin"
)

func main() {
	ctx := context.Background()

	cfg := config.Load()

	logger.Setup(cfg)
	slog.Info("relay starting", "env", cfg.Env)

	database, err := db.New(ctx, cfg.DB)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer database.Close()
	slog.Info("database connected")

	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}
	router := setupRouter()

	go func() {
		slog.Info("relay http server starting", "port", cfg.Port)
		if err := router.Run(":" + cfg.Port); err != nil {
			slog.Error("relay http server error", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_ = shutdownCtx // TODO: use for graceful HTTP shutdown when we add it

	slog.Info("shutdown complete")
}

func setupRouter() *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())

	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	return router
}
