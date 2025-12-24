package worker

import (
	"context"

	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/queue"
)

// Consumer abstracts the message queue for testability.
type Consumer interface {
	Read(ctx context.Context) ([]queue.Message, error)
	Ack(ctx context.Context, msg queue.Message) error
	Requeue(ctx context.Context, msg queue.Message, errMsg string) error
	SendDLQ(ctx context.Context, msg queue.Message, errMsg string) error
}

// IssueProcessor abstracts the pipeline processing for testability.
type IssueProcessor interface {
	Process(ctx context.Context, issue *model.Issue, events []model.EventLog) (*model.Issue, error)
}
