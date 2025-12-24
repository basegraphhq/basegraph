package worker

import (
	"context"
	"fmt"

	"basegraph.app/relay/common/llm"
	"basegraph.app/relay/internal/brain"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/store"
)

type Processor struct {
	keywords      *brain.KeywordsExtractor
	planner       *brain.Planner
	retriever     *brain.Retriever
	gapDetector   *brain.GapDetector
	specGenerator *brain.SpecGenerator
	evalStore     store.LLMEvalStore
}

func NewProcessor(llmClient llm.Client, evalStore store.LLMEvalStore) *Processor {
	return &Processor{
		keywords:      brain.NewKeywordsExtractor(llmClient),
		planner:       brain.NewPlanner(),
		retriever:     brain.NewRetriever(),
		gapDetector:   brain.NewGapDetector(),
		specGenerator: brain.NewSpecGenerator(),
		evalStore:     evalStore,
	}
}

func (p *Processor) Process(ctx context.Context, issue *model.Issue, events []model.EventLog) (*model.Issue, error) {
	issue, err := p.keywords.Extract(ctx, issue, p.evalStore)
	if err != nil {
		return nil, fmt.Errorf("keywords extraction: %w", err)
	}

	// TODO: Planner → Retriever loop → Gap Detector → Spec Generator

	return issue, nil
}
