package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"basegraph.app/relay/core/config"
	"basegraph.app/relay/core/db"
	"basegraph.app/relay/internal/http"
	"basegraph.app/relay/internal/repository"
	"github.com/gin-gonic/gin"
)

func main() {
	ctx := context.Background()

	// Load configuration
	cfg := config.Load()

	// Setup logger
	setupLogger(cfg)
	slog.Info("relay starting", "env", cfg.Env)

	// Initialize database
	database, err := db.New(ctx, cfg.DB)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer database.Close()
	slog.Info("database connected")

	// Create repositories (for non-transactional operations)
	repos := repository.NewRepositories(database.Queries())

	// Setup HTTP server
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}
	router := setupRouter(database, repos)

	// Graceful shutdown
	go func() {
		slog.Info("relay http server starting", "port", cfg.Port)
		if err := router.Run(":" + cfg.Port); err != nil {
			slog.Error("relay http server error", "error", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down...")

	// Give outstanding requests time to complete
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_ = shutdownCtx // TODO: use for graceful HTTP shutdown when we add it

	database.Close()
	slog.Info("shutdown complete")
}

func setupLogger(cfg config.Config) {
	var handler slog.Handler
	if cfg.IsProduction() {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	}
	slog.SetDefault(slog.New(handler))
}

func setupRouter(database *db.DB, repos *repository.Repositories) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())

	// Health check - doesn't need auth
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// GitLab callback
	router.POST("/api/gitlab/callback", http.GitlabCallback)

	// Example: API routes would be added here
	// api := router.Group("/api/v1")
	// {
	//     api.Use(authMiddleware)
	//     api.GET("/users/:id", userHandler.Get)
	// }

	// Example: Using transaction in a handler
	// router.POST("/organizations", func(c *gin.Context) {
	//     err := database.WithTx(c.Request.Context(), func(q *sqlc.Queries) error {
	//         repos := repository.NewRepositories(q)
	//         // All operations in same transaction
	//         if err := repos.Organizations().Create(ctx, org); err != nil {
	//             return err
	//         }
	//         return repos.Workspaces().Create(ctx, defaultWorkspace)
	//     })
	//     if err != nil {
	//         c.JSON(500, gin.H{"error": err.Error()})
	//         return
	//     }
	//     c.JSON(201, gin.H{"status": "created"})
	// })

	_ = database // will be used by handlers
	_ = repos    // will be used by handlers

	return router
}
