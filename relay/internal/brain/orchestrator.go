package brain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"basegraph.co/relay/common/arangodb"
	"basegraph.co/relay/common/llm"
	"basegraph.co/relay/common/logger"
	"basegraph.co/relay/internal/model"
	"basegraph.co/relay/internal/queue"
	"basegraph.co/relay/internal/service/issue_tracker"
	"basegraph.co/relay/internal/store"
)

var ErrIssueNotFound = errors.New("issue not found")

const maxValidationRetries = 2 // Allow 2 retries after initial attempt (3 total)

type EngagementInput struct {
	IssueID         int64
	EventLogID      int64
	EventType       string
	TriggerThreadID string
}

type EngagementError struct {
	Err       error
	Retryable bool
}

func (e *EngagementError) Error() string {
	return e.Err.Error()
}

func (e *EngagementError) Unwrap() error {
	return e.Err
}

func NewRetryableError(err error) *EngagementError {
	return &EngagementError{Err: err, Retryable: true}
}

func NewFatalError(err error) *EngagementError {
	return &EngagementError{Err: err, Retryable: false}
}

type OrchestratorConfig struct {
	RepoRoot   string
	ModulePath string
	DebugDir   string // Base directory for debug logs (empty = no logging)

	// Mock explore mode for A/B testing planner prompts
	MockExploreEnabled bool            // Enable mock explore mode
	MockExploreLLM     llm.AgentClient // Cheap LLM for fixture selection (e.g., gpt-4o-mini)
	MockFixtureFile    string          // Path to JSON file with pre-written explore responses

	// Spec generator LLM (required)
	SpecGeneratorClient llm.AgentClient
}

// SetupDebugRunDir creates a new debug run directory under baseDir/YYYY-MM-DD/NNN.
// Returns the path to the new run directory, or empty string if baseDir is empty.
// This is a development feature - remove once product goes live.
func SetupDebugRunDir(baseDir string) string {
	if baseDir == "" {
		return ""
	}

	date := time.Now().Format("2006-01-02")
	dateDir := filepath.Join(baseDir, date)

	if err := os.MkdirAll(dateDir, 0o755); err != nil {
		slog.Warn("failed to create debug date dir", "dir", dateDir, "error", err)
		return baseDir // fallback to base
	}

	// Find next run number
	runNum := 1
	entries, err := os.ReadDir(dateDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				if n, err := strconv.Atoi(e.Name()); err == nil && n >= runNum {
					runNum = n + 1
				}
			}
		}
	}

	runDir := filepath.Join(dateDir, fmt.Sprintf("%03d", runNum))
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		slog.Warn("failed to create debug run dir", "dir", runDir, "error", err)
		return dateDir // fallback to date dir
	}

	slog.Info("debug run directory created", "path", runDir)
	return runDir
}

type Orchestrator struct {
	cfg             OrchestratorConfig
	planner         *Planner
	specGenerator   *SpecGenerator
	contextBuilder  *contextBuilder
	actionValidator ActionValidator
	txRunner        TxRunner
	issues          store.IssueStore
	gaps            store.GapStore
	integrations    store.IntegrationStore
	learnings       store.LearningStore
	eventLogs       store.EventLogStore
	queueProducer   queue.Producer
	issueTrackers   map[model.Provider]issue_tracker.IssueTrackerService
}

