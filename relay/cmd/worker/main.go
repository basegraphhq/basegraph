package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"os/exec"
	"runtime/debug"
	"strconv"
	"sync"
	"syscall"
	"time"

	"basegraph.co/relay/common/arangodb"
	"basegraph.co/relay/common/id"
	"basegraph.co/relay/common/llm"
	"basegraph.co/relay/common/logger"
	"basegraph.co/relay/core/config"
	"basegraph.co/relay/core/db"
	"basegraph.co/relay/internal/brain"
	"basegraph.co/relay/internal/model"
	"basegraph.co/relay/internal/queue"
	"basegraph.co/relay/internal/service/issue_tracker"
	"basegraph.co/relay/internal/store"
	"basegraph.co/relay/internal/worker"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/trace"
)

const maxAttempts = 3

func main() {
	ctx := context.Background()

	cfg, err := config.Load(config.ServiceTypeWorker)
	if err != nil {
		slog.ErrorContext(ctx, "failed to load config", "error", err)
		os.Exit(1)
	}

	fmt.Printf("%s\n", banner)
	logger.Setup(cfg)

	if err := checkExternalDependencies(ctx); err != nil {
		slog.ErrorContext(ctx, "missing required worker dependencies", "error", err)
		os.Exit(1)
	}

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

	if !cfg.PlannerLLM.Enabled() {
		slog.ErrorContext(ctx, "PLANNER_LLM_API_KEY is required for pipeline processing")
		os.Exit(1)
	}

	plannerClient, err := llm.NewAgentClient(llm.Config{
		Provider:        cfg.PlannerLLM.Provider,
		APIKey:          cfg.PlannerLLM.APIKey,
		BaseURL:         cfg.PlannerLLM.BaseURL,
		Model:           cfg.PlannerLLM.Model,
		ReasoningEffort: llm.ReasoningEffort(cfg.PlannerLLM.ReasoningEffort),
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to create planner client", "error", err)
		os.Exit(1)
	}
	slog.InfoContext(ctx, "planner client initialized",
		"provider", cfg.PlannerLLM.Provider,
		"model", cfg.PlannerLLM.Model,
		"reasoning_effort", cfg.PlannerLLM.ReasoningEffort)

	if !cfg.ExploreLLM.Enabled() {
		slog.ErrorContext(ctx, "EXPLORE_LLM_API_KEY is required for pipeline processing")
		os.Exit(1)
	}

	exploreClient, err := llm.NewAgentClient(llm.Config{
		Provider:        cfg.ExploreLLM.Provider,
		APIKey:          cfg.ExploreLLM.APIKey,
		BaseURL:         cfg.ExploreLLM.BaseURL,
		Model:           cfg.ExploreLLM.Model,
		ReasoningEffort: llm.ReasoningEffort(cfg.ExploreLLM.ReasoningEffort),
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to create explore client", "error", err)
		os.Exit(1)
	}
	slog.InfoContext(ctx, "explore client initialized",
		"provider", cfg.ExploreLLM.Provider,
		"model", cfg.ExploreLLM.Model)

	if !cfg.SpecGeneratorLLM.Enabled() {
		slog.ErrorContext(ctx, "SPEC_GENERATOR_LLM_API_KEY is required for pipeline processing")
		os.Exit(1)
	}

	specGeneratorClient, err := llm.NewAgentClient(llm.Config{
		Provider:        cfg.SpecGeneratorLLM.Provider,
		APIKey:          cfg.SpecGeneratorLLM.APIKey,
		BaseURL:         cfg.SpecGeneratorLLM.BaseURL,
		Model:           cfg.SpecGeneratorLLM.Model,
		ReasoningEffort: llm.ReasoningEffort(cfg.SpecGeneratorLLM.ReasoningEffort),
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to create spec generator client", "error", err)
		os.Exit(1)
	}
	slog.InfoContext(ctx, "spec generator client initialized",
		"provider", cfg.SpecGeneratorLLM.Provider,
		"model", cfg.SpecGeneratorLLM.Model,
		"reasoning_effort", cfg.SpecGeneratorLLM.ReasoningEffort)

	workspaceIDStr := os.Getenv("WORKSPACE_ID")
	if workspaceIDStr == "" {
		slog.ErrorContext(ctx, "WORKSPACE_ID environment variable is required")
		os.Exit(1)
	}
	workspaceID, err := strconv.ParseInt(workspaceIDStr, 10, 64)
	if err != nil {
		slog.ErrorContext(ctx, "invalid WORKSPACE_ID", "error", err)
		os.Exit(1)
	}

	stores := store.NewStores(database.Queries())

	taskRunner, err := worker.NewTaskRunner(ctx, worker.TaskRunnerConfig{
		WorkspaceID: workspaceID,
		DataDir:     os.Getenv("DATA_DIR"),
		Stores:      stores,
		Redis:       redisClient,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to initialize workspace task runner", "error", err)
		os.Exit(1)
	}

	repoRoot := taskRunner.RepoRoot()
	modulePath := os.Getenv("MODULE_PATH")
	if modulePath == "" {
		slog.InfoContext(ctx, "MODULE_PATH not set; continuing without module path")
	}

	org := taskRunner.Organization()
	workspace := taskRunner.Workspace()
	if org != nil && workspace != nil {
		slog.InfoContext(ctx, "workspace context loaded",
			"workspace_id", workspace.ID,
			"workspace_slug", workspace.Slug,
			"org_id", org.ID,
			"org_slug", org.Slug,
			"repo_root", repoRoot)
	}

	if repos, err := stores.Repos().ListEnabledByWorkspace(ctx, workspaceID); err == nil {
		repoNames := make([]string, 0, len(repos))
		for _, repo := range repos {
			repoNames = append(repoNames, repo.Slug)
		}
		slog.InfoContext(ctx, "enabled repositories loaded",
			"count", len(repoNames),
			"repos", repoNames)
	} else {
		slog.WarnContext(ctx, "failed to list enabled repositories", "error", err)
	}

	var arangoClient arangodb.Client
	if cfg.ArangoDB.Enabled() {
		arangoClient, err = arangodb.New(ctx, arangodb.Config{
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
	} else {
		slog.InfoContext(ctx, "arangodb disabled; codegraph unavailable")
	}

	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		slog.ErrorContext(ctx, "failed to create repo root", "error", err, "path", repoRoot)
		os.Exit(1)
	}

	txRunner := brain.NewTxRunner(database)

	issueTrackers := map[model.Provider]issue_tracker.IssueTrackerService{
		model.ProviderGitLab: issue_tracker.NewGitLabIssueTrackerService(
			stores.Integrations(),
			stores.IntegrationCredentials(),
		),
	}

	// TODO(cleanup): Remove DebugDir once product goes live.
	// It creates debug_logs/YYYY-MM-DD/NNN/ folders for each worker run.
	// Related: brain.SetupDebugRunDir, Planner.debugDir, ExploreAgent.debugDir
	orchestratorCfg := brain.OrchestratorConfig{
		RepoRoot:            repoRoot,
		ModulePath:          modulePath,
		DebugDir:            os.Getenv("BRAIN_DEBUG_DIR"),
		SpecGeneratorClient: specGeneratorClient,
	}

	// Mock explore mode for A/B testing planner prompts
	// Set MOCK_EXPLORE_FIXTURES to enable (e.g., "evals/fixtures/explore.json")
	mockFixtureFile := os.Getenv("MOCK_EXPLORE_FIXTURES")
	if mockFixtureFile != "" {
		mockAPIKey := os.Getenv("MOCK_EXPLORE_KEY")
		if mockAPIKey == "" {
			mockAPIKey = cfg.PlannerLLM.APIKey // Fall back to PLANNER_LLM_API_KEY
		}
		mockModel := os.Getenv("MOCK_EXPLORE_MODEL")
		if mockModel == "" {
			mockModel = "grok-4-1-fast-reasoning"
		}

		mockLLM, err := llm.NewAgentClient(llm.Config{
			Provider: "openai",
			APIKey:   mockAPIKey,
			BaseURL:  os.Getenv("MOCK_EXPLORE_BASE_URL"),
			Model:    mockModel,
		})
		if err != nil {
			slog.ErrorContext(ctx, "failed to create mock explore LLM client", "error", err)
			os.Exit(1)
		}

		orchestratorCfg.MockExploreEnabled = true
		orchestratorCfg.MockExploreLLM = mockLLM
		orchestratorCfg.MockFixtureFile = mockFixtureFile

		slog.InfoContext(ctx, "mock explore mode enabled",
			"fixture_file", mockFixtureFile,
			"selector_model", mockModel)
	}

	orchestrator := brain.NewOrchestrator(
		orchestratorCfg,
		plannerClient,
		exploreClient,
		arangoClient,
		txRunner,
		stores.Issues(),
		stores.Gaps(),
		stores.EventLogs(),
		producer,
		stores.Integrations(),
		stores.IntegrationConfigs(),
		stores.Learnings(),
		issueTrackers,
	)

	processMessage := newMessageProcessor(consumer, orchestrator, taskRunner)

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
	if arangoClient != nil {
		if err := arangoClient.Close(); err != nil {
			slog.ErrorContext(ctx, "arangodb close error", "error", err)
		}
	}

	slog.InfoContext(ctx, "shutdown complete")
}

func checkExternalDependencies(ctx context.Context) error {
	ctx = logger.WithLogFields(ctx, logger.LogFields{
		Component: "relay.worker.deps",
	})

	type dep struct {
		name string
		args []string
	}

	deps := []dep{
		{name: "git", args: []string{"--version"}},
		{name: "ssh", args: []string{"-V"}},
		{name: "ssh-keygen"},
		{name: "ssh-keyscan"},
		{name: "rg", args: []string{"--version"}},
	}

	for _, d := range deps {
		if _, err := exec.LookPath(d.name); err != nil {
			return fmt.Errorf("%s not found in PATH: %w", d.name, err)
		}

		if len(d.args) == 0 {
			slog.InfoContext(ctx, "dependency available", "name", d.name)
			continue
		}

		// Best-effort version logging (some commands output version to stderr).
		cmd := exec.CommandContext(ctx, d.name, d.args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			slog.WarnContext(ctx, "dependency check command failed",
				"name", d.name,
				"error", err,
				"output", string(out))
			continue
		}
		if len(out) > 0 {
			slog.InfoContext(ctx, "dependency available", "name", d.name, "output", string(out))
		} else {
			slog.InfoContext(ctx, "dependency available", "name", d.name)
		}
	}

	return nil
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
		IssueID:    msg.IssueID,
		EventLogID: msg.EventLogID,
		MessageID:  &msg.ID,
		EventType:  &msg.EventType,
		WorkspaceID: msg.WorkspaceID,
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

func newMessageProcessor(consumer *queue.RedisConsumer, orchestrator *brain.Orchestrator, taskRunner *worker.TaskRunner) queue.MessageProcessor {
	return func(ctx context.Context, msg queue.Message) error {
		slog.InfoContext(ctx, "processing message",
			"task_type", msg.TaskType,
			"attempt", msg.Attempt)

		switch msg.TaskType {
		case queue.TaskTypeIssueEvent:
			if msg.IssueID == nil || msg.EventLogID == nil {
				return fmt.Errorf("missing issue fields")
			}

			ready, reason, err := taskRunner.EnsureIssueReady(ctx, *msg.IssueID)
			if err != nil {
				return err
			}
			if !ready {
				if err := consumer.RequeueWithAttempt(ctx, msg, msg.Attempt, reason); err != nil {
					return err
				}
				return nil
			}

			input := brain.EngagementInput{
				IssueID:         *msg.IssueID,
				EventLogID:      *msg.EventLogID,
				EventType:       msg.EventType,
				TriggerThreadID: msg.TriggerThreadID,
			}

			if err := orchestrator.HandleEngagement(ctx, input); err != nil {
				return err
			}
		case queue.TaskTypeWorkspaceSetup:
			if msg.RunID == nil {
				return fmt.Errorf("missing run_id")
			}
			if err := taskRunner.HandleWorkspaceSetup(ctx, *msg.RunID); err != nil {
				return err
			}
		case queue.TaskTypeRepoSync:
			if msg.RunID == nil || msg.RepoID == nil {
				return fmt.Errorf("missing run_id or repo_id")
			}
			if err := taskRunner.HandleRepoSync(ctx, *msg.RunID, *msg.RepoID, msg.Branch); err != nil {
				if errors.Is(err, worker.ErrRepoNotReady) || errors.Is(err, worker.ErrDefaultBranchRequired) {
					if err := consumer.RequeueWithAttempt(ctx, msg, msg.Attempt, err.Error()); err != nil {
						return err
					}
					return nil
				}
				return err
			}
		default:
			return fmt.Errorf("unsupported task_type: %s", msg.TaskType)
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
