package relay

import (
	"context"
	"log/slog"
)

type LearningsProvider interface {
	GetLearnings(context.Context, Event) ([]string, error)
}

type learningsProvider struct {
	contextDocumentProvider ContextDocumentProvider
}

var _ LearningsProvider = &learningsProvider{}

func NewLearningsProvider(p ContextDocumentProvider) *learningsProvider {
	return &learningsProvider{
		contextDocumentProvider: p,
	}
}

func (l *learningsProvider) GetLearnings(ctx context.Context, event Event) ([]string, error) {
	doc, err := l.contextDocumentProvider.GetContextDocument(event.Id)
	if err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "fetched context document", "id", event.Id, "contextDocument", doc)
	return nil, nil
}
