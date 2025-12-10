package dto

type ListGitLabProjectsRequest struct {
	InstanceURL string `json:"instance_url" binding:"required,url"`
	Token       string `json:"token" binding:"required,min=10"`
}

type GitLabProjectResponse struct {
	Name        string `json:"name"`
	PathWithNS  string `json:"path_with_namespace"`
	WebURL      string `json:"web_url"`
	Description string `json:"description,omitempty"`
	ID          int64  `json:"id"`
}

type SetupGitLabIntegrationRequest struct {
	InstanceURL    string `json:"instance_url" binding:"required,url"`
	Token          string `json:"token" binding:"required,min=10"`
	WorkspaceID    int64  `json:"workspace_id,string" binding:"required"`
	OrganizationID int64  `json:"organization_id,string" binding:"required"`
	SetupByUserID  int64  `json:"setup_by_user_id,string" binding:"required"`
}

type SetupGitLabIntegrationResponse struct {
	Projects          []GitLabProjectResponse `json:"projects"`
	Errors            []string                `json:"errors,omitempty"`
	IntegrationID     int64                   `json:"integration_id,string"`
	RepositoriesAdded int                     `json:"repositories_added"`
	WebhooksCreated   int                     `json:"webhooks_created"`
	IsNewIntegration  bool                    `json:"is_new_integration"`
}

type GitLabStatusResponse struct {
	IntegrationID *int64            `json:"integration_id,string,omitempty"`
	Status        *GitLabSyncStatus `json:"status,omitempty"`
	ReposCount    int               `json:"repos_count"`
	Connected     bool              `json:"connected"`
}

type GitLabSyncStatus struct {
	UpdatedAt         string   `json:"updated_at,omitempty"`
	Errors            []string `json:"errors,omitempty"`
	WebhooksCreated   int      `json:"webhooks_created"`
	RepositoriesAdded int      `json:"repositories_added"`
	Synced            bool     `json:"synced"`
}

type RefreshGitLabIntegrationResponse struct {
	SetupGitLabIntegrationResponse
	Synced bool `json:"synced"`
}
