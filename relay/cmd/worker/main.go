package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"basegraph.app/relay/common/id"
	"basegraph.app/relay/common/logger"
	"basegraph.app/relay/core/config"
	"basegraph.app/relay/core/db"
	"basegraph.app/relay/core/db/sqlc"
	"basegraph.app/relay/internal/queue"
	"basegraph.app/relay/internal/store"
	"basegraph.app/relay/internal/worker"
	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		slog.ErrorContext(ctx, "failed to load config", "error", err)
		os.Exit(1)
	}

	fmt.Printf("%s\n", banner)
	logger.Setup(cfg)

	slog.InfoContext(ctx, "relay worker starting",
		"env", cfg.Env,
		"consumer_group", cfg.Pipeline.RedisGroup,
		"consumer_name", cfg.Pipeline.RedisConsumer)

	// Initialize snowflake ID generator (use different node ID than server)
	if err := id.Init(2); err != nil {
		slog.ErrorContext(ctx, "failed to initialize id generator", "error", err)
		os.Exit(1)
	}

	// Initialize database
	database, err := db.New(ctx, cfg.DB)
	if err != nil {
		slog.ErrorContext(ctx, "failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer database.Close()
	slog.InfoContext(ctx, "database connected")

	// Initialize Redis
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
	defer redisClient.Close()
	slog.InfoContext(ctx, "redis connected", "stream", cfg.Pipeline.RedisStream)

	// Create consumer
	consumer, err := queue.NewRedisConsumer(redisClient, queue.ConsumerConfig{
		Stream:       cfg.Pipeline.RedisStream,
		Group:        cfg.Pipeline.RedisGroup,
		Consumer:     cfg.Pipeline.RedisConsumer,
		DLQStream:    cfg.Pipeline.RedisDLQStream,
		BatchSize:    1, // Process one issue at a time
		Block:        5 * time.Second,
		MaxAttempts:  3,
		RequeueDelay: time.Second,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to create consumer", "error", err)
		os.Exit(1)
	}

	// Create transaction runner adapter for worker
	txRunner := &workerTxRunnerAdapter{db: database}

	// Create processor (stub for now)
	processor := worker.NewStubProcessor()

	// Create worker
	w := worker.New(consumer, txRunner, processor, worker.Config{
		MaxAttempts: 3,
	})

	// Create reclaimer
	reclaimer := worker.NewReclaimer(redisClient, worker.ReclaimerConfig{
		Stream:    cfg.Pipeline.RedisStream,
		Group:     cfg.Pipeline.RedisGroup,
		Consumer:  cfg.Pipeline.RedisConsumer + "-reclaimer",
		MinIdle:   5 * time.Minute,
		Interval:  1 * time.Minute,
		BatchSize: 10,
	}, w.ProcessMessage)

	// Start worker and reclaimer
	errCh := make(chan error, 2)
	go func() {
		errCh <- w.Run(ctx)
	}()
	go func() {
		reclaimer.Run(ctx)
		errCh <- nil
	}()

	slog.InfoContext(ctx, "worker initialized and running")

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.InfoContext(ctx, "shutting down worker...")

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Stop reclaimer first (quick)
	reclaimer.Stop()

	// Stop worker (may be processing)
	w.Stop()

	// Wait for goroutines with timeout
	select {
	case <-shutdownCtx.Done():
		slog.WarnContext(ctx, "shutdown timeout exceeded")
	case err := <-errCh:
		if err != nil {
			slog.ErrorContext(ctx, "worker error during shutdown", "error", err)
		}
	}

	slog.InfoContext(ctx, "worker shutdown complete")
}

// workerTxRunnerAdapter bridges db.DB to worker.TxRunner.
type workerTxRunnerAdapter struct {
	db *db.DB
}

func (a *workerTxRunnerAdapter) WithTx(ctx context.Context, fn func(stores worker.StoreProvider) error) error {
	return a.db.WithTx(ctx, func(q *sqlc.Queries) error {
		stores := store.NewStores(q)
		return fn(stores)
	})
}

const banner = `
██████╗ ███████╗██╗      █████╗ ██╗   ██╗    ██████╗ ██╗██████╗ ███████╗██╗     ██╗███╗   ██╗███████╗
██╔══██╗██╔════╝██║     ██╔══██╗╚██╗ ██╔╝    ██╔══██╗██║██╔══██╗██╔════╝██║     ██║████╗  ██║██╔════╝
██████╔╝█████╗  ██║     ███████║ ╚████╔╝     ██████╔╝██║██████╔╝█████╗  ██║     ██║██╔██╗ ██║█████╗
██╔══██╗██╔══╝  ██║     ██╔══██║  ╚██╔╝      ██╔═══╝ ██║██╔═══╝ ██╔══╝  ██║     ██║██║╚██╗██║██╔══╝
██║  ██║███████╗███████╗██║  ██║   ██║       ██║     ██║██║     ███████╗███████╗██║██║ ╚████║███████╗
╚═╝  ╚═╝╚══════╝╚══════╝╚═╝  ╚═╝   ╚═╝       ╚═╝     ╚═╝╚═╝     ╚══════╝╚══════╝╚═╝╚═╝  ╚═══╝╚══════╝
`
