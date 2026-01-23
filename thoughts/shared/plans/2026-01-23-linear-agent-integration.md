# Linear Agent Integration - Implementation Plan

## Overview

Implement Linear's Agent API integration for Basegraph, enabling first-class agent presence in Linear workspaces. Users will @mention Basegraph or delegate issues directly, and see real-time thoughts, actions, and progress natively in Linear's UI.

This integration uses Linear's **Agent Session** paradigm rather than standard webhooks, providing full transparency into Basegraph's reasoning process.

## Current State Analysis

### What Exists
- `ProviderLinear` defined in `relay/internal/model/integration.go:10` with `CapabilityIssueTracker`
- Stub mapper at `relay/internal/mapper/linear_mapper.go` (returns "not implemented")
- Comment limit configured at `relay/internal/brain/action_executor.go:23` (64KB)
- UI assets ready: `dashboard/assets/linear-icon.svg`, `linear-icon.png`
- Demo reference: `demos/lendflow/.../webhooks/linear.py`

### What's Missing
- Linear OAuth application setup flow
- Webhook handler for `AgentSessionEvent`
- GraphQL client for Linear API
- `AgentSession` and `AgentActivity` models
- Activity emission hooks in brain orchestrator/planner
- `LinearAgentService` implementing session/activity mutations

### Key Discoveries
- Brain orchestrator is batch-oriented (`relay/internal/brain/orchestrator.go:274-329`)
- Detailed metrics already tracked in `PlannerMetrics` and `ExploreMetrics`
- Debug logs prove granular progress data exists but isn't exposed
- Credential store supports OAuth with refresh tokens (`model/integration_credential.go:15-28`)
- Integration configs can store arbitrary JSON metadata

## Desired End State

After implementation:

1. **Users can install Basegraph** as a Linear OAuth application in their workspace
2. **Basegraph appears as a workspace member** that can be @mentioned or assigned issues
3. **When triggered**, Basegraph:
   - Immediately acknowledges (within 10 seconds per Linear AIG)
   - Emits `thought` activities showing reasoning ("Analyzing issue description...")
   - Emits `action` activities showing tool usage ("Searching codebase for auth handlers...")
   - Updates session `plan` with checklist items and progress
   - Posts final `response` when complete
4. **Users see all progress** in Linear's native agent UI - no need to check external dashboards
5. **Sessions are tracked** and can be resumed if users provide follow-up prompts

### Verification
- [ ] OAuth flow completes and stores valid credentials
- [ ] AgentSessionEvent webhooks are received and validated
- [ ] Activities appear in Linear UI within 2 seconds of emission
- [ ] Session states transition correctly (pending → active → complete)
- [ ] Existing GitLab integration continues working unchanged

## What We're NOT Doing

- **Standard data-change webhooks** (Issue created/updated) - focusing on Agent API only
- **GitHub integration** - separate effort
- **Jira integration** - separate effort
- **Dashboard UI changes** - Linear's UI shows activities natively
- **Bi-directional sync** - Basegraph reads from Linear, writes activities, but doesn't sync issue state back

---

## Implementation Approach

**Strategy**: Layer Linear-specific capabilities onto existing architecture without disrupting current flows.

1. **Phase 1**: Foundation - OAuth, webhook handler, GraphQL client
2. **Phase 2**: Session Management - track sessions, emit basic activities
3. **Phase 3**: Full Transparency - granular activity emission throughout brain
4. **Phase 4**: Polish - error handling, recovery, monitoring

---

## Phase 1: Foundation

### Overview
Set up Linear OAuth application flow, webhook endpoint, and GraphQL client. After this phase, Basegraph can receive Linear webhooks and make authenticated API calls.

### Changes Required

#### 1.1 Add GraphQL Client Dependency

**File**: `relay/go.mod`
**Changes**: Add Linear GraphQL client dependency

```bash
go get github.com/hasura/go-graphql-client
```

#### 1.2 Linear Provider Constants

**File**: `relay/internal/model/integration.go`
**Changes**: Add Linear-specific constants (provider already exists)

```go
// Add after line 47 (after ProviderDefaultCapabilities)

// LinearOAuthScopes defines required scopes for Linear agent integration
var LinearOAuthScopes = []string{
    "read",
    "write",
    "app:assignable",    // Enable issue delegation
    "app:mentionable",   // Enable @mentions
}
```

#### 1.3 Linear GraphQL Client

**File**: `relay/internal/linear/client.go` (new file)
**Changes**: Create Linear API client wrapper

