package model

import "time"

// Provider represents the integration provider type
type Provider string

const (
	ProviderGitLab Provider = "gitlab"
	ProviderGitHub Provider = "github"
	ProviderLinear Provider = "linear"
)

type Integration struct {
	ID                  int64     `json:"id"`
	WorkspaceID         int64     `json:"workspace_id"`
	OrganizationID      int64     `json:"organization_id"`
	Provider            Provider  `json:"provider"`
	ProviderBaseURL     *string   `json:"provider_base_url,omitempty"`
	ExternalOrgID       *string   `json:"external_org_id,omitempty"`
	ExternalWorkspaceID *string   `json:"external_workspace_id,omitempty"`
	AccessToken         string    `json:"-"` // never expose tokens in API
	RefreshToken        *string   `json:"-"`
	ExpiresAt           *time.Time `json:"-"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

