package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"basegraph.co/relay/common/id"
	"basegraph.co/relay/common/logger"
	"basegraph.co/relay/common/otel"
	"basegraph.co/relay/core/config"
	"basegraph.co/relay/core/db"
	"basegraph.co/relay/internal/http/middleware"
	httprouter "basegraph.co/relay/internal/http/router"
	"basegraph.co/relay/internal/queue"
	"basegraph.co/relay/internal/service"
	"basegraph.co/relay/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func main() {
	fmt.Printf("%s\n", banner)
	ctx := context.Background()

	cfg, err := config.Load(config.ServiceTypeServer)
	if err != nil {
		slog.ErrorContext(ctx, "failed to load config", "error", err)
		os.Exit(1)
	}

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

	redisOpts, err := redis.ParseURL(cfg.Pipeline.RedisURL)
	if err != nil {
		slog.ErrorContext(ctx, "failed to parse redis url", "error", err)
		os.Exit(1)
	}

	redisClient := redis.NewClient(redisOpts)
	if err := redisClient.Ping(ctx).Err(); err != nil {
		slog.ErrorContext(ctx, "failed to connect to redis", "error", err)
		os.Exit(1)
	}
	slog.InfoContext(ctx, "redis connected", "stream", cfg.Pipeline.RedisStream)

	eventProducer := queue.NewRedisProducer(redisClient, cfg.Pipeline.RedisStream)
	defer eventProducer.Close()

	stores := store.NewStores(database.Queries())

	services := service.NewServices(service.ServicesConfig{
		Stores:        stores,
		TxRunner:      service.NewTxRunner(database),
		WorkOS:        cfg.WorkOS,
		DashboardURL:  cfg.DashboardURL,
		WebhookCfg:    cfg.EventWebhook,
		EventProducer: eventProducer,
	})

	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	router := setupRouter(cfg, services)
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
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

	httprouter.SetupRoutes(router, services, httprouter.RouterConfig{
		DashboardURL:    cfg.DashboardURL,
		IsProduction:    cfg.IsProduction(),
		TraceHeaderName: cfg.Pipeline.TraceHeaderName,
		AdminAPIKey:     cfg.AdminAPIKey,
	})

	return router
}

const banner = `
██████╗ ███████╗██╗      █████╗ ██╗   ██╗    ███████╗███████╗██████╗ ██╗   ██╗███████╗██████╗ 
██╔══██╗██╔════╝██║     ██╔══██╗╚██╗ ██╔╝    ██╔════╝██╔════╝██╔══██╗██║   ██║██╔════╝██╔══██╗
██████╔╝█████╗  ██║     ███████║ ╚████╔╝     ███████╗█████╗  ██████╔╝██║   ██║█████╗  ██████╔╝
██╔══██╗██╔══╝  ██║     ██╔══██║  ╚██╔╝      ╚════██║██╔══╝  ██╔══██╗╚██╗ ██╔╝██╔══╝  ██╔══██╗
██║  ██║███████╗███████╗██║  ██║   ██║       ███████║███████╗██║  ██║ ╚████╔╝ ███████╗██║  ██║
╚═╝  ╚═╝╚══════╝╚══════╝╚═╝  ╚═╝   ╚═╝       ╚══════╝╚══════╝╚═╝  ╚═╝  ╚═══╝  ╚══════╝╚═╝  ╚═╝
`
