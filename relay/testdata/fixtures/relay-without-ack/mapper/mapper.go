package mapper

import (
	"context"
)

type CanonicalEventType string

const (
	EventIssueCreated CanonicalEventType = "issue_created"
	EventIssueClosed  CanonicalEventType = "issue_closed"
	EventReply        CanonicalEventType = "reply"
	EventPRCreated    CanonicalEventType = "pull_request_created"
)

type EventMapper interface {
	Map(ctx context.Context, body map[string]any, headers map[string]string) (CanonicalEventType, error)
}
