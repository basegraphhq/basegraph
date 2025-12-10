package model

import "time"

type Provider string

const (
	ProviderGitLab Provider = "gitlab"
	ProviderGitHub Provider = "github"
	ProviderLinear Provider = "linear"
	ProviderJira   Provider = "jira"
	ProviderSlack  Provider = "slack"
	ProviderNotion Provider = "notion"
)

type Capability string

const (
	CapabilityCodeRepo      Capability = "code_repo"
	CapabilityIssueTracker  Capability = "issue_tracker"
	CapabilityDocumentation Capability = "documentation"
	CapabilityCommunication Capability = "communication"
	CapabilityWiki          Capability = "wiki"
)

type Integration struct {
	CreatedAt           time.Time    `json:"created_at"`
	UpdatedAt           time.Time    `json:"updated_at"`
	ProviderBaseURL     *string      `json:"provider_base_url,omitempty"`
	ExternalOrgID       *string      `json:"external_org_id,omitempty"`
	ExternalWorkspaceID *string      `json:"external_workspace_id,omitempty"`
	Provider            Provider     `json:"provider"`
	Capabilities        []Capability `json:"capabilities"`
	ID                  int64        `json:"id"`
	WorkspaceID         int64        `json:"workspace_id"`
	OrganizationID      int64        `json:"organization_id"`
	SetupByUserID       int64        `json:"setup_by_user_id"`
	IsEnabled           bool         `json:"is_enabled"`
}

var ProviderCapabilities = map[Provider][]Capability{
	ProviderGitLab: {CapabilityCodeRepo, CapabilityIssueTracker, CapabilityWiki},
	ProviderGitHub: {CapabilityCodeRepo, CapabilityIssueTracker, CapabilityWiki},
	ProviderLinear: {CapabilityIssueTracker},
	ProviderJira:   {CapabilityIssueTracker},
	ProviderSlack:  {CapabilityCommunication},
	ProviderNotion: {CapabilityDocumentation, CapabilityWiki},
}

func (p Provider) DefaultCapabilities() []Capability {
	if caps, ok := ProviderCapabilities[p]; ok {
		return caps
	}
	return []Capability{}
}
