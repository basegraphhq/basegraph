package relay

import (
	"context"
	"log/slog"
)

type Planner interface {
	Plan(context.Context, Event) error
}

type planner struct {
	contextDocumentProvider ContextDocumentProvider
	executor                Executor
}

var _ Planner = &planner{}

func NewPlanner(p ContextDocumentProvider, e Executor) *planner {
	return &planner{
		contextDocumentProvider: p,
		executor:                e,
	}
}

func (p *planner) Plan(ctx context.Context, event Event) error {
	doc, err := p.contextDocumentProvider.GetContextDocument(event.Id)
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "fetched context document", "id", event.Id, "contextDocument", doc)

	// Given the context document, find out what contexts are missing and trigger jobs to collect them
	// Return the jobs to execute

	return nil
}
