package dto

import "encoding/json"

type IngestEventRequest struct {
	IntegrationID   int64           `json:"integration_id" binding:"required"`
	ExternalIssueID string          `json:"external_issue_id" binding:"required"`
	EventType       string          `json:"event_type" binding:"required"`
	Source          *string         `json:"source,omitempty"`
	ExternalEventID *string         `json:"external_event_id,omitempty"`
	DedupeKey       *string         `json:"dedupe_key,omitempty"`
	Payload         json.RawMessage `json:"payload" binding:"required"`

	Title       *string  `json:"title,omitempty"`
	Description *string  `json:"description,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Members     []string `json:"members,omitempty"`
	Assignees   []string `json:"assignees,omitempty"`
	Reporter    *string  `json:"reporter,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
}

type IngestEventResponse struct {
	EventLogID int64  `json:"event_log_id"`
	IssueID    int64  `json:"issue_id"`
	DedupeKey  string `json:"dedupe_key"`
	Enqueued   bool   `json:"enqueued"`
	Duplicated bool   `json:"duplicated"`
}