```go
package linear

import (
    "context"
    "fmt"
    "net/http"

    "github.com/hasura/go-graphql-client"
)

const (
    LinearAPIEndpoint = "https://api.linear.app/graphql"
)

// Client wraps Linear's GraphQL API
type Client struct {
    gql *graphql.Client
}

// NewClient creates a Linear API client with the given access token
func NewClient(accessToken string) *Client {
    httpClient := &http.Client{
        Transport: &authTransport{
            token:     accessToken,
            transport: http.DefaultTransport,
        },
    }

    return &Client{
        gql: graphql.NewClient(LinearAPIEndpoint, httpClient),
    }
}

type authTransport struct {
    token     string
    transport http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    req.Header.Set("Authorization", t.token)
    req.Header.Set("Content-Type", "application/json")
    return t.transport.RoundTrip(req)
}

// Issue fetches an issue by ID
func (c *Client) Issue(ctx context.Context, issueID string) (*Issue, error) {
    var query struct {
        Issue Issue `graphql:"issue(id: $id)"`
    }

    variables := map[string]any{
        "id": graphql.ID(issueID),
    }

    if err := c.gql.Query(ctx, &query, variables); err != nil {
        return nil, fmt.Errorf("fetching issue: %w", err)
    }

    return &query.Issue, nil
}

// CreateAgentActivity emits an activity for an agent session
func (c *Client) CreateAgentActivity(ctx context.Context, input AgentActivityCreateInput) (*AgentActivity, error) {
    var mutation struct {
        AgentActivityCreate struct {
            Success       bool
            AgentActivity AgentActivity
        } `graphql:"agentActivityCreate(input: $input)"`
    }

    variables := map[string]any{
        "input": input,
    }

    if err := c.gql.Mutate(ctx, &mutation, variables); err != nil {
        return nil, fmt.Errorf("creating agent activity: %w", err)
    }

    if !mutation.AgentActivityCreate.Success {
        return nil, fmt.Errorf("agent activity creation failed")
    }

    return &mutation.AgentActivityCreate.AgentActivity, nil
}

// UpdateAgentSession updates session state, plan, or external URLs
func (c *Client) UpdateAgentSession(ctx context.Context, id string, input AgentSessionUpdateInput) (*AgentSession, error) {
    var mutation struct {
        AgentSessionUpdate struct {
            Success      bool
            AgentSession AgentSession
        } `graphql:"agentSessionUpdate(id: $id, input: $input)"`
    }

    variables := map[string]any{
        "id":    graphql.ID(id),
        "input": input,
    }

    if err := c.gql.Mutate(ctx, &mutation, variables); err != nil {
        return nil, fmt.Errorf("updating agent session: %w", err)
    }

    return &mutation.AgentSessionUpdate.AgentSession, nil
}
```

#### 1.4 Linear GraphQL Types

**File**: `relay/internal/linear/types.go` (new file)
**Changes**: Define GraphQL schema types

```go
package linear

import "time"

// AgentSession represents a Linear agent session
type AgentSession struct {
    ID            string     `graphql:"id"`
    Status        string     `graphql:"status"`
    StartedAt     *time.Time `graphql:"startedAt"`
    EndedAt       *time.Time `graphql:"endedAt"`
    PromptContext string     `graphql:"promptContext"`
    Plan          string     `graphql:"plan"` // JSON string
    Issue         *Issue     `graphql:"issue"`
}

// AgentActivity represents an activity within a session
type AgentActivity struct {
    ID      string `graphql:"id"`
    Content string `graphql:"content"` // JSON string
}

// AgentActivityCreateInput for creating activities
type AgentActivityCreateInput struct {
    AgentSessionID string `json:"agentSessionId"`
    Content        any    `json:"content"` // ActivityContent union
}

// AgentSessionUpdateInput for updating sessions
type AgentSessionUpdateInput struct {
    Plan            *string          `json:"plan,omitempty"`            // JSON string
    ExternalURLs    []ExternalURL    `json:"externalUrls,omitempty"`
    AddedExternalURLs   []ExternalURL `json:"addedExternalUrls,omitempty"`
    RemovedExternalURLs []string      `json:"removedExternalUrls,omitempty"`
}

// ExternalURL for linking to external resources
type ExternalURL struct {
    Label string `json:"label"`
    URL   string `json:"url"`
}

// Issue represents a Linear issue
type Issue struct {
    ID          string   `graphql:"id"`
    Identifier  string   `graphql:"identifier"` // e.g., "ENG-123"
    Title       string   `graphql:"title"`
    Description string   `graphql:"description"`
    URL         string   `graphql:"url"`
    State       State    `graphql:"state"`
    Labels      []Label  `graphql:"labels"`
    Assignee    *User    `graphql:"assignee"`
}

// State represents issue state
type State struct {
    ID   string `graphql:"id"`
    Name string `graphql:"name"`
    Type string `graphql:"type"` // backlog, unstarted, started, completed, canceled
}

// Label represents an issue label
type Label struct {
    ID   string `graphql:"id"`
    Name string `graphql:"name"`
}

// User represents a Linear user
type User struct {
    ID          string `graphql:"id"`
    Name        string `graphql:"name"`
    DisplayName string `graphql:"displayName"`
}

// Activity content types

// ThoughtContent for internal reasoning
type ThoughtContent struct {
    Type string `json:"type"` // "thought"
    Body string `json:"body"`
}

// ActionContent for tool invocations
type ActionContent struct {
    Type       string `json:"type"` // "action"
    Action     string `json:"action"`
    Parameters any    `json:"parameters,omitempty"`
    Result     any    `json:"result,omitempty"`
}

// ResponseContent for final responses
type ResponseContent struct {
    Type string `json:"type"` // "response"
    Body string `json:"body"`
}

// ErrorContent for failures
type ErrorContent struct {
    Type string `json:"type"` // "error"
    Body string `json:"body"`
}

// ElicitationContent for requesting user input
type ElicitationContent struct {
    Type string `json:"type"` // "elicitation"
    Body string `json:"body"`
}
```

#### 1.5 Linear Webhook Handler

**File**: `relay/internal/http/handler/webhook/linear.go` (new file)
**Changes**: Create webhook handler for AgentSessionEvent

