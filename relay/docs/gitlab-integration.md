# GitLab Integration - Technical Reference

This document provides a complete technical reference for the GitLab integration implementation. It covers the full data flow from the dashboard UI through the relay backend to the database.

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                                  DASHBOARD                                       │
├─────────────────────────────────────────────────────────────────────────────────┤
│  gitlab-connect-panel.tsx                                                        │
│         │                                                                        │
│         ▼                                                                        │
│  relayClient.gitlab.setup()  ────►  /api/integrations/gitlab/setup/route.ts     │
│  relayClient.gitlab.status() ────►  /api/integrations/gitlab/status/route.ts    │
│  relayClient.gitlab.refresh()────►  /api/integrations/gitlab/refresh/route.ts   │
└─────────────────────────────────────────────────────────────────────────────────┘
                                          │
                                          ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                                    RELAY                                         │
├─────────────────────────────────────────────────────────────────────────────────┤
│  Router (router/gitlab.go)                                                       │
│         │                                                                        │
│         ▼                                                                        │
│  Handler (handler/gitlab.go)                                                     │
│         │                                                                        │
│         ▼                                                                        │
│  Service (service/integration/gitlab.go)                                         │
│         │                                                                        │
│         ▼                                                                        │
│  Stores (store/*.go) ──► sqlc ──► PostgreSQL                                    │
└─────────────────────────────────────────────────────────────────────────────────┘
                                          │
                                          │
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              WEBHOOK RECEIVER                                    │
├─────────────────────────────────────────────────────────────────────────────────┤
│  POST /webhooks/gitlab/:integration_id                                           │
│         │                                                                        │
│         ▼                                                                        │
│  handler/webhook/gitlab.go                                                       │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## Dashboard Layer

### Components

#### `gitlab-connect-panel.tsx`

A slide-out sheet component that guides users through GitLab connection.

**Location**: `dashboard/components/gitlab-connect-panel.tsx`

**Props**:
```typescript
interface GitLabConnectPanelProps {
  children: React.ReactNode       // Trigger element
  onConnect?: (data: GitLabSetupResponse) => void
  onError?: (error: string) => void
}
```

**State Management**:
| State | Type | Purpose |
|-------|------|---------|
| `instanceUrl` | `string` | GitLab instance URL (default: `https://gitlab.com`) |
| `token` | `string` | Personal Access Token input |
| `connectLoading` | `boolean` | Loading state during API call |
| `error` | `string \| null` | Error message display |

**Flow**:
1. User opens panel via trigger element
2. User enters GitLab instance URL and PAT
3. On submit, calls `relayClient.gitlab.setup()`
4. On success, invokes `onConnect` callback with response data

### API Routes

All routes are Next.js API routes that proxy requests to the relay backend. They handle session validation before forwarding.

#### `POST /api/integrations/gitlab/setup`

**Location**: `dashboard/app/api/integrations/gitlab/setup/route.ts`

**Request Body** (from client):
```typescript
{
  instance_url: string
  token: string
}
```

**Forwarded Body** (to relay):
```typescript
{
  instance_url: string
  token: string
  workspace_id: string      // From session validation
  organization_id: string   // From session validation
  setup_by_user_id: string  // From session validation
}
```

**Response**:
```typescript
{
  integration_id: string
  is_new_integration: boolean
  projects: GitLabProject[]
  webhooks_created: number
  repositories_added: number
  errors?: string[]
}
```

#### `GET /api/integrations/gitlab/status`

**Location**: `dashboard/app/api/integrations/gitlab/status/route.ts`

**Query Params** (added by route): `workspace_id` from session

**Response**:
```typescript
{
  connected: boolean
  integration_id?: string
  status?: {
    synced: boolean
    webhooks_created: number
    repositories_added: number
    errors?: string[]
    updated_at?: string
  }
  repos_count: number
}
```

#### `POST /api/integrations/gitlab/refresh`

**Location**: `dashboard/app/api/integrations/gitlab/refresh/route.ts`

**Request Body** (forwarded to relay):
```typescript
{
  workspace_id: string
}
```

**Response**: Same as setup response.

### Relay Client

**Location**: `dashboard/lib/relay-client.ts`

```typescript
export const relayClient = {
  gitlab: {
    setup(params: GitLabSetupParams): Promise<GitLabSetupResponse>
    status(): Promise<GitLabStatusResponse>
    refresh(): Promise<GitLabRefreshResponse>
  }
}
```

---

## Relay Backend Layer

### Router

**Location**: `relay/internal/http/router/gitlab.go`

```go
func GitLabRouter(router *gin.RouterGroup, handler *handler.GitLabHandler) {
    router.POST("/projects", handler.ListProjects)
    router.POST("/setup", handler.SetupIntegration)
    router.GET("/status", handler.GetStatus)
    router.POST("/refresh", handler.RefreshIntegration)
}

func GitLabWebhookRouter(router *gin.RouterGroup, handler *webhook.GitLabWebhookHandler) {
    router.POST("/:integration_id", handler.HandleEvent)
}
```

**Route Registration** (in `router/router.go`):
```go
gitlabHandler := handler.NewGitLabHandler(services.GitLab(), services.WebhookBaseURL())
GitLabRouter(v1.Group("/integrations/gitlab"), gitlabHandler)

webhookHandler := webhook.NewGitLabWebhookHandler(services.IntegrationCredentials(), slog.Default())
GitLabWebhookRouter(router.Group("/webhooks/gitlab"), webhookHandler)
```

### DTOs

**Location**: `relay/internal/http/dto/gitlab.go`

#### Request DTOs

```go
type SetupGitLabIntegrationRequest struct {
    InstanceURL    string `json:"instance_url" binding:"required,url"`
    Token          string `json:"token" binding:"required,min=10"`
    WorkspaceID    int64  `json:"workspace_id,string" binding:"required"`
    OrganizationID int64  `json:"organization_id,string" binding:"required"`
    SetupByUserID  int64  `json:"setup_by_user_id,string" binding:"required"`
}

type ListGitLabProjectsRequest struct {
    InstanceURL string `json:"instance_url" binding:"required,url"`
    Token       string `json:"token" binding:"required,min=10"`
}
```

#### Response DTOs

```go
type GitLabProjectResponse struct {
    ID          int64  `json:"id"`
    Name        string `json:"name"`
    PathWithNS  string `json:"path_with_namespace"`
    WebURL      string `json:"web_url"`
    Description string `json:"description,omitempty"`
}

type SetupGitLabIntegrationResponse struct {
    IntegrationID     int64                   `json:"integration_id,string"`
    IsNewIntegration  bool                    `json:"is_new_integration"`
    Projects          []GitLabProjectResponse `json:"projects"`
    WebhooksCreated   int                     `json:"webhooks_created"`
    RepositoriesAdded int                     `json:"repositories_added"`
    Errors            []string                `json:"errors,omitempty"`
}

type GitLabStatusResponse struct {
    Connected     bool              `json:"connected"`
    IntegrationID *int64            `json:"integration_id,string,omitempty"`
    Status        *GitLabSyncStatus `json:"status,omitempty"`
    ReposCount    int               `json:"repos_count"`
}

type GitLabSyncStatus struct {
    Synced            bool     `json:"synced"`
    WebhooksCreated   int      `json:"webhooks_created"`
    RepositoriesAdded int      `json:"repositories_added"`
    Errors            []string `json:"errors,omitempty"`
    UpdatedAt         string   `json:"updated_at,omitempty"`
}
```

### Handler

**Location**: `relay/internal/http/handler/gitlab.go`

```go
type GitLabHandler struct {
    gitlabService  integration.GitLabService
    webhookBaseURL string
}
```

#### Handler Methods

| Method | Endpoint | Purpose |
|--------|----------|---------|
| `ListProjects` | `POST /projects` | Lists GitLab projects accessible to the token |
| `SetupIntegration` | `POST /setup` | Creates integration, credentials, webhooks, and repositories |
| `GetStatus` | `GET /status` | Returns current integration status for workspace |
| `RefreshIntegration` | `POST /refresh` | Re-syncs projects and webhooks using stored credentials |

### Service

**Location**: `relay/internal/service/integration/gitlab.go`

```go
type GitLabService interface {
    ListProjects(ctx context.Context, instanceURL, token string) ([]GitLabProject, error)
    SetupIntegration(ctx context.Context, params SetupIntegrationParams) (*SetupResult, error)
    Status(ctx context.Context, workspaceID int64) (*StatusResult, error)
    RefreshIntegration(ctx context.Context, workspaceID int64, webhookBaseURL string) (*SetupResult, error)
}
```

#### `SetupIntegration` Flow

This is the core method. Here's what happens step by step:

1. **Validate Token**: Create GitLab client, list projects to verify token works
2. **Check for Existing Integration**: Query `integrations` table by workspace + provider
3. **Create/Update Integration Record**:
   - New: Create `integrations` row + `integration_credentials` row (type: `api_key`)
   - Existing: Update token if changed
4. **Generate Webhook Secret**: Create `integration_credentials` row (type: `webhook_secret`)
5. **Create Webhooks**: For each project not already synced:
   - Call GitLab API: `POST /projects/:id/hooks`
   - Events enabled: `issues_events`, `merge_requests_events`, `note_events`
6. **Store Repository Records**: Create `repositories` row for each successful webhook
7. **Store Webhook Config**: Create `integration_configs` row for each project (type: `webhook`)
8. **Store Sync Status**: Create/update `integration_configs` row (key: `gitlab_sync_status`, type: `state`)

```go
type SetupIntegrationParams struct {
    InstanceURL    string
    Token          string
    WebhookBaseURL string
    WorkspaceID    int64
    OrganizationID int64
    SetupByUserID  int64
}

type SetupResult struct {
    IntegrationID     int64
    IsNewIntegration  bool
    Projects          []GitLabProject
    WebhooksCreated   int
    RepositoriesAdded int
    Errors            []string
}
```

#### Webhook URL Format

```
{webhookBaseURL}/webhooks/gitlab/{integration_id}
```

Example: `https://relay.example.com/webhooks/gitlab/1234567890`

### Webhook Handler

**Location**: `relay/internal/http/handler/webhook/gitlab.go`

```go
type GitLabWebhookHandler struct {
    credentialStore store.IntegrationCredentialStore
    logger          *slog.Logger
}
```

**Verification Flow**:
1. Parse `integration_id` from URL path
2. Get `X-Gitlab-Token` header
3. Look up `webhook_secret` credential for integration
4. Compare header value with stored secret
5. Parse payload and log event

**Payload Structure**:
```go
type gitlabWebhookPayload struct {
    ObjectKind       string `json:"object_kind"`
    EventType        string `json:"event_type"`
    ObjectAttributes struct {
        ID     int64  `json:"id"`
        IID    int64  `json:"iid"`
        Title  string `json:"title"`
        Note   string `json:"note"`
        Action string `json:"action"`
    } `json:"object_attributes"`
}
```

---

## Data Layer

### Database Tables

#### `integrations`

Main integration record. One per workspace per provider.

| Column | Type | Description |
|--------|------|-------------|
| `id` | `bigint` | Snowflake ID (PK) |
| `workspace_id` | `bigint` | FK to workspaces |
| `organization_id` | `bigint` | FK to organizations |
| `setup_by_user_id` | `bigint` | FK to users (who set it up) |
| `provider` | `text` | `'gitlab'` |
| `capabilities` | `text[]` | `['code_repo', 'issue_tracker', 'wiki']` |
| `provider_base_url` | `text` | GitLab instance URL (e.g., `https://gitlab.com`) |
| `external_org_id` | `text` | Not used for GitLab PAT flow |
| `external_workspace_id` | `text` | Not used for GitLab PAT flow |
| `is_enabled` | `boolean` | Whether integration is active |
| `created_at` | `timestamptz` | Creation timestamp |
| `updated_at` | `timestamptz` | Last update timestamp |

**Unique Constraint**: `(workspace_id, provider)`

#### `integration_credentials`

Stores authentication tokens. GitLab integration creates two credential records:

| Column | Type | Description |
|--------|------|-------------|
| `id` | `bigint` | Snowflake ID (PK) |
| `integration_id` | `bigint` | FK to integrations |
| `user_id` | `bigint` | FK to users (nullable, set for PAT) |
| `credential_type` | `text` | `'api_key'` or `'webhook_secret'` |
| `access_token` | `text` | The PAT or webhook secret value |
| `refresh_token` | `text` | Not used for GitLab |
| `token_expires_at` | `timestamptz` | Not used for GitLab |
| `scopes` | `text[]` | Not populated for GitLab |
| `is_primary` | `boolean` | `true` for the PAT, `false` for webhook secret |
| `created_at` | `timestamptz` | Creation timestamp |
| `updated_at` | `timestamptz` | Last update timestamp |
| `revoked_at` | `timestamptz` | Soft-delete timestamp |

**GitLab creates**:
1. **PAT credential**: `credential_type='api_key'`, `is_primary=true`, `user_id=setup_by_user_id`
2. **Webhook secret**: `credential_type='webhook_secret'`, `is_primary=false`, `user_id=null`

#### `integration_configs`

Stores per-project webhook metadata and sync status.

| Column | Type | Description |
|--------|------|-------------|
| `id` | `bigint` | Snowflake ID (PK) |
| `integration_id` | `bigint` | FK to integrations |
| `key` | `text` | Project external ID or `'gitlab_sync_status'` |
| `value` | `jsonb` | Configuration JSON |
| `config_type` | `text` | `'webhook'` or `'state'` |
| `created_at` | `timestamptz` | Creation timestamp |
| `updated_at` | `timestamptz` | Last update timestamp |

**Config Types**:

1. **Webhook config** (`config_type='webhook'`, `key=<project_id>`):
```json
{
  "webhook_id": 12345,
  "project_path": "group/project",
  "events": ["issues_events", "merge_requests_events", "note_events"]
}
```

2. **Sync status** (`config_type='state'`, `key='gitlab_sync_status'`):
```json
{
  "synced": true,
  "webhooks_created": 5,
  "repositories_added": 5,
  "errors": [],
  "updated_at": "2025-01-15T10:30:00Z"
}
```

#### `repositories`

Stores synced GitLab projects.

| Column | Type | Description |
|--------|------|-------------|
| `id` | `bigint` | Snowflake ID (PK) |
| `workspace_id` | `bigint` | FK to workspaces |
| `integration_id` | `bigint` | FK to integrations |
| `name` | `text` | Project name |
| `slug` | `text` | Full path (e.g., `group/subgroup/project`) |
| `url` | `text` | Project web URL |
| `description` | `text` | Project description (nullable) |
| `external_repo_id` | `text` | GitLab project ID as string |
| `created_at` | `timestamptz` | Creation timestamp |
| `updated_at` | `timestamptz` | Last update timestamp |

**Unique Constraint**: `(integration_id, external_repo_id)`

### Store Interfaces

**Location**: `relay/internal/store/interfaces.go`

```go
type IntegrationStore interface {
    GetByID(ctx context.Context, id int64) (*model.Integration, error)
    GetByWorkspaceAndProvider(ctx context.Context, workspaceID int64, provider model.Provider) (*model.Integration, error)
    Create(ctx context.Context, integration *model.Integration) error
    Update(ctx context.Context, integration *model.Integration) error
    SetEnabled(ctx context.Context, id int64, enabled bool) error
    Delete(ctx context.Context, id int64) error
    ListByWorkspace(ctx context.Context, workspaceID int64) ([]model.Integration, error)
    ListByOrganization(ctx context.Context, orgID int64) ([]model.Integration, error)
    ListByCapability(ctx context.Context, workspaceID int64, capability model.Capability) ([]model.Integration, error)
}

type IntegrationCredentialStore interface {
    GetByID(ctx context.Context, id int64) (*model.IntegrationCredential, error)
    GetPrimaryByIntegration(ctx context.Context, integrationID int64) (*model.IntegrationCredential, error)
    GetByIntegrationAndUser(ctx context.Context, integrationID int64, userID int64) (*model.IntegrationCredential, error)
    Create(ctx context.Context, cred *model.IntegrationCredential) error
    UpdateTokens(ctx context.Context, id int64, accessToken string, refreshToken *string, expiresAt *time.Time) error
    SetAsPrimary(ctx context.Context, integrationID int64, credentialID int64) error
    Revoke(ctx context.Context, id int64) error
    RevokeAllByIntegration(ctx context.Context, integrationID int64) error
    Delete(ctx context.Context, id int64) error
    ListByIntegration(ctx context.Context, integrationID int64) ([]model.IntegrationCredential, error)
    ListActiveByIntegration(ctx context.Context, integrationID int64) ([]model.IntegrationCredential, error)
}

type IntegrationConfigStore interface {
    GetByID(ctx context.Context, id int64) (*model.IntegrationConfig, error)
    GetByIntegrationAndKey(ctx context.Context, integrationID int64, key string) (*model.IntegrationConfig, error)
    ListByIntegration(ctx context.Context, integrationID int64) ([]model.IntegrationConfig, error)
    ListByIntegrationAndType(ctx context.Context, integrationID int64, configType string) ([]model.IntegrationConfig, error)
    Create(ctx context.Context, config *model.IntegrationConfig) error
    Update(ctx context.Context, config *model.IntegrationConfig) error
    Upsert(ctx context.Context, config *model.IntegrationConfig) error
    Delete(ctx context.Context, id int64) error
    DeleteByIntegration(ctx context.Context, integrationID int64) error
}

type RepoStore interface {
    GetByID(ctx context.Context, id int64) (*model.Repository, error)
    GetByExternalID(ctx context.Context, integrationID int64, externalRepoID string) (*model.Repository, error)
    Create(ctx context.Context, repo *model.Repository) error
    Update(ctx context.Context, repo *model.Repository) error
    Delete(ctx context.Context, id int64) error
    DeleteByIntegration(ctx context.Context, integrationID int64) error
    ListByWorkspace(ctx context.Context, workspaceID int64) ([]model.Repository, error)
    ListByIntegration(ctx context.Context, integrationID int64) ([]model.Repository, error)
}
```

### Domain Models

**Location**: `relay/internal/model/`

```go
// integration.go
type Integration struct {
    ID                  int64        `json:"id"`
    WorkspaceID         int64        `json:"workspace_id"`
    OrganizationID      int64        `json:"organization_id"`
    SetupByUserID       int64        `json:"setup_by_user_id"`
    Provider            Provider     `json:"provider"`
    Capabilities        []Capability `json:"capabilities"`
    ProviderBaseURL     *string      `json:"provider_base_url,omitempty"`
    ExternalOrgID       *string      `json:"external_org_id,omitempty"`
    ExternalWorkspaceID *string      `json:"external_workspace_id,omitempty"`
    IsEnabled           bool         `json:"is_enabled"`
    CreatedAt           time.Time    `json:"created_at"`
    UpdatedAt           time.Time    `json:"updated_at"`
}

// integration_credential.go
type IntegrationCredential struct {
    ID             int64          `json:"id"`
    IntegrationID  int64          `json:"integration_id"`
    UserID         *int64         `json:"user_id,omitempty"`
    CredentialType CredentialType `json:"credential_type"`
    AccessToken    string         `json:"-"`
    RefreshToken   *string        `json:"-"`
    TokenExpiresAt *time.Time     `json:"-"`
    Scopes         []string       `json:"scopes,omitempty"`
    IsPrimary      bool           `json:"is_primary"`
    RevokedAt      *time.Time     `json:"revoked_at,omitempty"`
    CreatedAt      time.Time      `json:"created_at"`
    UpdatedAt      time.Time      `json:"updated_at"`
}

// integration_config.go
type IntegrationConfig struct {
    ID            int64           `json:"id"`
    IntegrationID int64           `json:"integration_id"`
    Key           string          `json:"key"`
    Value         json.RawMessage `json:"value"`
    ConfigType    string          `json:"config_type"`
    CreatedAt     time.Time       `json:"created_at"`
    UpdatedAt     time.Time       `json:"updated_at"`
}

// repository.go
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
```

### Enums

```go
// Provider
const (
    ProviderGitLab Provider = "gitlab"
    ProviderGitHub Provider = "github"
    ProviderLinear Provider = "linear"
    ProviderJira   Provider = "jira"
    ProviderSlack  Provider = "slack"
    ProviderNotion Provider = "notion"
)

// Capability
const (
    CapabilityCodeRepo      Capability = "code_repo"
    CapabilityIssueTracker  Capability = "issue_tracker"
    CapabilityDocumentation Capability = "documentation"
    CapabilityCommunication Capability = "communication"
    CapabilityWiki          Capability = "wiki"
)

// CredentialType
const (
    CredentialTypeUserOAuth     CredentialType = "user_oauth"
    CredentialTypeBot           CredentialType = "bot"
    CredentialTypeAppInstall    CredentialType = "app_installation"
    CredentialTypeAPIKey        CredentialType = "api_key"
    CredentialTypeWebhookSecret CredentialType = "webhook_secret"
)
```

---

## GitLab API Calls

The service uses the official GitLab Go client (`gitlab.com/gitlab-org/api/client-go`).

### Client Initialization

```go
func (s *gitLabService) newClient(instanceURL, token string) (*gitlab.Client, error) {
    baseURL := strings.TrimSuffix(instanceURL, "/") + "/api/v4"
    return gitlab.NewClient(token, gitlab.WithBaseURL(baseURL))
}
```

### API Calls Made

| Operation | GitLab API | When |
|-----------|------------|------|
| List Projects | `GET /api/v4/projects?min_access_level=40` | Setup, Refresh |
| Add Webhook | `POST /api/v4/projects/:id/hooks` | Setup (per project) |

### Webhook Configuration

When creating webhooks via GitLab API:

```go
client.Projects.AddProjectHook(projectID, &gitlab.AddProjectHookOptions{
    URL:                   gitlab.Ptr(webhookURL),
    IssuesEvents:          gitlab.Ptr(true),
    MergeRequestsEvents:   gitlab.Ptr(true),
    NoteEvents:            gitlab.Ptr(true),
    Token:                 gitlab.Ptr(webhookSecret),
    EnableSSLVerification: gitlab.Ptr(true),
})
```

---

## Error Handling

### GitLab API Errors

The handler maps GitLab API errors to appropriate HTTP status codes:

```go
func mapGitLabError(err error) (int, string) {
    var gitlabErr *gitlab.ErrorResponse
    if errors.As(err, &gitlabErr) {
        switch gitlabErr.Response.StatusCode {
        case 401:
            return 401, "Invalid token..."
        case 403:
            return 403, "Token does not have sufficient permissions..."
        case 404:
            return 400, "GitLab instance not found..."
        }
    }
    // ... pattern matching on error messages
}
```

### Dashboard Error Formatting

The relay client formats errors for user display:

```typescript
function formatErrorMessage(error: string, status: number): string {
    if (error.includes('no projects found with maintainer access')) {
        return error
    }
    if (status === 401) {
        return 'Invalid token...'
    }
    // ...
}
```

---

## File Reference

| File | Purpose |
|------|---------|
| `dashboard/components/gitlab-connect-panel.tsx` | UI component for GitLab connection |
| `dashboard/lib/relay-client.ts` | Client-side API wrapper |
| `dashboard/app/api/integrations/gitlab/setup/route.ts` | Setup proxy route |
| `dashboard/app/api/integrations/gitlab/status/route.ts` | Status proxy route |
| `dashboard/app/api/integrations/gitlab/refresh/route.ts` | Refresh proxy route |
| `dashboard/app/dashboard/page.tsx` | Dashboard page using GitLab panel |
| `relay/internal/http/router/gitlab.go` | Route definitions |
| `relay/internal/http/dto/gitlab.go` | Request/response DTOs |
| `relay/internal/http/handler/gitlab.go` | HTTP handlers |
| `relay/internal/http/handler/webhook/gitlab.go` | Webhook handler |
| `relay/internal/service/integration/gitlab.go` | Business logic |
| `relay/internal/model/integration.go` | Integration domain model |
| `relay/internal/model/integration_credential.go` | Credential domain model |
| `relay/internal/model/integration_config.go` | Config domain model |
| `relay/internal/model/repository.go` | Repository domain model |
| `relay/internal/store/integration.go` | Integration store |
| `relay/internal/store/integration_credential.go` | Credential store |
| `relay/internal/store/integration_config.go` | Config store |
| `relay/internal/store/repo.go` | Repository store |
| `relay/core/db/queries/integrations.sql` | Integration SQL queries |
| `relay/core/db/queries/integration_credentials.sql` | Credential SQL queries |
| `relay/core/db/queries/integration_configs.sql` | Config SQL queries |
| `relay/core/db/queries/repositories.sql` | Repository SQL queries |
