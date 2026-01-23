# Workspace Setup + Sync (Backend Plan)

## Goal
Enable per-workspace workers to clone and sync enabled repositories, stream setup and sync progress, and gate issue processing until repositories are ready.

## Current State (Already Done)
- Per-workspace Redis task stream routing for issue events.
- Task envelope supports `issue_event`, `workspace_setup`, `repo_sync`.
- `workspace_event_logs` table plus repo selection fields (`repositories.is_enabled`, `repositories.default_branch`, `workspaces.repo_ready_at`).
- GitLab onboarding split: integration setup vs repo enablement.
- GitLab push hook enqueues `repo_sync`.
- Worker task runner for `workspace_setup` and `repo_sync`, issue gating, deploy key setup, SSH clone.
- Redis SSE endpoint streams status events; worker emits progress to `agent-status:org-...:workspace-...`.

## Remaining Backend Work
1) Workspace setup enqueue flow
   - When repo enablement is saved:
     - Create stream and consumer group for the workspace (`XGROUP CREATE ... MKSTREAM`).
     - Create `workspace_event_logs` run row (`workspace_setup`, status `queued`).
     - Enqueue `workspace_setup` task into `agent-stream:org-{org_id}:workspace-{ws_id}`.
   - Ensure failure paths update run status.

2) Run history endpoint (optional for beta)
   - `GET /api/v1/workspaces/:id/workspace-event-logs?limit=N`
   - Returns recent setup/sync runs for dashboard history.

3) Worker env contract documentation
   - Update example env: `WORKSPACE_ID`, `DATA_DIR`, `REDIS_STREAM`, `REDIS_CONSUMER_GROUP`, `REDIS_CONSUMER_NAME`.

## Data Model
- `workspaces.repo_ready_at`: set on first successful repo clone.
- `repositories.is_enabled`: source of truth for selection.
- `repositories.default_branch`: used for push gating and sync.
- `workspace_event_logs`: one row per setup/sync run (durable audit).

## Redis Streams
- Task stream: `agent-stream:org-{org_id}:workspace-{ws_id}`.
- Status stream: `agent-status:org-{org_id}:workspace-{ws_id}` (trim ~2000).
- Consumer group: `agent-group`.

## Worker Behavior
- `workspace_setup`:
  - Ensure SSH keypair and known_hosts.
  - Attach deploy keys (workspace-level) to enabled repos.
  - Clone enabled repos, delete disabled repos.
  - Update `workspace_event_logs` status and `workspaces.repo_ready_at`.
- `repo_sync`:
  - `git fetch origin {branch}` + `git reset --hard origin/{branch}`.
  - Skip or requeue if repo not cloned or default branch missing.

## Gating Issue Events
- Issue processing requeues without attempt bump until:
  - `workspaces.repo_ready_at` is set, and
  - repo directory exists for issue `external_project_id`.

## Open Decisions
- None for backend; required choices already made.
