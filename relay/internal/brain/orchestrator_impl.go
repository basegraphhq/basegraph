package brain

import (
	"context"
	"fmt"
	"log/slog"

	"basegraph.app/relay/common/arangodb"
	"basegraph.app/relay/common/llm"
	"basegraph.app/relay/common/logger"
	"basegraph.app/relay/internal/store"
)

type OrchestratorConfig struct {
	RepoRoot   string
	ModulePath string
}

type OrchestratorImpl struct {
	cfg       OrchestratorConfig
	planner   *Planner
	issues    store.IssueStore
	eventLogs store.EventLogStore
}

func NewOrchestrator(
	cfg OrchestratorConfig,
	agentClient llm.AgentClient,
	arangoDB arangodb.Client,
	issues store.IssueStore,
	eventLogs store.EventLogStore,
) *OrchestratorImpl {
	tools := NewExploreTools(cfg.RepoRoot, arangoDB)
	explore := NewExploreAgent(agentClient, tools, cfg.ModulePath)
	planner := NewPlanner(agentClient, explore)

	slog.InfoContext(context.Background(), "orchestrator initialized",
		"repo_root", cfg.RepoRoot,
		"module_path", cfg.ModulePath)

	return &OrchestratorImpl{
		cfg:       cfg,
		planner:   planner,
		issues:    issues,
		eventLogs: eventLogs,
	}
}

func (o *OrchestratorImpl) HandleEngagement(ctx context.Context, input EngagementInput) error {
	// Enrich context with engagement metadata
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

	// Enrich context with issue details
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
	// 4. Build context (issue + learnings + findings + gaps + discussions)
	// 5. Invoke Planner
	// 6. Execute actions returned by Planner
	// 7. Mark events processed, release issue

	return nil
}
