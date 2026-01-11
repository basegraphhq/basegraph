package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	"basegraph.app/relay/common/arangodb"
	"basegraph.app/relay/common/id"
	"basegraph.app/relay/common/llm"
	"basegraph.app/relay/common/logger"
	"basegraph.app/relay/core/config"
	"basegraph.app/relay/core/db"
	"basegraph.app/relay/internal/brain"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/queue"
	"basegraph.app/relay/internal/service/issue_tracker"
	"basegraph.app/relay/internal/store"
	"basegraph.app/relay/internal/worker"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/trace"
)

const maxAttempts = 3

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

	if err := id.Init(2); err != nil {
		slog.ErrorContext(ctx, "failed to initialize id generator", "error", err)
		os.Exit(1)
	}

	database, err := db.New(ctx, cfg.DB)
	if err != nil {
		slog.ErrorContext(ctx, "failed to connect to database", "error", err)
		os.Exit(1)
	}
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

	consumer, err := queue.NewRedisConsumer(redisClient, queue.ConsumerConfig{
		Stream:       cfg.Pipeline.RedisStream,
		Group:        cfg.Pipeline.RedisGroup,
		Consumer:     cfg.Pipeline.RedisConsumer,
		DLQStream:    cfg.Pipeline.RedisDLQStream,
		BatchSize:    1,
		Block:        5 * time.Second,
		MaxAttempts:  3,
		RequeueDelay: time.Second,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to create consumer", "error", err)
		os.Exit(1)
	}

	producer := queue.NewRedisProducer(redisClient, cfg.Pipeline.RedisStream)

	if !cfg.LLM.Enabled() {
		slog.ErrorContext(ctx, "LLM_API_KEY is required for pipeline processing")
		os.Exit(1)
	}

	agentClient, err := llm.NewAgentClient(llm.Config{
		Provider:        cfg.LLM.Provider,
		APIKey:          cfg.LLM.APIKey,
		BaseURL:         cfg.LLM.BaseURL,
		Model:           cfg.LLM.Model,
		ReasoningEffort: llm.ReasoningEffort(cfg.LLM.ReasoningEffort),
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to create agent client", "error", err)
		os.Exit(1)
	}
	slog.InfoContext(ctx, "agent client initialized",
		"provider", cfg.LLM.Provider,
		"model", cfg.LLM.Model,
		"reasoning_effort", cfg.LLM.ReasoningEffort)

	repoRoot := os.Getenv("REPO_ROOT")
	if repoRoot == "" {
		slog.ErrorContext(ctx, "REPO_ROOT environment variable is required")
		os.Exit(1)
	}

	modulePath := os.Getenv("MODULE_PATH")
	if modulePath == "" {
		slog.ErrorContext(ctx, "MODULE_PATH environment variable is required")
		os.Exit(1)
	}
	slog.InfoContext(ctx, "repo configured", "root", repoRoot, "module", modulePath)

	if !cfg.ArangoDB.Enabled() {
		slog.ErrorContext(ctx, "ArangoDB configuration is required")
		os.Exit(1)
	}
	arangoClient, err := arangodb.New(ctx, arangodb.Config{
		URL:      cfg.ArangoDB.URL,
		Username: cfg.ArangoDB.Username,
		Password: cfg.ArangoDB.Password,
		Database: cfg.ArangoDB.Database,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to create ArangoDB client", "error", err)
		os.Exit(1)
	}
	if err := arangoClient.EnsureDatabase(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to ensure ArangoDB database", "error", err)
		os.Exit(1)
	}
	slog.InfoContext(ctx, "arangodb connected", "database", cfg.ArangoDB.Database)

	stores := store.NewStores(database.Queries())

	specStore, err := store.NewLocalSpecStore(cfg.Spec.RootDir)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create spec store", "error", err)
		os.Exit(1)
	}
	slog.InfoContext(ctx, "spec store initialized", "root_dir", cfg.Spec.RootDir)

	issueTrackers := map[model.Provider]issue_tracker.IssueTrackerService{
		model.ProviderGitLab: issue_tracker.NewGitLabIssueTrackerService(
			stores.Integrations(),
			stores.IntegrationCredentials(),
		),
	}

	// TODO(cleanup): Remove DebugDir once product goes live.
	// It creates debug_logs/YYYY-MM-DD/NNN/ folders for each worker run.
	// Related: brain.SetupDebugRunDir, Planner.debugDir, ExploreAgent.debugDir
	orchestrator := brain.NewOrchestrator(
		brain.OrchestratorConfig{
			RepoRoot:   repoRoot,
			ModulePath: modulePath,
			DebugDir:   os.Getenv("BRAIN_DEBUG_DIR"),
		},
		agentClient,
		arangoClient,
		stores.Issues(),
		stores.Gaps(),
		stores.EventLogs(),
		producer,
		stores.Integrations(),
		stores.IntegrationConfigs(),
		stores.Learnings(),
		specStore,
		issueTrackers,
	)

	processMessage := newMessageProcessor(consumer, orchestrator)

	reclaimer := worker.NewRedisReclaimer(redisClient, worker.RedisReclaimerConfig{
		Stream:    cfg.Pipeline.RedisStream,
		Group:     cfg.Pipeline.RedisGroup,
		Consumer:  cfg.Pipeline.RedisConsumer + "-reclaimer",
		MinIdle:   5 * time.Minute,
		Interval:  1 * time.Minute,
		BatchSize: 10,
	}, consumer, processMessage)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go reclaimer.Run(ctx)
	go runLoop(ctx, &wg, consumer, processMessage)

	slog.InfoContext(ctx, "worker running")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.InfoContext(ctx, "shutdown signal received, initiating graceful shutdown...")

	// Cancel context to signal all goroutines to stop
	cancel()

	// Wait for in-flight work to complete with timeout
	shutdownComplete := make(chan struct{})
	go func() {
		reclaimer.Stop()
		wg.Wait()
		close(shutdownComplete)
	}()

	shutdownTimeout := 30 * time.Second
	select {
	case <-shutdownComplete:
		slog.InfoContext(ctx, "graceful shutdown completed")
	case <-time.After(shutdownTimeout):
		slog.WarnContext(ctx, "shutdown timeout exceeded, forcing exit", "timeout", shutdownTimeout)
	}

	// Close connections explicitly (defers will also run, but explicit close provides better logging)
	slog.InfoContext(ctx, "closing database connection")
	database.Close()

	slog.InfoContext(ctx, "closing redis connection")
	if err := redisClient.Close(); err != nil {
		slog.ErrorContext(ctx, "redis close error", "error", err)
	}

	slog.InfoContext(ctx, "closing arangodb connection")
	if err := arangoClient.Close(); err != nil {
		slog.ErrorContext(ctx, "arangodb close error", "error", err)
	}

	slog.InfoContext(ctx, "shutdown complete")
}

func runLoop(ctx context.Context, wg *sync.WaitGroup, consumer *queue.RedisConsumer, process queue.MessageProcessor) {
	defer wg.Done()

	ctx = logger.WithLogFields(ctx, logger.LogFields{
		Component: "relay.worker.loop",
	})

	slog.InfoContext(ctx, "worker loop started")

	for {
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, "worker loop stopping")
			return
		default:
			messages, err := consumer.Read(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				slog.ErrorContext(ctx, "failed to read from stream", "error", err)
				time.Sleep(time.Second)
				continue
			}

			for _, msg := range messages {
				// Check for shutdown before processing each message
				if ctx.Err() != nil {
					slog.InfoContext(ctx, "shutdown requested, stopping message processing")
					return
				}

				// Create message-specific context with trace propagation
				msgCtx, endSpan := createMessageContext(ctx, msg)

				err := processMessageSafe(msgCtx, msg, process)
				endSpan()

				if err != nil {
					slog.ErrorContext(msgCtx, "message processing failed", "error", err)
					handleFailure(msgCtx, consumer, msg, err)
				}
			}
		}
	}
}

