package domain

import (
	"encoding/json"
	"time"
)

// EventType represents the semantic type of an issue activity that Relay processes.
type EventType string

const (
	EventTypeIssueCreated EventType = "issue_created"
	EventTypeReply        EventType = "reply"
	EventTypePostReply    EventType = "post_reply"
)

// Event represents a canonical event pulled from the event_logs table and queued to Relay.
type Event struct {
	ID            int64           // event_logs.id
	IssueID       int64           // event_logs.issue_id
	WorkspaceID   int64           // event_logs.workspace_id
	IntegrationID int64           // derived via issue → integration
	Type          EventType       // event_logs.event_type (normalized)
	Source        string          // event_logs.source
	Payload       json.RawMessage // canonical payload stored in event_logs.payload
	ExternalID    *string         // upstream event identifier when available
	TraceID       *string         // tracing identifier propagated from ingress
	Attempt       int             // pipeline attempt count (from Redis message)
	CreatedAt     time.Time       // event_logs.created_at
}
