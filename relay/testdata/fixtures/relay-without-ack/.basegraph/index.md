# Codebase Index

## Directory Structure

```
/brain       - AI planning agents and orchestration
/domain      - Core domain models (Issue, Gap, Finding, Event)
/http        - HTTP layer (handlers, middleware, routing, DTOs)
/mapper      - Event mappers for different providers (GitLab, GitHub, Linear)
/model       - Database models and entities
/queue       - Redis queue producer/consumer
/service     - Business logic services
/store       - Database access layer (PostgreSQL)
/worker      - Background job processors
```

## Files by Directory

### /brain
- action.go - Defines ActionType enum and Action struct for planner outputs (post_comment, update_gaps, etc.)
- action_executor.go - Executes planner actions: posts comments, updates gaps/findings via store
- action_validator.go - Validates action parameters before execution
- context_builder.go - Builds LLM context from issue state (discussions, gaps, learnings, findings)
- explore_agent.go - Sub-agent that explores codebase using glob/grep/read tools, returns prose reports
- explore_tools.go - Implements glob, grep, read, bash tools for explore agent
- orchestrator.go - Coordinates planner workflow: loads issue, runs planner, executes actions
- planner.go - Main planning agent: spawns explore agents, decides next actions, logs token usage
- spec_generator.go - Generates implementation specs from gathered context

### /domain
- discussion.go - Discussion/comment thread model for issue conversations
- event.go - Webhook event representation (provider-agnostic)
- finding.go - Code finding from exploration with synthesis and sources
- gap.go - Knowledge gap: unanswered question with severity and target
- issue.go - Issue domain model with processing state
- job.go - Background job representation

### /http/dto
- gitlab.go - GitLab webhook payload DTOs (issue events, note events)
- organization.go - Organization request/response DTOs
- user.go - User profile DTOs

### /http/handler
- auth.go - Handles login/logout, session management, OAuth callbacks
- gitlab.go - GitLab OAuth flow and project listing
- organization.go - CRUD handlers for organizations
- user.go - User profile handlers (get/update current user)
- webhook/gitlab.go - Receives GitLab webhooks, validates token, calls EventIngestService

### /http/middleware
- auth.go - JWT validation, extracts user from token, protects routes
- logger.go - Request logging with trace ID propagation
- recovery.go - Panic recovery, returns 500 on unhandled errors

### /http/router
- auth.go - Auth routes: /auth/login, /auth/callback
- gitlab.go - GitLab routes: /integrations/gitlab/*, /webhooks/gitlab/:id
- organization.go - Organization routes: /organizations/*
- router.go - Main router setup, mounts all route groups
- user.go - User routes: /users/me

### /mapper
- github_mapper.go - Maps GitHub webhook payloads to canonical events
- gitlab_mapper.go - Maps GitLab webhook payloads to canonical events
- linear_mapper.go - Maps Linear webhook payloads to canonical events
- mapper.go - EventMapper interface: MapEvent(payload) -> CanonicalEvent
- registry.go - Registry of mappers by provider, GetMapper(provider) -> EventMapper

### /model
- event_log.go - Audit log entry: tracks every webhook event received
- gap.go - Gap database model: question, severity, target, resolution
- integration_config.go - Per-project integration settings (webhook ID, enabled repos)
- integration_credential.go - OAuth tokens: access_token, refresh_token, expiry
- integration.go - Integration: links workspace to provider (GitLab/GitHub/Linear)
- issue.go - Issue with processing state (idle/queued/processing), discussions, findings, gaps
- learning.go - Captured tribal knowledge: domain rules, codebase patterns
- llm_eval.go - Tracks LLM call metrics: prompt_tokens, completion_tokens, latency, rating
- organization.go - Organization: billing entity, owns workspaces
- repository.go - Repository linked to integration
- session.go - User session for auth
- user.go - User profile: email, name, avatar
- workspace.go - Workspace: contains integrations, issues, learnings

### /queue
- consumer.go - Redis stream consumer: XReadGroup, Ack, Requeue, SendDLQ
- producer.go - Redis stream producer: Enqueue events for worker processing

### /service
- auth.go - Auth service: login, logout, refresh tokens, validate sessions
- engagement_detector.go - Detects @mentions, decides if Relay should engage with issue
- event_ingest.go - Webhook ingestion: validates, dedupes, upserts issue, queues for processing
- factory.go - Creates all services with dependencies
- integration_credential.go - Manages OAuth credentials: store, refresh, revoke
- organization.go - Organization CRUD with membership checks
- txrunner.go - Wraps database transactions, provides store access within tx
- user.go - User profile service
- integration/gitlab.go - GitLab API client: list projects, setup webhooks, fetch user
- issue_tracker/gitlab.go - Fetches issues/discussions from GitLab API
- issue_tracker/issue_tracker.go - IssueTrackerService interface: FetchIssue, FetchDiscussions, PostComment

### /store
- event_log.go - EventLogStore: CreateOrGet (with dedupe), ListByIssue
- factory.go - Creates all stores with database connection
- gap.go - GapStore: CRUD for gaps, ListByIssue
- integration_config.go - IntegrationConfigStore: per-project settings
- integration_credential.go - IntegrationCredentialStore: OAuth token storage
- integration.go - IntegrationStore: GetByID, ListByWorkspace
- interfaces.go - All store interfaces defined here
- issue.go - IssueStore: Upsert, GetByID, QueueIfIdle, MarkProcessing
- learning.go - LearningStore: CRUD for learnings, ListByWorkspace
- llm_eval.go - LLMEvalStore: stores LLM call metrics for analysis
- organization.go - OrganizationStore: CRUD with membership
- repo.go - RepositoryStore: linked repos for integration
- session.go - SessionStore: user sessions
- user.go - UserStore: user profiles
- workspace.go - WorkspaceStore: workspace CRUD

### /worker
- reclaimer.go - Reclaims stuck messages from Redis stream after timeout (XAutoClaim)
