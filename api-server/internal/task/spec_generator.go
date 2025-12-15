package task

import (
	"context"
	"log/slog"
)

type SpecGenerator interface {
	GenerateSpec(context.Context, Event) (string, error)
}

type specGenerator struct {
	contextDocumentProvider ContextDocumentProvider
}

var _ SpecGenerator = &specGenerator{}

func NewSpecGenerator(contextDocumentProvider ContextDocumentProvider) *specGenerator {
	return &specGenerator{
		contextDocumentProvider: contextDocumentProvider,
	}
}

func (s *specGenerator) GenerateSpec(ctx context.Context, event Event) (string, error) {
	contextDocument, err := s.contextDocumentProvider.GetContextDocument(event.Id)
	if err != nil {
		return "", err
	}
	slog.InfoContext(ctx, "fetched context document", "id", event.Id, "contextDocument", contextDocument)

	return "", nil
}
