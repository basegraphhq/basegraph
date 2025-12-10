# Integration Flows

This document describes the onboarding and integration flows for each supported provider.

## Overview

| Provider | Auth Method | Identity | Entities Synced |
|----------|-------------|----------|-----------------|
| GitLab | PAT (Service Account) | Bot user | Repositories |
| GitHub | GitHub App | App installation | Repositories |
| Linear | OAuth / Agent Install | Relay Agent | Issues, Projects |
| Jira | Atlassian Connect | App | Issues, Projects |

---

## GitLab Integration

GitLab uses a PAT-based flow with a service account. Admin creates a bot user in GitLab and provides its access token.

```mermaid
sequenceDiagram
    participant Admin
    participant Relay
    participant GitLab
    participant DB

    Note over Admin,GitLab: 1. Sign In to Relay
    Admin->>Relay: Visit dashboard
    Relay->>GitLab: OAuth redirect (read_user)
    GitLab-->>Admin: Authorize?
    Admin->>GitLab: Approve
    GitLab-->>Relay: Auth code
    Relay->>GitLab: Exchange for token
    GitLab-->>Relay: User info
    Relay->>DB: Create/update user, session
    Relay-->>Admin: Dashboard

    Note over Admin,GitLab: 2. Create Bot (outside Relay)
    Admin->>GitLab: Create relay-bot user
    Admin->>GitLab: Add to groups (Developer+)
    Admin->>GitLab: Create PAT (api scope)
    GitLab-->>Admin: glpat-xxx...

    Note over Admin,DB: 3. Connect Integration
    Admin->>Relay: Paste PAT + GitLab URL
    Relay->>GitLab: GET /api/v4/user (validate)
    GitLab-->>Relay: Bot user info
    Relay->>DB: Create integration + credential
    Relay->>GitLab: GET /api/v4/projects?membership=true
    GitLab-->>Relay: Project list
    Relay-->>Admin: Show projects

    Note over Admin,DB: 4. Select & Setup
    Admin->>Relay: Select projects to sync
    Relay->>DB: Store in repositories table
    
    loop Each project
        Relay->>GitLab: POST /projects/:id/hooks
        GitLab-->>Relay: Webhook created (id: 123)
        Relay->>DB: Store webhook config
    end

    Note over Relay,DB: 5. Index Code
    loop Each repository
        Relay->>GitLab: Clone/fetch via API
        Relay->>Relay: Parse code, extract graph
        Relay->>DB: Store code graph
    end

    Relay-->>Admin: Setup complete ✓

    Note over GitLab,Relay: 6. Ongoing: Webhook Events
    GitLab->>Relay: Push event
    GitLab->>Relay: Issue created
    GitLab->>Relay: MR opened
    Relay->>DB: Store in event_logs

    Note over Relay,GitLab: 7. Relay Actions
    Relay->>GitLab: POST /issues/:id/notes (comment)
    Note right of GitLab: Shows as @relay-bot
```

### Credential Storage

| Field | Value |
|-------|-------|
| `credential_type` | `personal_access_token` |
| `user_id` | `null` (bot token) |
| `scopes` | `['api']` |

---

## GitHub Integration

GitHub uses GitHub Apps - the recommended integration method. Admin installs the Relay app on their organization.

