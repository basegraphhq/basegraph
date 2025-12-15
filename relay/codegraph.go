package relay

import (
	"context"
	"log/slog"
)

type CodegraphProvider interface {
	GetCodeFindings(context.Context, Event) ([]string, error)
}

type codegraphProvider struct {
	contextDocumentProvider ContextDocumentProvider
}

var _ CodegraphProvider = &codegraphProvider{}

func NewCodegraphProvider(p ContextDocumentProvider) *codegraphProvider {
	return &codegraphProvider{
		contextDocumentProvider: p,
	}
}

func (c *codegraphProvider) GetCodeFindings(ctx context.Context, event Event) ([]string, error) {
	doc, err := c.contextDocumentProvider.GetContextDocument(event.Id)
	if err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "fetched context document", "id", event.Id, "contextDocument", doc)
	return nil, nil
}