func NewOrchestrator(
	cfg OrchestratorConfig,
	plannerClient llm.AgentClient,
	exploreClient llm.AgentClient,
	arango arangodb.Client,
	txRunner TxRunner,
	issues store.IssueStore,
	gaps store.GapStore,
	eventLogs store.EventLogStore,
	queueProducer queue.Producer,
	integrations store.IntegrationStore,
	configs store.IntegrationConfigStore,
	learnings store.LearningStore,
	issueTrackers map[model.Provider]issue_tracker.IssueTrackerService,
) *Orchestrator {
	debugDir := SetupDebugRunDir(cfg.DebugDir)

	tools := NewExploreTools(cfg.RepoRoot, arango)
	explore := NewExploreAgent(exploreClient, tools, cfg.ModulePath, debugDir)

	// Enable mock explore mode if configured (for A/B testing planner prompts)
	if cfg.MockExploreEnabled && cfg.MockExploreLLM != nil && cfg.MockFixtureFile != "" {
		explore = explore.WithMockMode(cfg.MockExploreLLM, cfg.MockFixtureFile)
		slog.InfoContext(context.Background(), "mock explore mode enabled",
			"fixture_file", cfg.MockFixtureFile)
	}

	planner := NewPlanner(plannerClient, explore, debugDir)
	ctxBuilder := NewContextBuilder(integrations, configs, learnings, gaps)
	validator := NewActionValidator(gaps)

	// Create spec generator (required)
	specGen := NewSpecGenerator(cfg.SpecGeneratorClient, explore, debugDir)
	slog.InfoContext(context.Background(), "spec generator enabled",
		"model", cfg.SpecGeneratorClient.Model())

	slog.InfoContext(context.Background(), "orchestrator initialized",
		"repo_root", cfg.RepoRoot,
		"module_path", cfg.ModulePath)

	return &Orchestrator{
		cfg:             cfg,
		planner:         planner,
		specGenerator:   specGen,
		contextBuilder:  ctxBuilder,
		actionValidator: validator,
		txRunner:        txRunner,
		issues:          issues,
		gaps:            gaps,
		integrations:    integrations,
		learnings:       learnings,
		eventLogs:       eventLogs,
		queueProducer:   queueProducer,
		issueTrackers:   issueTrackers,
	}
}