```go
package webhook

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "io"
    "log/slog"
    "net/http"
    "strconv"
    "time"

    "github.com/basegraph/relay/internal/logger"
    "github.com/basegraph/relay/internal/mapper"
    "github.com/basegraph/relay/internal/model"
    "github.com/basegraph/relay/internal/service"
    "github.com/go-chi/chi/v5"
)

const (
    // LinearSignatureHeader contains HMAC-SHA256 signature
    LinearSignatureHeader = "Linear-Signature"
    // LinearEventHeader contains the event type
    LinearEventHeader = "Linear-Event"
    // LinearDeliveryHeader contains unique delivery ID
    LinearDeliveryHeader = "Linear-Delivery"
    // Maximum timestamp drift for replay protection (60 seconds)
    maxTimestampDrift = 60 * time.Second
)

// LinearWebhookHandler handles webhooks from Linear
type LinearWebhookHandler struct {
    credentialService service.IntegrationCredentialService
    eventIngest       service.EventIngestService
    mapper            mapper.EventMapper
}

// NewLinearWebhookHandler creates a new Linear webhook handler
func NewLinearWebhookHandler(
    credentialService service.IntegrationCredentialService,
    eventIngest service.EventIngestService,
    mapper mapper.EventMapper,
) *LinearWebhookHandler {
    return &LinearWebhookHandler{
        credentialService: credentialService,
        eventIngest:       eventIngest,
        mapper:            mapper,
    }
}

// Handle processes incoming Linear webhooks
func (h *LinearWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    ctx = logger.WithLogFields(ctx, "component", "relay.http.webhook.linear")

    // Extract integration ID from URL
    integrationIDStr := chi.URLParam(r, "integration_id")
    integrationID, err := strconv.ParseInt(integrationIDStr, 10, 64)
    if err != nil {
        slog.WarnContext(ctx, "invalid integration_id",
            "integration_id", integrationIDStr,
            "error", err)
        http.Error(w, "invalid integration_id", http.StatusBadRequest)
        return
    }
    ctx = logger.WithLogFields(ctx, "integration_id", integrationID)

    // Read body for signature verification
    body, err := io.ReadAll(r.Body)
    if err != nil {
        slog.ErrorContext(ctx, "failed to read request body", "error", err)
        http.Error(w, "failed to read body", http.StatusBadRequest)
        return
    }

    // Validate signature
    signature := r.Header.Get(LinearSignatureHeader)
    if signature == "" {
        slog.WarnContext(ctx, "missing Linear-Signature header")
        http.Error(w, "missing signature", http.StatusUnauthorized)
        return
    }

    if err := h.validateSignature(ctx, integrationID, body, signature); err != nil {
        slog.WarnContext(ctx, "signature validation failed", "error", err)
        http.Error(w, "invalid signature", http.StatusUnauthorized)
        return
    }

    // Parse payload
    var payload linearWebhookPayload
    if err := json.Unmarshal(body, &payload); err != nil {
        slog.ErrorContext(ctx, "failed to parse webhook payload", "error", err)
        http.Error(w, "invalid payload", http.StatusBadRequest)
        return
    }

    // Validate timestamp for replay protection
    if payload.WebhookTimestamp > 0 {
        webhookTime := time.UnixMilli(payload.WebhookTimestamp)
        if time.Since(webhookTime).Abs() > maxTimestampDrift {
            slog.WarnContext(ctx, "webhook timestamp outside allowed drift",
                "webhook_time", webhookTime,
                "drift", time.Since(webhookTime))
            http.Error(w, "timestamp too old", http.StatusBadRequest)
            return
        }
    }

    slog.DebugContext(ctx, "received Linear webhook",
        "event_type", r.Header.Get(LinearEventHeader),
        "delivery_id", r.Header.Get(LinearDeliveryHeader),
        "action", payload.Action,
        "type", payload.Type)

    // Only handle AgentSession events
    if payload.Type != "AgentSession" {
        slog.InfoContext(ctx, "ignoring non-agent-session event",
            "type", payload.Type)
        w.WriteHeader(http.StatusOK)
        return
    }

    // Process the agent session event
    if err := h.processAgentSessionEvent(ctx, integrationID, payload, body); err != nil {
        slog.ErrorContext(ctx, "failed to process agent session event",
            "error", err)
        http.Error(w, "processing failed", http.StatusInternalServerError)
        return
    }

    slog.InfoContext(ctx, "processed Linear agent session event",
        "action", payload.Action,
        "session_id", payload.AgentSession.ID,
        "issue_id", payload.AgentSession.Issue.ID)

    w.WriteHeader(http.StatusOK)
}

func (h *LinearWebhookHandler) validateSignature(ctx context.Context, integrationID int64, body []byte, signature string) error {
    secret, err := h.credentialService.GetWebhookSecret(ctx, integrationID)
    if err != nil {
        return fmt.Errorf("getting webhook secret: %w", err)
    }

    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write(body)
    expected := hex.EncodeToString(mac.Sum(nil))

    if !hmac.Equal([]byte(signature), []byte(expected)) {
        return fmt.Errorf("signature mismatch")
    }

    return nil
}

func (h *LinearWebhookHandler) processAgentSessionEvent(
    ctx context.Context,
    integrationID int64,
    payload linearWebhookPayload,
    rawBody []byte,
) error {
    // Map payload to body map for canonical event mapping
    var bodyMap map[string]any
    if err := json.Unmarshal(rawBody, &bodyMap); err != nil {
        return fmt.Errorf("unmarshaling body: %w", err)
    }

    // Map to canonical event type
    eventType, err := h.mapper.Map(ctx, bodyMap, map[string]string{
        LinearEventHeader: "AgentSession",
    })
    if err != nil {
        return fmt.Errorf("mapping event type: %w", err)
    }

    // Extract issue details
    issue := payload.AgentSession.Issue
    if issue.ID == "" {
        return fmt.Errorf("no issue in agent session event")
    }

    // Build ingest params
    params := service.EventIngestParams{
        IntegrationID:     integrationID,
        ExternalIssueID:   issue.ID,
        ExternalProjectID: "", // Linear doesn't have projects in the same way
        Provider:          model.ProviderLinear,
        IssueBody:         issue.Description,
        EventType:         eventType,
        Payload:           bodyMap,
        // Linear-specific: store session context
        LinearSessionID:   payload.AgentSession.ID,
        LinearPromptContext: payload.PromptContext,
    }

    // If this is a "prompted" action, extract the comment body
    if payload.Action == "prompted" && payload.AgentActivity != nil {
        params.CommentBody = payload.AgentActivity.Body
    }

    // Ingest the event
    result, err := h.eventIngest.Ingest(ctx, params)
    if err != nil {
        return fmt.Errorf("ingesting event: %w", err)
    }

    slog.InfoContext(ctx, "ingested Linear agent session event",
        "engaged", result.Engaged,
        "dedupe_key", result.DedupeKey,
        "issue_id", result.Issue.ID)

    return nil
}

// linearWebhookPayload represents the webhook payload structure
type linearWebhookPayload struct {
    Action           string                 `json:"action"` // "created" or "prompted"
    Type             string                 `json:"type"`   // "AgentSession"
    WebhookTimestamp int64                  `json:"webhookTimestamp"`
    WebhookID        string                 `json:"webhookId"`
    PromptContext    string                 `json:"promptContext"`
    Guidance         []any                  `json:"guidance"`
    AgentSession     agentSessionPayload    `json:"agentSession"`
    AgentActivity    *agentActivityPayload  `json:"agentActivity"`
    PreviousComments []any                  `json:"previousComments"`
}

type agentSessionPayload struct {
    ID     string       `json:"id"`
    Status string       `json:"status"`
    Issue  issuePayload `json:"issue"`
}

type issuePayload struct {
    ID          string `json:"id"`
    Identifier  string `json:"identifier"`
    Title       string `json:"title"`
    Description string `json:"description"`
    URL         string `json:"url"`
}

type agentActivityPayload struct {
    Type string `json:"type"`
    Body string `json:"body"`
}
```

