package relay

import (
	"context"
	"log/slog"
)

type Job string

const (
	JobExtractKeywords   Job = "extract_keywords"
	JobFetchLearnings    Job = "fetch_learnings"
	JobFetchCodeFindings Job = "fetch_code_findings"
)

type Executor interface {
	Execute(ctx context.Context, event Event, jobs []Job) error
}

type executor struct {
	keywordsProvider  KeywordsProvider
	learningsProvider LearningsProvider
	codegraphProvider CodegraphProvider

	contextDocumentProvider ContextDocumentProvider
}

var _ Executor = &executor{}

func NewExecutor(
	keywordsProvider KeywordsProvider,
	learningsProvider LearningsProvider,
	codegraphProvider CodegraphProvider,
	contextDocumentProvider ContextDocumentProvider,
) *executor {
	return &executor{
		keywordsProvider:        keywordsProvider,
		codegraphProvider:       codegraphProvider,
		learningsProvider:       learningsProvider,
		contextDocumentProvider: contextDocumentProvider,
	}
}

func (e *executor) Execute(ctx context.Context, event Event, jobs []Job) error {
	slog.InfoContext(ctx, "executing jobs", "event", event, "jobs", jobs)
	return nil
}
