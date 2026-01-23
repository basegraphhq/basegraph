package model

import (
	"encoding/json"
	"time"
)

type WorkspaceEventType string

const (
	WorkspaceEventTypeSetup   WorkspaceEventType = "workspace_setup"
	WorkspaceEventTypeRepoSync WorkspaceEventType = "repo_sync"
)

type WorkspaceEventStatus string

const (
	WorkspaceEventStatusQueued            WorkspaceEventStatus = "queued"
	WorkspaceEventStatusRunning           WorkspaceEventStatus = "running"
	WorkspaceEventStatusSucceeded         WorkspaceEventStatus = "succeeded"
	WorkspaceEventStatusFailed            WorkspaceEventStatus = "failed"
	WorkspaceEventStatusSucceededWithErrors WorkspaceEventStatus = "succeeded_with_errors"
)

type WorkspaceEventLog struct {
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	StartedAt      *time.Time      `json:"started_at,omitempty"`
	FinishedAt     *time.Time      `json:"finished_at,omitempty"`
	Error          *string         `json:"error,omitempty"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	EventType      string          `json:"event_type"`
	Status         string          `json:"status"`
	ID             int64           `json:"id"`
	WorkspaceID    int64           `json:"workspace_id"`
	OrganizationID int64           `json:"organization_id"`
	RepoID         *int64          `json:"repo_id,omitempty"`
}