#### 1.6 Linear Event Mapper

**File**: `relay/internal/mapper/linear_mapper.go`
**Changes**: Replace stub with actual implementation

```go
package mapper

import (
    "context"
    "fmt"
)

// LinearEventMapper maps Linear webhook events to canonical event types
type LinearEventMapper struct{}

// NewLinearEventMapper creates a new Linear event mapper
func NewLinearEventMapper() *LinearEventMapper {
    return &LinearEventMapper{}
}

// Map converts Linear webhook payload to canonical event type
func (m *LinearEventMapper) Map(ctx context.Context, body map[string]any, headers map[string]string) (CanonicalEventType, error) {
    eventType := headers["Linear-Event"]
    action, _ := body["action"].(string)
    payloadType, _ := body["type"].(string)

    // Handle AgentSession events (primary use case)
    if payloadType == "AgentSession" || eventType == "AgentSession" {
        switch action {
        case "created":
            // New agent session - user @mentioned or delegated
            return EventAgentSessionCreated, nil
        case "prompted":
            // User sent follow-up message
            return EventReply, nil
        default:
            return "", fmt.Errorf("unknown agent session action: %s", action)
        }
    }

    // Handle standard data-change webhooks (future support)
    switch payloadType {
    case "Issue":
        switch action {
        case "create":
            return EventIssueCreated, nil
        case "update":
            return EventIssueUpdated, nil
        }
    case "Comment":
        if action == "create" {
            return EventReply, nil
        }
    }

    return "", fmt.Errorf("unsupported Linear event: type=%s action=%s", payloadType, action)
}
```

#### 1.7 Add Agent Session Event Type

**File**: `relay/internal/mapper/mapper.go`
**Changes**: Add new canonical event type

```go
// Add after line 14

// EventAgentSessionCreated is emitted when a Linear agent session is created
EventAgentSessionCreated CanonicalEventType = "agent_session_created"

// EventIssueUpdated is emitted when an issue is updated
EventIssueUpdated CanonicalEventType = "issue_updated"
```

#### 1.8 Register Linear Mapper

**File**: `relay/internal/mapper/registry.go`
**Changes**: Register Linear mapper in init

```go
// Add after line 18 (after GitLab registration)
registry.Register("linear", NewLinearEventMapper())
```

#### 1.9 Linear Webhook Router

**File**: `relay/internal/http/router/linear.go` (new file)
**Changes**: Define Linear webhook routes

```go
package router

import (
    "github.com/basegraph/relay/internal/http/handler/webhook"
    "github.com/go-chi/chi/v5"
)

// LinearWebhookRoutes returns routes for Linear webhooks
func LinearWebhookRoutes(handler *webhook.LinearWebhookHandler) func(chi.Router) {
    return func(r chi.Router) {
        r.Post("/{integration_id}", handler.Handle)
    }
}
```

#### 1.10 Mount Linear Routes

**File**: `relay/internal/http/router/router.go`
**Changes**: Add Linear webhook routes

```go
// Add import for linear mapper
// In NewRouter function, after GitLab webhook handler setup (around line 46):

// Linear webhook handler
linearMapper := mapper.MustGet("linear")
linearWebhookHandler := webhook.NewLinearWebhookHandler(
    services.IntegrationCredential(),
    services.EventIngest(),
    linearMapper,
)
r.Route("/webhooks/linear", LinearWebhookRoutes(linearWebhookHandler))
```

### Success Criteria

#### Automated Verification:
- [x] Build succeeds: `make build`
- [x] All tests pass: `make test`
- [x] Lint passes: `make lint`
- [x] New Linear client compiles with GraphQL queries

#### Manual Verification:
- [ ] Webhook endpoint responds at `/webhooks/linear/{integration_id}`
- [ ] Invalid signatures return 401
- [ ] Valid signatures with test payload return 200
- [ ] Events are logged with correct structure

**Implementation Note**: After completing this phase and all automated verification passes, pause here for manual confirmation that webhook handling works correctly before proceeding to Phase 2.

---

## Phase 2: Session Management