```mermaid
sequenceDiagram
    participant Admin
    participant Relay
    participant GitHub
    participant DB

    Note over Admin,Relay: 1. Sign In to Relay
    Admin->>Relay: Visit dashboard
    Relay-->>Admin: Dashboard (already signed in)

    Note over Admin,GitHub: 2. Install GitHub App
    Admin->>Relay: Click 'Connect GitHub'
    Relay->>GitHub: Redirect to app install
    GitHub-->>Admin: "Install Relay on which org?"
    Admin->>GitHub: Select organization
    GitHub-->>Admin: "Select repositories"
    Admin->>GitHub: All repos / Select repos
    GitHub-->>Admin: Confirm permissions
    Admin->>GitHub: Install
    GitHub->>Relay: Installation callback (installation_id)
    Relay->>DB: Store integration + installation_id

    Note over Relay,GitHub: 3. Generate Installation Token
    Relay->>Relay: Sign JWT with app private key
    Relay->>GitHub: POST /app/installations/:id/access_tokens
    GitHub-->>Relay: Installation token (expires 1hr)
    Relay->>DB: Cache token

    Note over Relay,GitHub: 4. Fetch Repositories
    Relay->>GitHub: GET /installation/repositories
    GitHub-->>Relay: Repository list
    Relay->>DB: Store in repositories table
    Relay-->>Admin: Show connected repos

    Note over GitHub,Relay: 5. Webhooks (Automatic)
    Note right of GitHub: GitHub Apps have built-in webhooks
    GitHub->>Relay: push event
    GitHub->>Relay: issues event
    GitHub->>Relay: pull_request event
    Relay->>DB: Store in event_logs

    Note over Relay,DB: 6. Index Code
    loop Each repository
        Relay->>GitHub: GET /repos/:owner/:repo/contents
        Relay->>Relay: Parse code, extract graph
        Relay->>DB: Store code graph
    end

    Relay-->>Admin: Setup complete ✓

    Note over Relay,GitHub: 7. Relay Actions
    Relay->>GitHub: POST /repos/:owner/:repo/issues/:id/comments
    Note right of GitHub: Shows as "Relay[bot]"

    Note over Relay,GitHub: 8. Token Refresh (hourly)
    Relay->>GitHub: POST /app/installations/:id/access_tokens
    GitHub-->>Relay: New token
    Relay->>DB: Update cached token
```

### Credential Storage

| Field | Value |
|-------|-------|
| `credential_type` | `app_installation` |
| `user_id` | `null` |
| `access_token` | Installation token (cached, refreshed hourly) |
| `scopes` | Defined by app permissions |

### GitHub App Setup (One-time, Relay Admin)

1. Create GitHub App in Relay's GitHub account
2. Configure permissions: `contents: read`, `issues: write`, `pull_requests: write`, `webhooks: read`
3. Store app private key securely (not in DB)
4. App ID and private key used to generate installation tokens

---

## Linear Integration

Linear uses OAuth with an Agent model. The installed agent becomes a first-class participant in the workspace.

```mermaid
sequenceDiagram
    participant Admin
    participant Relay
    participant Linear
    participant DB

    Note over Admin,Relay: 1. Sign In to Relay
    Admin->>Relay: Visit dashboard
    Relay-->>Admin: Dashboard

    Note over Admin,Linear: 2. Connect Linear
    Admin->>Relay: Click 'Connect Linear'
    Relay->>Linear: OAuth redirect
    Linear-->>Admin: "Authorize Relay?"
    Admin->>Linear: Select workspace
    Linear-->>Admin: "Grant access to which teams?"
    Admin->>Linear: Select teams
    Admin->>Linear: Authorize
    Linear->>Relay: Callback with auth code
    Relay->>Linear: Exchange code for token
    Linear-->>Relay: Access token + workspace info
    Relay->>DB: Create integration + credential

    Note over Relay,Linear: 3. Fetch Workspace Data
    Relay->>Linear: GraphQL: teams, projects
    Linear-->>Relay: Teams and projects
    Relay-->>Admin: Show workspace structure

    Note over Admin,DB: 4. Configure Sync
    Admin->>Relay: Select teams to monitor
    Admin->>Relay: Configure: issues, comments, status
    Relay->>DB: Store config

    Note over Relay,Linear: 5. Subscribe to Webhooks
    Relay->>Linear: Create webhook subscription
    Linear-->>Relay: Webhook ID
    Relay->>DB: Store webhook config

    Note over Linear,Relay: 6. Ongoing: Webhook Events
    Linear->>Relay: Issue created
    Linear->>Relay: Comment added
    Linear->>Relay: Issue status changed
    Linear->>Relay: Issue assigned
    Relay->>DB: Store in event_logs

    Note over Relay,Linear: 7. Relay Actions
    Relay->>Linear: GraphQL: createComment
    Note right of Linear: Shows as "Relay" agent
    Relay->>Linear: GraphQL: updateIssue
    Note right of Linear: Relay can be @mentioned
```

