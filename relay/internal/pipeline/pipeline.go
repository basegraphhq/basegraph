package pipeline

import (
	"context"
	"fmt"

	"basegraph.app/relay/common/llm"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/store"
)

type Pipeline struct {
	keywords      *KeywordsExtractor
	planner       *Planner
	retriever     *Retriever
	gapDetector   *GapDetector
	specGenerator *SpecGenerator
	evalStore     store.LLMEvalStore
}

func New(llmClient llm.Client, evalStore store.LLMEvalStore) *Pipeline {
	return &Pipeline{
		keywords:      NewKeywordsExtractor(llmClient),
		planner:       NewPlanner(),
		retriever:     NewRetriever(),
		gapDetector:   NewGapDetector(),
		specGenerator: NewSpecGenerator(),
		evalStore:     evalStore,
	}
}

func (p *Pipeline) Process(ctx context.Context, issue *model.Issue, events []model.EventLog) (*model.Issue, error) {
	issue, err := p.keywords.Extract(ctx, issue, p.evalStore)
	if err != nil {
		return nil, fmt.Errorf("keywords extraction: %w", err)
	}

	// TODO: Planner → Retriever loop → Gap Detector → Spec Generator

	return issue, nil
}