### Overview
Add session tracking to persist Linear agent sessions and emit basic activities. After this phase, Basegraph will acknowledge agent sessions and show basic progress in Linear.

### Changes Required

#### 2.1 Agent Session Model

**File**: `relay/internal/model/agent_session.go` (new file)
**Changes**: Define agent session domain model

```go
package model

import "time"

// AgentSessionStatus represents the lifecycle state of an agent session
type AgentSessionStatus string

const (
    AgentSessionStatusPending      AgentSessionStatus = "pending"
    AgentSessionStatusActive       AgentSessionStatus = "active"
    AgentSessionStatusAwaitingInput AgentSessionStatus = "awaiting_input"
    AgentSessionStatusComplete     AgentSessionStatus = "complete"
    AgentSessionStatusError        AgentSessionStatus = "error"
)

// AgentSession tracks a Linear agent session
type AgentSession struct {
    ID                int64              // Internal Snowflake ID
    IntegrationID     int64
    IssueID           int64              // FK to issues table
    ExternalSessionID string             // Linear's session ID
    Status            AgentSessionStatus
    PromptContext     string             // Linear's pre-formatted context
    Plan              string             // JSON plan with status items
    CreatedAt         time.Time
    UpdatedAt         time.Time
    CompletedAt       *time.Time
}

// AgentActivityType represents the semantic type of an activity
type AgentActivityType string

const (
    AgentActivityTypeThought     AgentActivityType = "thought"
    AgentActivityTypeAction      AgentActivityType = "action"
    AgentActivityTypeResponse    AgentActivityType = "response"
    AgentActivityTypeError       AgentActivityType = "error"
    AgentActivityTypeElicitation AgentActivityType = "elicitation"
)

// AgentActivity records an activity emitted to Linear
type AgentActivity struct {
    ID               int64
    AgentSessionID   int64
    ExternalID       string            // Linear's activity ID (after sync)
    ActivityType     AgentActivityType
    Content          string            // JSON content
    EmittedAt        time.Time
    SyncedAt         *time.Time        // When confirmed by Linear
}
```

#### 2.2 Database Migration

**File**: `relay/migrations/YYYYMMDDHHMMSS_add_agent_sessions.sql` (new file)
**Changes**: Add agent session tables

```sql
-- +goose Up

-- Agent sessions track Linear agent session lifecycle
CREATE TABLE agent_sessions (
    id BIGINT PRIMARY KEY,
    integration_id BIGINT NOT NULL REFERENCES integrations(id),
    issue_id BIGINT NOT NULL REFERENCES issues(id),
    external_session_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    prompt_context TEXT,
    plan JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,

    CONSTRAINT unique_external_session UNIQUE (integration_id, external_session_id)
);

CREATE INDEX idx_agent_sessions_issue ON agent_sessions(issue_id);
CREATE INDEX idx_agent_sessions_status ON agent_sessions(status) WHERE status != 'complete';

-- Agent activities record emitted activities for audit/retry
CREATE TABLE agent_activities (
    id BIGINT PRIMARY KEY,
    agent_session_id BIGINT NOT NULL REFERENCES agent_sessions(id),
    external_id TEXT,
    activity_type TEXT NOT NULL,
    content JSONB NOT NULL,
    emitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    synced_at TIMESTAMPTZ
);

CREATE INDEX idx_agent_activities_session ON agent_activities(agent_session_id);
CREATE INDEX idx_agent_activities_unsynced ON agent_activities(agent_session_id)
    WHERE synced_at IS NULL;

-- +goose Down
DROP TABLE agent_activities;
DROP TABLE agent_sessions;
```

#### 2.3 Agent Session Store

**File**: `relay/internal/store/agent_session.go` (new file)
**Changes**: Implement store layer for agent sessions

```go
package store

import (
    "context"
    "fmt"

    "github.com/basegraph/relay/core/db"
    "github.com/basegraph/relay/internal/model"
)

type AgentSessionStore interface {
    Create(ctx context.Context, session model.AgentSession) error
    GetByExternalID(ctx context.Context, integrationID int64, externalID string) (model.AgentSession, error)
    GetByIssueID(ctx context.Context, issueID int64) (model.AgentSession, error)
    UpdateStatus(ctx context.Context, id int64, status model.AgentSessionStatus) error
    UpdatePlan(ctx context.Context, id int64, plan string) error
    MarkComplete(ctx context.Context, id int64) error
}

type agentSessionStore struct {
    q *db.Queries
}

func NewAgentSessionStore(q *db.Queries) AgentSessionStore {
    return &agentSessionStore{q: q}
}

func (s *agentSessionStore) Create(ctx context.Context, session model.AgentSession) error {
    return s.q.CreateAgentSession(ctx, db.CreateAgentSessionParams{
        ID:                session.ID,
        IntegrationID:     session.IntegrationID,
        IssueID:           session.IssueID,
        ExternalSessionID: session.ExternalSessionID,
        Status:            string(session.Status),
        PromptContext:     toNullString(session.PromptContext),
    })
}

func (s *agentSessionStore) GetByExternalID(ctx context.Context, integrationID int64, externalID string) (model.AgentSession, error) {
    row, err := s.q.GetAgentSessionByExternalID(ctx, db.GetAgentSessionByExternalIDParams{
        IntegrationID:     integrationID,
        ExternalSessionID: externalID,
    })
    if err != nil {
        return model.AgentSession{}, fmt.Errorf("querying agent session: %w", err)
    }
    return mapAgentSession(row), nil
}

// ... additional methods
```

#### 2.4 Activity Emitter Interface

**File**: `relay/internal/brain/activity_emitter.go` (new file)
**Changes**: Define activity emission interface

