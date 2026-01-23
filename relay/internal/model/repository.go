package model

import "time"

// Repository represents a code repository connected through an integration
type Repository struct {
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Description    *string   `json:"description,omitempty"`
	DefaultBranch  *string   `json:"default_branch,omitempty"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	URL            string    `json:"url"`
	ExternalRepoID string    `json:"external_repo_id"`
	ID             int64     `json:"id"`
	WorkspaceID    int64     `json:"workspace_id"`
	IntegrationID  int64     `json:"integration_id"`
	IsEnabled      bool      `json:"is_enabled"`
}
