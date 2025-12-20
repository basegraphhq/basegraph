package model

import (
	"encoding/json"
	"time"
)

type EventLog struct {
	ID                  int64           `json:"id"`
	WorkspaceID         int64           `json:"workspace_id"`
	IssueID             int64           `json:"issue_id"`
	TriggeredByUsername string          `json:"triggered_by_username"`
	Source              string          `json:"source"`
	EventType           string          `json:"event_type"`
	Payload             json.RawMessage `json:"payload"`
	ExternalID          *string         `json:"external_id,omitempty"`
	DedupeKey           string          `json:"dedupe_key"`
	ProcessedAt         *time.Time      `json:"processed_at,omitempty"`
	ProcessingError     *string         `json:"processing_error,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
}