func (o *Orchestrator) HandleEngagement(ctx context.Context, input EngagementInput) error {
	ctx = logger.WithLogFields(ctx, logger.LogFields{
		IssueID:    &input.IssueID,
		EventLogID: &input.EventLogID,
		EventType:  &input.EventType,
		Component:  "relay.brain.orchestrator",
	})

	slog.InfoContext(ctx, "handling engagement")

	issue, err := o.issues.GetByID(ctx, input.IssueID)
	if err != nil {
		if err == store.ErrNotFound {
			slog.ErrorContext(ctx, "issue not found")
			return NewFatalError(fmt.Errorf("issue not found (issue_id=%d): %w", input.IssueID, ErrIssueNotFound))
		}
		return NewRetryableError(fmt.Errorf("fetching issue: %w", err))
	}

	ctx = logger.WithLogFields(ctx, logger.LogFields{
		IntegrationID: &issue.IntegrationID,
	})

	slog.InfoContext(ctx, "issue loaded",
		"external_issue_id", issue.ExternalIssueID,
		"title", issue.Title)

	// Claim the issue: queued → processing
	// This prevents duplicate processing if multiple workers receive the same message
	claimed, issue, err := o.issues.ClaimQueued(ctx, input.IssueID)
	if err != nil {
		return NewRetryableError(fmt.Errorf("claiming issue: %w", err))
	}
	if !claimed {
		slog.InfoContext(ctx, "issue already claimed by another worker, skipping")
		return nil
	}

	const maxCycles = 8 // Drain until empty, then requeue if we're still seeing new events.
	var (
		needsFollowUp     bool
		followUpEventID   int64
		followUpEventType string
	)

	// Ensure we release the issue back to idle when done (success or failure)
	defer func() {
		if setIdleErr := o.issues.SetIdle(ctx, input.IssueID); setIdleErr != nil {
			slog.ErrorContext(ctx, "failed to set issue idle", "error", setIdleErr)
			return
		}

		if !needsFollowUp {
			return
		}

		queued, err := o.issues.QueueIfIdle(ctx, input.IssueID)
		if err != nil {
			slog.ErrorContext(ctx, "failed to requeue issue for follow-up", "error", err)
			return
		}
		if !queued {
			slog.InfoContext(ctx, "issue already queued or processing, skipping follow-up enqueue")
			return
		}

		if err := o.queueProducer.Enqueue(ctx, queue.Event{
			EventLogID:      followUpEventID,
			IssueID:         input.IssueID,
			EventType:       followUpEventType,
			Attempt:         1,
			TriggerThreadID: input.TriggerThreadID,
		}); err != nil {
			slog.ErrorContext(ctx, "failed to enqueue follow-up event", "error", err)
			if resetErr := o.issues.ResetQueuedToIdle(ctx, input.IssueID); resetErr != nil {
				slog.ErrorContext(ctx, "failed to reset queued issue after enqueue failure", "error", resetErr)
			}
		}
	}()

	// First contact ack (only once, before any planner cycles)
	firstContact, err := o.isFirstContact(ctx, *issue)
	if err != nil {
		slog.WarnContext(ctx, "failed to detect first contact, proceeding without ack", "error", err)
		firstContact = false
	}
	if firstContact {
		if err := o.postInstantAck(ctx, *issue, input.TriggerThreadID); err != nil {
			slog.WarnContext(ctx, "failed to post instant ack", "error", err)
		}
	}

	for cycle := 1; cycle <= maxCycles; cycle++ {
		// Get all unprocessed events for this issue (to mark them after cycle)
		pendingEvents, err := o.eventLogs.ListUnprocessedByIssue(ctx, issue.ID)
		if err != nil {
			slog.WarnContext(ctx, "failed to list pending events", "error", err)
		}

		// Run planner cycle (5-60s for LLM + actions)
		if err := o.runPlannerCycle(ctx, issue, input.TriggerThreadID); err != nil {
			return err
		}

		// Mark all pending events as processed
		if len(pendingEvents) > 0 {
			eventIDs := make([]int64, len(pendingEvents))
			for i, e := range pendingEvents {
				eventIDs[i] = e.ID
			}
			if err := o.eventLogs.MarkBatchProcessed(ctx, eventIDs); err != nil {
				slog.WarnContext(ctx, "failed to mark events processed", "error", err)
			}
		}

		// Check for NEW unprocessed events (arrived during this cycle)
		newEvents, err := o.eventLogs.ListUnprocessedByIssue(ctx, issue.ID)
		if err != nil {
			slog.WarnContext(ctx, "failed to check for new events", "error", err)
			break
		}

		if len(newEvents) == 0 {
			break // No new events, we're done
		}

		if cycle == maxCycles {
			needsFollowUp = true
			followUpEventID = newEvents[0].ID
			followUpEventType = newEvents[0].EventType
			slog.WarnContext(ctx, "max planner cycles reached with pending events, scheduling follow-up",
				"cycle", cycle,
				"max_cycles", maxCycles,
				"pending_event_count", len(newEvents))
			break
		}

		slog.InfoContext(ctx, "new events arrived during processing, re-running planner",
			"cycle", cycle,
			"max_cycles", maxCycles,
			"new_event_count", len(newEvents))

		// Refresh issue state (discussions updated by webhook handler)
		issue, err = o.issues.GetByID(ctx, issue.ID)
		if err != nil {
			return NewRetryableError(fmt.Errorf("refreshing issue: %w", err))
		}
	}

	slog.InfoContext(ctx, "engagement completed successfully")
	return nil
}

