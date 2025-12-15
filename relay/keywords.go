package relay

import (
	"context"
	"log/slog"
)

type KeywordsProvider interface {
	GetKeywords(context.Context, Event) ([]string, error)
}

type keywordsProvider struct {
	contextDocumentProvider ContextDocumentProvider
}

var _ KeywordsProvider = &keywordsProvider{}

func NewKeywordsProvider(p ContextDocumentProvider) *keywordsProvider {
	return &keywordsProvider{
		contextDocumentProvider: p,
	}
}

func (k *keywordsProvider) GetKeywords(ctx context.Context, event Event) ([]string, error) {
	doc, err := k.contextDocumentProvider.GetContextDocument(event.Id)
	if err != nil {
		return nil, err
	}

	slog.InfoContext(ctx, "fetched context document", "id", event.Id, "contextDocument", doc)

	// Event will have new words to add to the context document
	// Given the context document and new input, detect new keywords
	// Return the keywords

	return nil, nil
}