```go
package brain

import (
    "context"

    "github.com/basegraph/relay/internal/model"
)

// ActivityEmitter emits activities to external systems (e.g., Linear)
type ActivityEmitter interface {
    // EmitThought emits a reasoning/thinking activity
    EmitThought(ctx context.Context, body string) error

    // EmitAction emits a tool/action invocation activity
    EmitAction(ctx context.Context, action string, params any, result any) error

    // EmitResponse emits a final response activity
    EmitResponse(ctx context.Context, body string) error

    // EmitError emits an error activity
    EmitError(ctx context.Context, body string) error

    // UpdatePlan updates the session's plan checklist
    UpdatePlan(ctx context.Context, items []PlanItem) error

    // SessionID returns the external session ID (for logging)
    SessionID() string
}

// PlanItem represents a checklist item in the agent's plan
type PlanItem struct {
    Content string `json:"content"`
    Status  string `json:"status"` // "pending", "inProgress", "completed", "canceled"
}

// NoOpEmitter is used when no external session tracking is needed
type NoOpEmitter struct{}

func (n *NoOpEmitter) EmitThought(ctx context.Context, body string) error { return nil }
func (n *NoOpEmitter) EmitAction(ctx context.Context, action string, params, result any) error { return nil }
func (n *NoOpEmitter) EmitResponse(ctx context.Context, body string) error { return nil }
func (n *NoOpEmitter) EmitError(ctx context.Context, body string) error { return nil }
func (n *NoOpEmitter) UpdatePlan(ctx context.Context, items []PlanItem) error { return nil }
func (n *NoOpEmitter) SessionID() string { return "" }
```

#### 2.5 Linear Activity Emitter

**File**: `relay/internal/brain/linear_emitter.go` (new file)
**Changes**: Implement activity emission for Linear

```go
package brain

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"

    "github.com/basegraph/relay/internal/linear"
    "github.com/basegraph/relay/internal/logger"
)

// LinearEmitter emits activities to Linear's Agent Session API
type LinearEmitter struct {
    client    *linear.Client
    sessionID string
}

// NewLinearEmitter creates an emitter for a specific session
func NewLinearEmitter(client *linear.Client, sessionID string) *LinearEmitter {
    return &LinearEmitter{
        client:    client,
        sessionID: sessionID,
    }
}

func (e *LinearEmitter) SessionID() string {
    return e.sessionID
}

func (e *LinearEmitter) EmitThought(ctx context.Context, body string) error {
    ctx = logger.WithLogFields(ctx, "activity_type", "thought")

    content := linear.ThoughtContent{
        Type: "thought",
        Body: body,
    }

    return e.emit(ctx, content)
}

func (e *LinearEmitter) EmitAction(ctx context.Context, action string, params, result any) error {
    ctx = logger.WithLogFields(ctx, "activity_type", "action", "action_name", action)

    content := linear.ActionContent{
        Type:       "action",
        Action:     action,
        Parameters: params,
        Result:     result,
    }

    return e.emit(ctx, content)
}

func (e *LinearEmitter) EmitResponse(ctx context.Context, body string) error {
    ctx = logger.WithLogFields(ctx, "activity_type", "response")

    content := linear.ResponseContent{
        Type: "response",
        Body: body,
    }

    return e.emit(ctx, content)
}

func (e *LinearEmitter) EmitError(ctx context.Context, body string) error {
    ctx = logger.WithLogFields(ctx, "activity_type", "error")

    content := linear.ErrorContent{
        Type: "error",
        Body: body,
    }

    return e.emit(ctx, content)
}

func (e *LinearEmitter) UpdatePlan(ctx context.Context, items []PlanItem) error {
    planJSON, err := json.Marshal(items)
    if err != nil {
        return fmt.Errorf("marshaling plan: %w", err)
    }

    plan := string(planJSON)
    _, err = e.client.UpdateAgentSession(ctx, e.sessionID, linear.AgentSessionUpdateInput{
        Plan: &plan,
    })
    if err != nil {
        slog.WarnContext(ctx, "failed to update session plan",
            "session_id", e.sessionID,
            "error", err)
        return err
    }

    slog.DebugContext(ctx, "updated session plan",
        "session_id", e.sessionID,
        "item_count", len(items))

    return nil
}

func (e *LinearEmitter) emit(ctx context.Context, content any) error {
    input := linear.AgentActivityCreateInput{
        AgentSessionID: e.sessionID,
        Content:        content,
    }

    activity, err := e.client.CreateAgentActivity(ctx, input)
    if err != nil {
        slog.WarnContext(ctx, "failed to emit activity",
            "session_id", e.sessionID,
            "error", err)
        // Don't fail the main workflow for activity emission failures
        return nil
    }

    slog.DebugContext(ctx, "emitted activity",
        "session_id", e.sessionID,
        "activity_id", activity.ID)

    return nil
}
```

#### 2.6 Modify Orchestrator for Activity Emission

**File**: `relay/internal/brain/orchestrator.go`
**Changes**: Add emitter field and emit activities at key points

```go
// In Orchestrator struct (around line 109), add:
    emitter ActivityEmitter

// In NewOrchestrator constructor, add parameter and initialization

// In HandleEngagement method, add emission points:

// After line 190 (start of engagement):
if o.emitter != nil {
    _ = o.emitter.EmitThought(ctx, "Starting to analyze this issue...")
}

// After line 274 (before planning loop):
if o.emitter != nil {
    _ = o.emitter.EmitThought(ctx, fmt.Sprintf("Beginning analysis cycle %d", cycle))
}

// After successful planning (around line 290):
if o.emitter != nil && len(output.Actions) > 0 {
    _ = o.emitter.EmitThought(ctx, fmt.Sprintf("Planning complete, executing %d actions", len(output.Actions)))
}

// At engagement completion (around line 330):
if o.emitter != nil {
    _ = o.emitter.EmitResponse(ctx, "Finished processing this engagement")
}
```

