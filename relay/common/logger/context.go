package logger

import "context"

type contextKey string

const logFieldsKey contextKey = "log_fields"

// LogFields contains structured fields automatically added to all logs within a context.
// Fields flow through context enrichment, enabling zero-touch logging where business
// context (issue_id, event_log_id, etc.) is automatically included in all log statements.
type LogFields struct {
	IssueID       *int64  // Relay issue ID
	EventLogID    *int64  // Event log ID that triggered this engagement
	MessageID     *string // Redis stream message ID
	IntegrationID *int64  // Integration ID
	WorkspaceID   *int64  // Workspace ID
	EventType     *string // Event type (e.g., "comment_created", "issue_opened")
	Component     string  // Component name (OTel semantic convention style, e.g., "relay.brain.orchestrator")
}

// WithLogFields enriches context with structured log fields.
// Multiple calls merge fields, with newer non-nil/non-empty values taking precedence.
// Context timeouts and cancellation are preserved.
func WithLogFields(ctx context.Context, fields LogFields) context.Context {
	existing := GetLogFields(ctx)
	merged := mergeFields(existing, fields)
	return context.WithValue(ctx, logFieldsKey, merged)
}

// GetLogFields retrieves log fields from context.
// Returns empty LogFields if none are set.
func GetLogFields(ctx context.Context) LogFields {
	if fields, ok := ctx.Value(logFieldsKey).(LogFields); ok {
		return fields
	}
	return LogFields{}
}

// mergeFields merges two LogFields, preferring non-nil/non-empty values from 'new'.
func mergeFields(existing, new LogFields) LogFields {
	result := existing

	if new.IssueID != nil {
		result.IssueID = new.IssueID
	}
	if new.EventLogID != nil {
		result.EventLogID = new.EventLogID
	}
	if new.MessageID != nil {
		result.MessageID = new.MessageID
	}
	if new.IntegrationID != nil {
		result.IntegrationID = new.IntegrationID
	}
	if new.WorkspaceID != nil {
		result.WorkspaceID = new.WorkspaceID
	}
	if new.EventType != nil {
		result.EventType = new.EventType
	}
	if new.Component != "" {
		result.Component = new.Component
	}

	return result
}

// Ptr is a helper to create a pointer from a value.
// Useful for setting LogFields inline: logger.WithLogFields(ctx, logger.LogFields{IssueID: logger.Ptr(id)})
func Ptr[T any](v T) *T {
	return &v
}

// Truncate truncates a string to maxLen characters, appending "..." if truncated.
// Useful for logging potentially long strings like queries or error messages.
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
