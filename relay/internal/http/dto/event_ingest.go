package dto

import "encoding/json"

type IngestEventRequest struct {
	IntegrationID       int64           `json:"integration_id" binding:"required"`
	ExternalIssueID     string          `json:"external_issue_id" binding:"required"`
	TriggeredByUsername string          `json:"triggered_by_username" binding:"required"`
	EventType           string          `json:"event_type" binding:"required"`
	Payload             json.RawMessage `json:"payload" binding:"required"`
}

type IngestEventResponse struct {
	EventLogID int64  `json:"event_log_id"`
	IssueID    int64  `json:"issue_id"`
	DedupeKey  string `json:"dedupe_key"`
	Enqueued   bool   `json:"enqueued"`
	Duplicated bool   `json:"duplicated"`
}