### Success Criteria

#### Automated Verification:
- [x] Migration applies cleanly: `make migrate-up` (migration created)
- [x] Build succeeds: `make build`
- [x] All tests pass: `make test`
- [x] Lint passes: `make lint`

#### Manual Verification:
- [ ] Agent session created when Linear webhook received
- [ ] Basic thoughts appear in Linear UI ("Starting to analyze...")
- [ ] Session status updates visible in Linear
- [ ] Errors logged but don't break main workflow

**Implementation Note**: Phase 2 complete - database schema, stores, emitter interface, Linear emitter, and full orchestrator integration with emission calls at strategic points (engagement start/end, planning, action execution, spec generation). Context-based emitter pattern allows flexible per-engagement emitter creation. NoOpEmitter used when no Linear session is active.

---

## Phase 3: Full Transparency

### Overview
Add granular activity emission throughout the brain - planner iterations, explore tool calls, action execution. After this phase, users see complete visibility into Basegraph's reasoning.

### Changes Required

#### 3.1 Planner Activity Emission

**File**: `relay/internal/brain/planner.go`
**Changes**: Add emitter and emit activities during planning

```go
// Add emitter field to Planner struct
// Add to NewPlanner constructor
// Emit at key points:

// Before LLM call (around line 134):
if p.emitter != nil {
    _ = p.emitter.EmitThought(ctx, "Analyzing context and determining next steps...")
}

// After explore tool call (around line 380):
if p.emitter != nil {
    _ = p.emitter.EmitAction(ctx, "explore_codebase",
        map[string]any{"query": exploreCall.Query},
        map[string]any{"files_found": len(exploreCall.Results)})
}

// When submitting actions (around line 200):
if p.emitter != nil {
    actionSummary := summarizeActions(actions)
    _ = p.emitter.EmitThought(ctx, fmt.Sprintf("Decided to: %s", actionSummary))
}
```

#### 3.2 Explore Agent Activity Emission

**File**: `relay/internal/brain/explore_agent.go`
**Changes**: Emit activities for code exploration

```go
// Before starting exploration:
if e.emitter != nil {
    _ = e.emitter.EmitAction(ctx, "search_code",
        map[string]any{"query": query, "scope": scope},
        nil) // Result added after completion
}

// After finding files:
if e.emitter != nil {
    _ = e.emitter.EmitThought(ctx,
        fmt.Sprintf("Found %d relevant files in %s", len(files), scope))
}
```

#### 3.3 Action Executor Activity Emission

**File**: `relay/internal/brain/action_executor.go`
**Changes**: Emit activities for each action type

```go
// Add emitter field and constructor param

// In executePostComment:
if a.emitter != nil {
    _ = a.emitter.EmitAction(ctx, "post_comment",
        map[string]any{"length": len(content)},
        map[string]any{"success": true})
}

// In executeReadyForSpecGeneration:
if a.emitter != nil {
    _ = a.emitter.EmitThought(ctx, "Generating implementation specification...")
}
// After spec generated:
if a.emitter != nil {
    _ = a.emitter.EmitAction(ctx, "generate_spec",
        map[string]any{"gaps_closed": len(closedGaps)},
        map[string]any{"spec_length": len(spec)})
}
```

#### 3.4 Plan Progress Updates

**File**: `relay/internal/brain/orchestrator.go`
**Changes**: Update plan checklist as work progresses

```go
// At start of engagement, set initial plan:
if o.emitter != nil {
    _ = o.emitter.UpdatePlan(ctx, []PlanItem{
        {Content: "Analyze issue context", Status: "inProgress"},
        {Content: "Search relevant code", Status: "pending"},
        {Content: "Determine response", Status: "pending"},
    })
}

// After context built:
if o.emitter != nil {
    _ = o.emitter.UpdatePlan(ctx, []PlanItem{
        {Content: "Analyze issue context", Status: "completed"},
        {Content: "Search relevant code", Status: "inProgress"},
        {Content: "Determine response", Status: "pending"},
    })
}

// After exploration:
if o.emitter != nil {
    _ = o.emitter.UpdatePlan(ctx, []PlanItem{
        {Content: "Analyze issue context", Status: "completed"},
        {Content: "Search relevant code", Status: "completed"},
        {Content: "Determine response", Status: "inProgress"},
    })
}
```

### Success Criteria

#### Automated Verification:
- [ ] Build succeeds: `make build`
- [ ] All tests pass: `make test`
- [ ] Lint passes: `make lint`

#### Manual Verification:
- [ ] Thoughts appear as planner iterates
- [ ] Code exploration shows as actions with file counts
- [ ] Plan checklist updates in real-time
- [ ] Action execution visible (comment posting, spec generation)
- [ ] Activity emission doesn't noticeably slow down processing

**Implementation Note**: After completing this phase, pause for UX review. Verify that activity granularity feels right - not too verbose, not too sparse.

---

## Phase 4: Polish

### Overview
Add OAuth setup flow, error recovery, monitoring, and final polish. After this phase, Linear integration is production-ready.

### Changes Required

#### 4.1 Linear OAuth Handler

**File**: `relay/internal/http/handler/linear.go` (new file)
**Changes**: OAuth callback handling

```go
package handler

// LinearHandler handles Linear OAuth and setup
type LinearHandler struct {
    linearService service.LinearService
}

func (h *LinearHandler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
    // Exchange code for tokens
    // Store in integration_credentials
    // Configure webhook with signing secret
}

func (h *LinearHandler) SetupIntegration(w http.ResponseWriter, r *http.Request) {
    // Validate OAuth tokens
    // Register as agent in workspace
    // Return success/error
}
```