// createMessageContext creates a context enriched with message metadata and trace propagation.
// Returns the context and a function to end the span.
func createMessageContext(ctx context.Context, msg queue.Message) (context.Context, func()) {
	// Start a new span linked to the original trace from the API server
	sc := logger.StartSpanFromTraceID(ctx, msg.TraceID, "worker.process_message",
		trace.WithSpanKind(trace.SpanKindConsumer))

	ctx = sc.Context()

	// Enrich context with message metadata for automatic logging
	ctx = logger.WithLogFields(ctx, logger.LogFields{
		IssueID:    &msg.IssueID,
		EventLogID: &msg.EventLogID,
		MessageID:  &msg.ID,
		EventType:  &msg.EventType,
		Component:  "relay.worker.processor",
	})

	return ctx, sc.End
}

func processMessageSafe(ctx context.Context, msg queue.Message, process queue.MessageProcessor) (err error) {
	start := time.Now()
	span := trace.SpanFromContext(ctx)

	defer func() {
		duration := time.Since(start)

		if rec := recover(); rec != nil {
			if span.SpanContext().IsValid() {
				span.RecordError(fmt.Errorf("panic: %v", rec))
			}
			slog.ErrorContext(ctx, "panic recovered",
				"panic", rec,
				"stack", string(debug.Stack()),
				"duration_ms", duration.Milliseconds())
			err = fmt.Errorf("panic: %v", rec)
			return
		}

		if err == nil {
			slog.InfoContext(ctx, "message processed successfully",
				"duration_ms", duration.Milliseconds())
		}
	}()

	return process(ctx, msg)
}

