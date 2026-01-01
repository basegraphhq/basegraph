package brain

import (
	"context"
	"errors"

	"basegraph.app/relay/internal/model"
)

var ErrIssueNotFound = errors.New("issue not found")

type Orchestrator interface {
	HandleEngagement(ctx context.Context, input EngagementInput) error
}

type EngagementInput struct {
	IssueID    int64
	EventLogID int64
	EventType  string
}

type EngagementError struct {
	Err       error
	Retryable bool
}

func (e *EngagementError) Error() string {
	return e.Err.Error()
}

func (e *EngagementError) Unwrap() error {
	return e.Err
}

func NewRetryableError(err error) *EngagementError {
	return &EngagementError{Err: err, Retryable: true}
}

func NewFatalError(err error) *EngagementError {
	return &EngagementError{Err: err, Retryable: false}
}

type ActionExecutor interface {
	Execute(ctx context.Context, issue model.Issue, action Action) error
	ExecuteBatch(ctx context.Context, issue model.Issue, actions []Action) []ActionError
}

type ActionError struct {
	Action      Action
	Error       string
	Recoverable bool
}

type ActionValidator interface {
	Validate(ctx context.Context, issue model.Issue, input SubmitActionsInput) error
}

type ContextBuilder interface {
	BuildPlannerMessages(ctx context.Context, issue model.Issue) ([]Message, error)
}

type Message struct {
	Role    string
	Content string
}