### Credential Storage

| Field | Value |
|-------|-------|
| `credential_type` | `app_installation` or `oauth` |
| `user_id` | `null` |
| `access_token` | OAuth token |
| `refresh_token` | If provided |
| `scopes` | `['read', 'write', 'issues:create', ...]` |

### Linear Agent Capabilities

- Can be @mentioned in issues and comments
- Can be assigned issues
- Actions attributed to "Relay" (not a person)
- Appears in workspace member list as an agent

---

## Jira Integration

Jira uses Atlassian Connect apps or OAuth 2.0 (3LO). Connect apps are recommended for deeper integration.

```mermaid
sequenceDiagram
    participant Admin
    participant Relay
    participant Atlassian
    participant Jira
    participant DB

    Note over Admin,Relay: 1. Sign In to Relay
    Admin->>Relay: Visit dashboard
    Relay-->>Admin: Dashboard

    Note over Admin,Atlassian: 2. Install Atlassian Connect App
    Admin->>Relay: Click 'Connect Jira'
    Relay-->>Admin: "Go to Atlassian Marketplace"
    Admin->>Atlassian: Search for "Relay"
    Atlassian-->>Admin: Relay app listing
    Admin->>Atlassian: Click Install
    Atlassian-->>Admin: "Install on which site?"
    Admin->>Atlassian: Select Jira site
    Atlassian-->>Admin: "Grant permissions?"
    Admin->>Atlassian: Approve
    Atlassian->>Relay: App installed callback (site_id, shared_secret)
    Relay->>DB: Create integration + credentials

    Note over Relay,Jira: 3. Establish Connection
    Relay->>Relay: Generate JWT with shared_secret
    Relay->>Jira: GET /rest/api/3/myself (validate)
    Jira-->>Relay: App info confirmed
    
    Note over Relay,Jira: 4. Fetch Projects
    Relay->>Jira: GET /rest/api/3/project
    Jira-->>Relay: Project list
    Relay->>DB: Store projects
    Relay-->>Admin: Show Jira projects

    Note over Admin,DB: 5. Configure Sync
    Admin->>Relay: Select projects to monitor
    Admin->>Relay: Configure: issues, comments, transitions
    Relay->>DB: Store config

    Note over Relay,Jira: 6. Register Webhooks
    Relay->>Jira: POST /rest/api/3/webhook
    Jira-->>Relay: Webhook registered
    Relay->>DB: Store webhook config

    Note over Jira,Relay: 7. Ongoing: Webhook Events
    Jira->>Relay: jira:issue_created
    Jira->>Relay: jira:issue_updated
    Jira->>Relay: comment_created
    Relay->>DB: Store in event_logs

    Note over Relay,Jira: 8. Relay Actions
    Relay->>Jira: POST /rest/api/3/issue/:id/comment
    Note right of Jira: Shows as "Relay for Jira"
    Relay->>Jira: PUT /rest/api/3/issue/:id
    Note right of Jira: Can update issue fields
```

### Alternative: OAuth 2.0 (3LO)

For simpler integrations without Marketplace listing:

```mermaid
sequenceDiagram
    participant Admin
    participant Relay
    participant Atlassian
    participant DB

    Admin->>Relay: Click 'Connect Jira'
    Relay->>Atlassian: OAuth redirect
    Atlassian-->>Admin: "Authorize Relay?"
    Admin->>Atlassian: Select site + approve
    Atlassian->>Relay: Auth code
    Relay->>Atlassian: Exchange for tokens
    Atlassian-->>Relay: Access token + refresh token
    Relay->>DB: Store credentials
    Relay-->>Admin: Connected ✓
```

### Credential Storage

**Connect App:**
| Field | Value |
|-------|-------|
| `credential_type` | `app_installation` |
| `user_id` | `null` |
| `access_token` | Shared secret (for JWT signing) |