func newMessageProcessor(consumer *queue.RedisConsumer, orchestrator *brain.Orchestrator) queue.MessageProcessor {
	return func(ctx context.Context, msg queue.Message) error {
		slog.InfoContext(ctx, "processing message",
			"attempt", msg.Attempt)

		input := brain.EngagementInput{
			IssueID:         msg.IssueID,
			EventLogID:      msg.EventLogID,
			EventType:       msg.EventType,
			TriggerThreadID: msg.TriggerThreadID,
		}

		if err := orchestrator.HandleEngagement(ctx, input); err != nil {
			return err
		}

		if err := consumer.Ack(ctx, msg); err != nil {
			slog.WarnContext(ctx, "failed to ack message", "error", err)
		}

		return nil
	}
}

func handleFailure(ctx context.Context, consumer *queue.RedisConsumer, msg queue.Message, err error) {
	var engErr *brain.EngagementError
	retryable := true
	if errors.As(err, &engErr) {
		retryable = engErr.Retryable
	}

	willRequeue := retryable && msg.Attempt < maxAttempts
	willDLQ := !retryable || msg.Attempt >= maxAttempts

	slog.InfoContext(ctx, "handling message failure",
		"error", err,
		"error_type", fmt.Sprintf("%T", err),
		"retryable", retryable,
		"attempt", msg.Attempt,
		"max_attempts", maxAttempts,
		"will_requeue", willRequeue,
		"will_dlq", willDLQ)

	if willDLQ {
		if dlqErr := consumer.SendDLQ(ctx, msg, err.Error()); dlqErr != nil {
			slog.ErrorContext(ctx, "failed to send to DLQ", "error", dlqErr)
		}
		return
	}

	if requeueErr := consumer.Requeue(ctx, msg, err.Error()); requeueErr != nil {
		slog.ErrorContext(ctx, "failed to requeue", "error", requeueErr)
	}
}

const banner = `
 ███████████   ██████████ █████         █████████   █████ █████    ███████████  ███████████     █████████   █████ ██████   █████
░░███░░░░░███ ░░███░░░░░█░░███         ███░░░░░███ ░░███ ░░███    ░░███░░░░░███░░███░░░░░███   ███░░░░░███ ░░███ ░░██████ ░░███ 
 ░███    ░███  ░███  █ ░  ░███        ░███    ░███  ░░███ ███      ░███    ░███ ░███    ░███  ░███    ░███  ░███  ░███░███ ░███ 
 ░██████████   ░██████    ░███        ░███████████   ░░█████       ░██████████  ░██████████   ░███████████  ░███  ░███░░███░███ 
 ░███░░░░░███  ░███░░█    ░███        ░███░░░░░███    ░░███        ░███░░░░░███ ░███░░░░░███  ░███░░░░░███  ░███  ░███ ░░██████ 
 ░███    ░███  ░███ ░   █ ░███      █ ░███    ░███     ░███        ░███    ░███ ░███    ░███  ░███    ░███  ░███  ░███  ░░█████ 
 █████   █████ ██████████ ███████████ █████   █████    █████       ███████████  █████   █████ █████   █████ █████ █████  ░░█████
░░░░░   ░░░░░ ░░░░░░░░░░ ░░░░░░░░░░░ ░░░░░   ░░░░░    ░░░░░       ░░░░░░░░░░░  ░░░░░   ░░░░░ ░░░░░   ░░░░░ ░░░░░ ░░░░░    ░░░░░ 
`
