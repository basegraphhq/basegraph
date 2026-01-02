package brain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"

	"basegraph.app/relay/common/arangodb"
	"basegraph.app/relay/common/llm"
	"basegraph.app/relay/common/logger"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/service/issue_tracker"
	"basegraph.app/relay/internal/store"
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
	cfg             OrchestratorConfig
	planner         *Planner
	contextBuilder  *contextBuilder
	actionValidator ActionValidator
	issues          store.IssueStore
	gaps            store.GapStore
	eventLogs       store.EventLogStore
	issueTrackers   map[model.Provider]issue_tracker.IssueTrackerService
}

func NewOrchestrator(
	cfg OrchestratorConfig,
	agentClient llm.AgentClient,
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
	explore := NewExploreAgent(agentClient, tools, cfg.ModulePath)
	planner := NewPlanner(agentClient, explore)
	ctxBuilder := NewContextBuilder(integrations, configs, learnings)
	validator := NewActionValidator(gaps)

	slog.InfoContext(context.Background(), "orchestrator initialized",
		"repo_root", cfg.RepoRoot,
		"module_path", cfg.ModulePath)

	return &Orchestrator{
		cfg:             cfg,
		planner:         planner,
		contextBuilder:  ctxBuilder,
		actionValidator: validator,
		issues:          issues,
		gaps:            gaps,
		eventLogs:       eventLogs,
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

	// Ensure we release the issue back to idle when done (success or failure)
	defer func() {
		if setIdleErr := o.issues.SetIdle(ctx, input.IssueID); setIdleErr != nil {
			slog.ErrorContext(ctx, "failed to set issue idle", "error", setIdleErr)
		}
	}()

	firstContact, err := o.isFirstContact(ctx, *issue)
	if err != nil {
		slog.WarnContext(ctx, "failed to detect first contact, proceeding without ack",
			"error", err)
		firstContact = false
	}

	if firstContact {
		if err := o.postInstantAck(ctx, *issue, input.TriggerThreadID); err != nil {
			slog.WarnContext(ctx, "failed to post instant ack", "error", err)
		}
	}

	messages, err := o.contextBuilder.BuildPlannerMessages(ctx, *issue, input.TriggerThreadID)
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
		// Validate actions before execution to catch LLM output errors early
		validationInput := SubmitActionsInput{
			Actions:   output.Actions,
			Reasoning: output.Reasoning,
		}
		if err := o.actionValidator.Validate(ctx, *issue, validationInput); err != nil {
			slog.ErrorContext(ctx, "action validation failed", "error", err)
			return NewFatalError(fmt.Errorf("validating actions: %w", err))
		}

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

	slog.InfoContext(ctx, "engagement completed successfully")

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
