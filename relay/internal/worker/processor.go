package worker

import (
	"context"
	"fmt"
	"log/slog"

	"basegraph.app/relay/common/arangodb"
	"basegraph.app/relay/common/llm"
	"basegraph.app/relay/internal/brain"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/store"
)

type Processor struct {
	planner       *brain.Planner
	gapDetector   *brain.GapDetector
	specGenerator *brain.SpecGenerator
	evalStore     store.LLMEvalStore
}

// ProcessorDeps holds required dependencies for the processor.
type ProcessorDeps struct {
	AgentClient llm.AgentClient
	RepoRoot    string // Root directory of the repository to search
	ModulePath  string // Go module path for qname construction (e.g., "basegraph.app/relay")
	ArangoDB    arangodb.Client
}

func NewProcessor(evalStore store.LLMEvalStore, deps ProcessorDeps) *Processor {
	// Create retriever tools with grep/glob/read and graph
	tools := brain.NewRetrieverTools(deps.RepoRoot, deps.ArangoDB)

	// Create retriever sub-agent with module path for qname construction
	retriever := brain.NewRetriever(deps.AgentClient, tools, deps.ModulePath)

	// Create planner with retriever
	planner := brain.NewPlanner(deps.AgentClient, retriever)

	slog.Info("brain initialized",
		"retriever", "sub-agent with grep/glob/read/graph tools",
		"planner", "agentic loop with retrieve tool",
		"repo_root", deps.RepoRoot,
		"module_path", deps.ModulePath)

	return &Processor{
		planner:       planner,
		gapDetector:   brain.NewGapDetector(),
		specGenerator: brain.NewSpecGenerator(),
		evalStore:     evalStore,
	}
}

func (p *Processor) Process(ctx context.Context, issue *model.Issue, events []model.EventLog) (*model.Issue, error) {
	// Stage 1: Planning (now includes retrieval internally)
	// The Planner spawns Retriever sub-agents as needed and returns accumulated context
	plan, err := p.planner.Plan(ctx, issue)
	if err != nil {
		return nil, fmt.Errorf("planning: %w", err)
	}

	slog.InfoContext(ctx, "planning completed",
		"issue_id", issue.ID,
		"context_length", len(plan.Context),
		"reasoning", truncateString(plan.Reasoning, 200))

	fmt.Println("========================= PLANNER OUTPUT =============================")
	fmt.Println(plan.Context)
	fmt.Println(plan.Reasoning)

	// TODO: Stage 3: Gap Detection
	// gapDetector.Detect(ctx, issue, plan.Context)

	// TODO: Stage 4: Spec Generation
	// specGenerator.Generate(ctx, issue, gaps)

	return issue, nil
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
