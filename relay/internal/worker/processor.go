package worker

import (
	"context"
	"fmt"
	"log/slog"

	"basegraph.app/relay/common/llm"
	"basegraph.app/relay/internal/brain"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/store"
)

type Processor struct {
	keywords      *brain.KeywordsExtractor
	planner       *brain.Planner
	executor      *brain.Executor
	gapDetector   *brain.GapDetector
	specGenerator *brain.SpecGenerator
	evalStore     store.LLMEvalStore
}

func NewProcessor(llmClient llm.Client, evalStore store.LLMEvalStore) *Processor {
	return &Processor{
		keywords:      brain.NewKeywordsExtractor(llmClient),
		planner:       brain.NewPlanner(llmClient),
		executor:      brain.NewExecutor(brain.NewStubCodeGraphRetriever(), brain.NewStubLearningsRetriever()),
		gapDetector:   brain.NewGapDetector(),
		specGenerator: brain.NewSpecGenerator(),
		evalStore:     evalStore,
	}
}

func (p *Processor) Process(ctx context.Context, issue *model.Issue, events []model.EventLog) (*model.Issue, error) {
	// Stage 1: Keywords Extraction
	issue, err := p.keywords.Extract(ctx, issue, p.evalStore)
	if err != nil {
		return nil, fmt.Errorf("keywords extraction: %w", err)
	}

	// Stage 2: Planning
	plan, err := p.planner.Plan(ctx, issue, p.evalStore)
	if err != nil {
		return nil, fmt.Errorf("planning: %w", err)
	}

	// Stage 3: Execution (if context is insufficient)
	if !plan.ContextSufficient && len(plan.Jobs) > 0 {
		slog.InfoContext(ctx, "executing retrieval jobs",
			"issue_id", issue.ID,
			"job_count", len(plan.Jobs))

		issue, err = p.executor.Execute(ctx, issue, plan.Jobs)
		if err != nil {
			return nil, fmt.Errorf("retrieval execution: %w", err)
		}
	} else {
		slog.InfoContext(ctx, "context sufficient, skipping retrieval",
			"issue_id", issue.ID,
			"reasoning", plan.Reasoning)
	}

	// TODO: Stage 4: Gap Detection
	// TODO: Stage 5: Spec Generation

	return issue, nil
}