#### 4.2 Linear Integration Service

**File**: `relay/internal/service/integration/linear.go` (new file)
**Changes**: Integration lifecycle management

```go
package integration

type linearService struct {
    txRunner  TxRunner
    stores    *store.Stores
}

func (s *linearService) SetupIntegration(ctx context.Context, params SetupParams) (*SetupResult, error) {
    // Validate OAuth tokens
    // Create integration record
    // Store credentials (access token, refresh token)
    // Generate and store webhook secret
    // Fetch and store service account identity
    // Return result with webhook URL
}

func (s *linearService) RefreshToken(ctx context.Context, integrationID int64) error {
    // Check token expiration
    // Call Linear OAuth refresh endpoint
    // Update stored credentials
}
```

#### 4.3 Session Recovery

**File**: `relay/internal/brain/session_recovery.go` (new file)
**Changes**: Handle session interruptions

```go
package brain

// RecoverSession handles worker crashes or timeouts
func RecoverSession(ctx context.Context, sessionID int64) error {
    // Load session from database
    // Check last activity timestamp
    // If stale, emit error activity to Linear
    // Mark session as error state
}

// ResumeSession continues a paused session
func ResumeSession(ctx context.Context, sessionID int64, newPrompt string) error {
    // Load existing session context
    // Append new prompt to context
    // Continue processing
}
```

#### 4.4 Monitoring & Metrics

**File**: `relay/internal/brain/metrics.go`
**Changes**: Add Linear-specific metrics

```go
// Track activity emission latency
// Track session lifecycle duration
// Track failure rates by activity type
// Export to observability system
```

#### 4.5 Linear Issue Tracker Service

**File**: `relay/internal/service/issue_tracker/linear.go` (new file)
**Changes**: Implement IssueTrackerService for Linear

```go
package issue_tracker

type linearIssueTrackerService struct {
    integrations store.IntegrationStore
    credentials  store.IntegrationCredentialStore
}

func (s *linearIssueTrackerService) FetchIssue(ctx context.Context, params FetchIssueParams) (*model.Issue, error) {
    client := s.getClient(ctx, params.IntegrationID)
    issue, err := client.Issue(ctx, params.ExternalIssueID)
    // Map to model.Issue
}

func (s *linearIssueTrackerService) CreateDiscussion(ctx context.Context, params CreateDiscussionParams) (*CreateDiscussionResult, error) {
    // Use commentCreate mutation
}

func (s *linearIssueTrackerService) ReplyToThread(ctx context.Context, params ReplyToThreadParams) (*ReplyToThreadResult, error) {
    // Linear doesn't have threaded comments like GitLab
    // Create new comment referencing the previous
}

func (s *linearIssueTrackerService) AddReaction(ctx context.Context, params AddReactionParams) error {
    // Use reactionCreate mutation with emoji
}
```

#### 4.6 Register Linear in Service Factory

**File**: `relay/internal/service/factory.go`
**Changes**: Uncomment and implement Linear service

```go
// In IssueTrackers() method:
map[model.Provider]tracker.IssueTrackerService{
    model.ProviderGitLab: s.GitlabIssueTracker(),
    model.ProviderLinear: s.LinearIssueTracker(), // Uncomment and implement
}

// Add factory method:
func (s *Services) LinearIssueTracker() tracker.IssueTrackerService {
    return issue_tracker.NewLinearIssueTrackerService(
        s.stores.Integrations(),
        s.stores.IntegrationCredentials(),
    )
}
```

### Success Criteria

#### Automated Verification:
- [ ] Build succeeds: `make build`
- [ ] All tests pass: `make test`
- [ ] Lint passes: `make lint`
- [ ] Integration tests pass with Linear sandbox

#### Manual Verification:
- [ ] OAuth flow completes successfully
- [ ] Basegraph appears as assignable/mentionable in Linear
- [ ] Full engagement flow works end-to-end
- [ ] Activities appear with <2 second latency
- [ ] Session recovery works after simulated crash
- [ ] Token refresh happens automatically before expiration
- [ ] Existing GitLab integration unaffected

---

## Testing Strategy

### Unit Tests
- Linear client methods (mock GraphQL responses)
- Event mapper (various payload types)
- Activity emitter (verify content structure)
- Session store operations

### Integration Tests
- Webhook signature validation
- OAuth token exchange
- Session lifecycle (create → active → complete)
- Activity emission to Linear sandbox

### Manual Testing Steps
1. Install Basegraph OAuth app in Linear workspace
2. Create test issue, @mention Basegraph
3. Verify immediate acknowledgment (<10 seconds)
4. Verify thoughts appear during processing
5. Verify final response posted
6. Verify session marked complete in Linear
7. Test follow-up prompt flow
8. Test error scenarios (invalid code, timeout)

---

## Performance Considerations

- **Activity emission is non-blocking**: Failures logged but don't stop main workflow
- **Batch activities when possible**: Group rapid thoughts into single emission
- **Rate limiting**: Linear allows 500 requests/hour/user for OAuth apps
- **Token refresh**: Proactively refresh before expiration (5 min buffer)

---

## Migration Notes

- No migration needed for existing GitLab integrations
- Linear integration is additive, no breaking changes
- Database migration is backward-compatible (new tables only)

---

## References

- Linear Developers Documentation: https://linear.app/developers
- Linear Agent Interaction: https://linear.app/developers/agent-interaction
- Linear Webhooks: https://linear.app/developers/webhooks
- Linear AIG (Agent Interaction Guidelines): https://linear.app/developers/aig
- Existing GitLab integration: `relay/internal/service/integration/gitlab.go`
- Existing issue tracker interface: `relay/internal/service/issue_tracker/issue_tracker.go`
