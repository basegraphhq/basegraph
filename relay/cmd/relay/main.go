package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"basegraph.app/relay/common/id"
	"basegraph.app/relay/common/logger"
	"basegraph.app/relay/common/otel"
	"basegraph.app/relay/core/config"
	"basegraph.app/relay/core/db"
	"basegraph.app/relay/internal/http/middleware"
	httprouter "basegraph.app/relay/internal/http/router"
	"basegraph.app/relay/internal/service"
	"basegraph.app/relay/internal/store"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func main() {
	ctx := context.Background()

	cfg := config.Load()

	// OTel must init before logger (logger uses OTel provider in production)
	telemetry, err := otel.Setup(ctx, cfg.OTel)
	if err != nil {
		// Can't use slog yet — OTel failed before logger setup
		os.Stderr.WriteString("failed to initialize otel: " + err.Error() + "\n")
		os.Exit(1)
	}

	logger.Setup(cfg)

	if telemetry != nil {
		slog.InfoContext(ctx, "otel initialized", "endpoint", cfg.OTel.Endpoint)
	} else {
		slog.InfoContext(ctx, "otel disabled (no endpoint configured)")
	}

	slog.InfoContext(ctx, "relay starting", "env", cfg.Env, "service", cfg.OTel.ServiceName)

	if err := id.Init(1); err != nil {
		slog.ErrorContext(ctx, "failed to initialize snowflake id generator", "error", err)
		os.Exit(1)
	}

	database, err := db.New(ctx, cfg.DB)
	if err != nil {
		slog.ErrorContext(ctx, "failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer database.Close()
	slog.InfoContext(ctx, "database connected")

	stores := store.NewStores(database.Queries())
	services := service.NewServices(stores, service.NewTxRunner(database))

	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	router := setupRouter(cfg, services)
	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	go func() {
		slog.InfoContext(ctx, "http server starting", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.ErrorContext(ctx, "http server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.InfoContext(ctx, "shutting down...")

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.ErrorContext(shutdownCtx, "http server shutdown error", "error", err)
	}

	if telemetry != nil {
		if err := telemetry.Shutdown(shutdownCtx); err != nil {
			slog.ErrorContext(shutdownCtx, "otel shutdown error", "error", err)
		}
	}

	slog.InfoContext(shutdownCtx, "shutdown complete")
}

func setupRouter(cfg config.Config, services *service.Services) *gin.Engine {
	router := gin.New()

	// Order matters: OTel creates span → Recovery catches panics → Logger logs with trace context
	if cfg.OTel.Enabled() {
		router.Use(otelgin.Middleware(cfg.OTel.ServiceName))
	}
	router.Use(middleware.Recovery())
	router.Use(middleware.Logger())

	httprouter.SetupRoutes(router, services)

	return router
}