**OAuth 2.0 (3LO):**
| Field | Value |
|-------|-------|
| `credential_type` | `oauth` |
| `user_id` | `null` |
| `access_token` | OAuth access token |
| `refresh_token` | OAuth refresh token |

---

## Comparison Summary

```mermaid
flowchart LR
    subgraph GitLab
        GL1[Admin creates bot] --> GL2[Generate PAT]
        GL2 --> GL3[Paste in Relay]
        GL3 --> GL4[Create webhooks per project]
    end

    subgraph GitHub
        GH1[Admin clicks Install] --> GH2[Select org/repos]
        GH2 --> GH3[GitHub handles webhooks]
        GH3 --> GH4[Relay generates tokens]
    end

    subgraph Linear
        LN1[Admin clicks Connect] --> LN2[OAuth + team selection]
        LN2 --> LN3[Agent identity created]
        LN3 --> LN4[Workspace-level webhooks]
    end

    subgraph Jira
        JR1[Admin installs from Marketplace] --> JR2[Select site]
        JR2 --> JR3[App identity created]
        JR3 --> JR4[Register webhooks]
    end
```

| Aspect | GitLab | GitHub | Linear | Jira |
|--------|--------|--------|--------|------|
| **Setup steps** | 4 (create bot, PAT, paste, select) | 2 (install, select repos) | 2 (OAuth, select teams) | 2 (install, select projects) |
| **Token management** | Manual (PAT doesn't expire or has long expiry) | Automatic (hourly refresh) | OAuth refresh | Connect: JWT / OAuth: refresh |
| **Webhook setup** | Manual per-project | Automatic | Workspace-level | Manual registration |
| **Bot identity** | You create it | GitHub creates it | Linear creates it | Atlassian creates it |
| **Enterprise tier required** | No (PAT) / Yes (Group Token) | No | No | No |

---

## Database Schema Mapping

All integrations use the same schema:

```sql
-- integrations table
integration.provider            -- 'gitlab', 'github', 'linear', 'jira', 'slack', 'notion'
integration.capabilities        -- text[] e.g., {'code_repo', 'issue_tracker', 'wiki'}
integration.setup_by_user_id    -- Relay admin who configured it
integration.external_org_id     -- GitLab group, GitHub org, Linear workspace, Jira site

-- integration_credentials table
credential.credential_type      -- 'personal_access_token', 'app_installation', 'oauth'
credential.user_id              -- null for all (bot/app tokens)
credential.access_token         -- PAT, installation token, OAuth token, or shared secret
credential.refresh_token        -- For OAuth flows
credential.scopes               -- Granted permissions
```

### Capabilities

Providers can have multiple capabilities (e.g., GitLab is both code_repo and issue_tracker):

| Capability | Providers | Description |
|------------|-----------|-------------|
| `code_repo` | GitLab, GitHub | Source code, MRs/PRs, code reviews |
| `issue_tracker` | GitLab, GitHub, Linear, Jira | Issues, projects, sprints |
| `documentation` | Notion | Docs, knowledge base |
| `wiki` | GitLab, GitHub, Notion | Wiki pages |
| `communication` | Slack | Messages, threads, channels |

### Provider Capabilities

| Provider | Capabilities |
|----------|-------------|
| GitLab | `code_repo`, `issue_tracker`, `wiki` |
| GitHub | `code_repo`, `issue_tracker`, `wiki` |
| Linear | `issue_tracker` |
| Jira | `issue_tracker` |
| Slack | `communication` |
| Notion | `documentation`, `wiki` |

---

## Security Considerations

1. **Token storage**: All tokens encrypted at rest
2. **Webhook verification**: Each provider has a signature mechanism
   - GitLab: `X-Gitlab-Token` header
   - GitHub: `X-Hub-Signature-256` header (HMAC)
   - Linear: Webhook signing secret
   - Jira: JWT verification with shared secret
3. **Least privilege**: Request only necessary scopes
4. **Token refresh**: Handle expiration gracefully, refresh before expiry

