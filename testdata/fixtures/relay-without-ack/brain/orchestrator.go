package brain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"basegraph.co/relay/common/arangodb"
	"basegraph.co/relay/common/llm"
	"basegraph.co/relay/common/logger"
	"basegraph.co/relay/internal/model"
	"basegraph.co/relay/internal/service/issue_tracker"
	"basegraph.co/relay/internal/store"
)

var ErrIssueNotFound = errors.New("issue not found")

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
}

type Orchestrator struct {
	cfg            OrchestratorConfig
	planner        *Planner
	contextBuilder *contextBuilder
	issues         store.IssueStore
	gaps           store.GapStore
	eventLogs      store.EventLogStore
	issueTrackers  map[model.Provider]issue_tracker.IssueTrackerService
}

func NewOrchestrator(
	cfg OrchestratorConfig,
	plannerClient llm.AgentClient,
	exploreClient llm.AgentClient,
	arangoDB arangodb.Client,
	issues store.IssueStore,
	gaps store.GapStore,
	eventLogs store.EventLogStore,
	integrations store.IntegrationStore,
	configs store.IntegrationConfigStore,
	learnings store.LearningStore,
	issueTrackers map[model.Provider]issue_tracker.IssueTrackerService,
) *Orchestrator {
	tools := NewExploreTools(cfg.RepoRoot, arangoDB)
	explore := NewExploreAgent(exploreClient, tools, cfg.ModulePath)
	planner := NewPlanner(plannerClient, explore)
	ctxBuilder := NewContextBuilder(integrations, configs, learnings)

	slog.InfoContext(context.Background(), "orchestrator initialized",
		"repo_root", cfg.RepoRoot,
		"module_path", cfg.ModulePath)

	return &Orchestrator{
		cfg:            cfg,
		planner:        planner,
		contextBuilder: ctxBuilder,
		issues:         issues,
		gaps:           gaps,
		eventLogs:      eventLogs,
		issueTrackers:  issueTrackers,
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

	// TODO: Implement full orchestration flow per spec:
	// 1. Claim issue (prevent duplicate processing)
	// 2. Determine if first contact
	// 3. Post instant ack if first engagement

	messages, err := o.contextBuilder.BuildPlannerMessages(ctx, *issue)
	if err != nil {
		return NewRetryableError(fmt.Errorf("building planner context: %w", err))
	}

	slog.InfoContext(ctx, "context built",
		"message_count", len(messages))

	output, err := o.planner.Plan(ctx, messages)
	if err != nil {
		return NewRetryableError(fmt.Errorf("running planner: %w", err))
	}

	slog.InfoContext(ctx, "planner completed",
		"action_count", len(output.Actions),
		"reasoning", logger.Truncate(output.Reasoning, 200))

	if len(output.Actions) > 0 {
		tracker, ok := o.issueTrackers[issue.Provider]
		if !ok {
			return NewFatalError(fmt.Errorf("no issue tracker for provider: %s", issue.Provider))
		}

		executor := NewActionExecutor(tracker, o.issues, o.gaps)
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
	}

	// TODO: Mark events processed, release issue

	return nil
}
