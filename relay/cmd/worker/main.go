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
	"basegraph.app/relay/common/llm"
	"basegraph.app/relay/common/logger"
	"basegraph.app/relay/core/config"
	"basegraph.app/relay/core/db"
	"basegraph.app/relay/internal/queue"
	"basegraph.app/relay/internal/service"
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
	defer redisClient.Close()
	slog.InfoContext(ctx, "redis connected", "stream", cfg.Pipeline.RedisStream)

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

	txRunner := &workerTxRunnerAdapter{tx: service.NewTxRunner(database)}

	if !cfg.OpenAI.Enabled() {
		slog.ErrorContext(ctx, "OPENAI_API_KEY is required for pipeline processing")
		os.Exit(1)
	}

	llmClient, err := llm.New(llm.Config{
		APIKey:  cfg.OpenAI.APIKey,
		BaseURL: cfg.OpenAI.BaseURL,
		Model:   cfg.OpenAI.Model,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to create LLM client", "error", err)
		os.Exit(1)
	}

	slog.InfoContext(ctx, "llm client initialized", "model", cfg.OpenAI.Model)

	stores := store.NewStores(database.Queries())
	processor := worker.NewProcessor(llmClient, stores.LLMEvals())

	// MaxAttempts=1: DLQ is a safety valve, not a retry mechanism. Poison messages go to DLQ immediately.
	w := worker.New(consumer, txRunner, stores.Issues(), processor, worker.Config{
		MaxAttempts: 1,
	})

	// Redis reclaimer handles unACK'd messages from crashed workers.
	reclaimer := worker.NewRedisReclaimer(redisClient, worker.RedisReclaimerConfig{
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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.InfoContext(ctx, "shutting down worker...")

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

// workerTxRunnerAdapter bridges service.TxRunner to worker.TxRunner.
type workerTxRunnerAdapter struct {
	tx service.TxRunner
}

func (a *workerTxRunnerAdapter) WithTx(ctx context.Context, fn func(stores worker.StoreProvider) error) error {
	return a.tx.WithTx(ctx, func(sp service.StoreProvider) error {
		s, ok := sp.(*store.Stores)
		if !ok {
			return fmt.Errorf("unexpected store provider type %T", sp)
		}
		return fn(s)
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