// runPlannerCycle runs a single planner iteration: build context → plan → validate → execute.
// If validation fails, the error is sent back to the Planner as a tool result, giving the model
// a chance to fix the issue (up to maxValidationRetries attempts).
func (o *Orchestrator) runPlannerCycle(ctx context.Context, issue *model.Issue, triggerThreadID string) error {
	messages, err := o.contextBuilder.BuildPlannerMessages(ctx, *issue, triggerThreadID)
	if err != nil {
		return NewRetryableError(fmt.Errorf("building planner context: %w", err))
	}

	slog.InfoContext(ctx, "context built", "message_count", len(messages))

	var output PlannerOutput
	var validationErr error

	for attempt := 0; attempt <= maxValidationRetries; attempt++ {
		if attempt == 0 {
			output, err = o.planner.Plan(ctx, messages)
		} else {
			// Inject validation error as tool result and let the model decide what to do
			feedback := FormatValidationErrorForLLM(validationErr)
			slog.WarnContext(ctx, "retrying planner with validation feedback",
				"attempt", attempt,
				"error", validationErr)

			feedbackMessages := append(output.Messages, llm.Message{
				Role:       "tool",
				Content:    feedback,
				ToolCallID: output.LastToolCallID,
			})
			output, err = o.planner.Plan(ctx, feedbackMessages)
		}

		if err != nil {
			return NewRetryableError(fmt.Errorf("running planner: %w", err))
		}

		slog.InfoContext(ctx, "planner completed",
			"action_count", len(output.Actions),
			"attempt", attempt+1,
			"reasoning", logger.Truncate(output.Reasoning, 200))

		// Model may decide not to submit actions after seeing the error
		if len(output.Actions) == 0 {
			return nil
		}

		validationInput := SubmitActionsInput{
			Actions:   output.Actions,
			Reasoning: output.Reasoning,
		}
		validationErr = o.actionValidator.Validate(ctx, *issue, validationInput)

		if validationErr == nil {
			break // Validation passed
		}

		slog.WarnContext(ctx, "action validation failed",
			"attempt", attempt+1,
			"error", validationErr)
	}

	if validationErr != nil {
		slog.ErrorContext(ctx, "validation failed after retries",
			"attempts", maxValidationRetries+1,
			"error", validationErr)
		return NewFatalError(fmt.Errorf("validating actions after %d attempts: %w",
			maxValidationRetries+1, validationErr))
	}

	tracker, ok := o.issueTrackers[issue.Provider]
	if !ok {
		return NewFatalError(fmt.Errorf("no issue tracker for provider: %s", issue.Provider))
	}

	executor := NewActionExecutor(tracker, o.txRunner, o.issues, o.gaps, o.integrations, o.learnings, o.specGenerator)
	errs := executor.ExecuteBatch(ctx, *issue, output.Actions)
	if len(errs) > 0 {
		for _, e := range errs {
			slog.ErrorContext(ctx, "action failed",
				"action_type", e.Action.Type,
				"error", e.Error,
				"recoverable", e.Recoverable)
		}
		return NewRetryableError(fmt.Errorf("executing actions: %d failed", len(errs)))
	}

	return nil
}

var ackMessages = []string{
	"I'll take a look at this.",
	"On it...",
	"Got it, digging in.",
	"Looking into this now.",
	"I'll check this out and come back to you.",
}

func (o *Orchestrator) isFirstContact(ctx context.Context, issue model.Issue) (bool, error) {
	sa, err := o.contextBuilder.GetRelayServiceAccount(ctx, issue.IntegrationID)
	if err != nil {
		return false, fmt.Errorf("getting relay service account: %w", err)
	}

	relayAuthorByID := fmt.Sprintf("id:%d", sa.UserID)
	for _, disc := range issue.Discussions {
		if strings.EqualFold(disc.Author, sa.Username) || disc.Author == relayAuthorByID {
			return false, nil
		}
	}

	slog.InfoContext(ctx, "first contact detected",
		"discussion_count", len(issue.Discussions))

	return true, nil
}

func (o *Orchestrator) postInstantAck(ctx context.Context, issue model.Issue, triggerThreadID string) error {
	tracker, ok := o.issueTrackers[issue.Provider]
	if !ok {
		return fmt.Errorf("no issue tracker for provider: %s", issue.Provider)
	}

	ackMsg := ackMessages[rand.Intn(len(ackMessages))]

	if triggerThreadID != "" {
		// Reply in the same thread where we were mentioned
		_, err := tracker.ReplyToThread(ctx, issue_tracker.ReplyToThreadParams{
			Issue:        issue,
			DiscussionID: triggerThreadID,
			Content:      ackMsg,
		})
		if err != nil {
			return fmt.Errorf("replying to thread: %w", err)
		}
	} else {
		// No thread context (e.g., mentioned in issue description) - create new thread
		_, err := tracker.CreateDiscussion(ctx, issue_tracker.CreateDiscussionParams{
			Issue:   issue,
			Content: ackMsg,
		})
		if err != nil {
			return fmt.Errorf("creating ack discussion: %w", err)
		}
	}

	slog.InfoContext(ctx, "posted instant ack",
		"trigger_thread_id", triggerThreadID,
		"message", ackMsg)

	return nil
}
