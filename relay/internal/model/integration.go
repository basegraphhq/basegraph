package model

import "time"

type Provider string

const (
	ProviderGitLab Provider = "gitlab"
	ProviderGitHub Provider = "github"
	ProviderLinear Provider = "linear"
	ProviderJira   Provider = "jira"
)

type Integration struct {
	ID                  int64     `json:"id"`
	WorkspaceID         int64     `json:"workspace_id"`
	OrganizationID      int64     `json:"organization_id"`
	ConnectedByUserID   int64     `json:"connected_by_user_id"`
	Provider            Provider  `json:"provider"`
	ProviderBaseURL     *string   `json:"provider_base_url,omitempty"`
	ExternalOrgID       *string   `json:"external_org_id,omitempty"`
	ExternalWorkspaceID *string   `json:"external_workspace_id,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}
