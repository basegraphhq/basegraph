package relay

import (
	"context"
	"log/slog"
)

type GapDetector interface {
	DetectGaps(context.Context, Event) ([]string, error)
}

type gapDetector struct {
	contextDocumentProvider ContextDocumentProvider
}

var _ GapDetector = &gapDetector{}

func NewGapDetector(contextDocumentProvider ContextDocumentProvider) *gapDetector {
	return &gapDetector{
		contextDocumentProvider: contextDocumentProvider,
	}
}

func (g *gapDetector) DetectGaps(ctx context.Context, event Event) ([]string, error) {
	doc, err := g.contextDocumentProvider.GetContextDocument(event.Id)
	if err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "fetched context document", "id", event.Id, "contextDocument", doc)

	// Given the context document, find out gaps in the issue and the findings
	// Return the gaps

	return nil, nil
}
