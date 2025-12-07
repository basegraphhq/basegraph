package model

import "time"

// Repository represents a code repository connected through an integration
type Repository struct {
	ID             int64     `json:"id"`
	WorkspaceID    int64     `json:"workspace_id"`
	IntegrationID  int64     `json:"integration_id"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	URL            string    `json:"url"`
	Description    *string   `json:"description,omitempty"`
	ExternalRepoID string    `json:"external_repo_id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

