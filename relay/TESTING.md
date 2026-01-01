# Testing Action Executor

## Prerequisites

1. **Services running:**
   ```bash
   make dev-db  # Starts PostgreSQL, Redis, ArangoDB
   ```

2. **Database migrated:**
   ```bash
   DATABASE_URL="postgresql://postgres:postgres@localhost:5432/relay?sslmode=disable" make migrate-up
   ```

3. **Environment variables** (create `.env.local`):
   ```bash
   DATABASE_URL=postgresql://postgres:postgres@localhost:5432/relay?sslmode=disable
   REDIS_URL=redis://localhost:6379
   ARANGO_URL=http://localhost:8529
   ARANGO_PASSWORD=your-secret-password
   ANTHROPIC_API_KEY=<your-key>
   ```

## Test Approach

### Option 1: Unit Tests (Recommended for isolated testing)

Test individual components with mocks:

```bash
cd /Users/nithin/basegraph/relay
make test-unit
```

### Option 2: Integration Test (Full pipeline)

This requires:
1. A GitLab instance (or use gitlab.com)
2. GitLab integration setup in database
3. Webhook configured to point to your server

**Steps:**

1. **Start the server:**
   ```bash
   make run-server
   ```

2. **Start the worker:**
   ```bash
   make run-worker
   ```

3. **Setup GitLab Integration** (via SQL or API):
   ```sql
   -- Insert test data (adjust IDs/credentials as needed)
   INSERT INTO users (id, name, email, workos_id, created_at, updated_at)
   VALUES (1, 'Test User', 'test@example.com', NULL, now(), now());

   INSERT INTO organizations (id, admin_user_id, name, slug, created_at, updated_at)
   VALUES (1, 1, 'Test Org', 'test-org', now(), now());

   INSERT INTO workspaces (id, admin_user_id, organization_id, user_id, name, slug, created_at, updated_at)
   VALUES (1, 1, 1, 1, 'Test Workspace', 'test-workspace', now(), now());

   INSERT INTO integrations (id, workspace_id, organization_id, setup_by_user_id, provider, capabilities, provider_base_url, is_enabled, created_at, updated_at)
   VALUES (1, 1, 1, 1, 'gitlab', '{"issue_tracking"}', 'https://gitlab.com', true, now(), now());

   INSERT INTO integration_credentials (id, integration_id, user_id, credential_type, access_token, is_primary, created_at, updated_at)
   VALUES (1, 1, 1, 'oauth', '<GITLAB_TOKEN>', true, now(), now());
   
   INSERT INTO integration_configs (id, integration_id, key, value, config_type, created_at, updated_at)
   VALUES (1, 1, 'relay_username', '"relay-bot"', 'identity', now(), now());
   ```

4. **Trigger webhook** - @mention relay in a GitLab issue:
   ```
   @relay-bot please help with this issue
   ```

5. **Watch logs:**
   - Server logs: Event ingestion, engagement detection
   - Worker logs: Orchestrator → Planner → Executor → GitLab

### Option 3: Manual Testing (Direct DB + Queue)

Bypass webhooks and insert directly:

```sql
-- Create a test issue
INSERT INTO issues (id, integration_id, external_project_id, external_issue_id, provider, title, description, created_at, updated_at)
VALUES (1, 1, '12345', '1', 'gitlab', 'Test Issue', 'Test description', now(), now());

-- Queue it for processing
INSERT INTO event_logs (id, workspace_id, issue_id, triggered_by_username, source, event_type, dedupe_key, created_at)
VALUES (1, 1, 1, 'test-user', 'gitlab', 'comment', 'test-dedupe-1', now());

-- Manually publish to Redis (or let worker poll)
```

Then start worker and watch it process.

## What to Verify

1. **Planner runs** - Check logs for "planner completed"
2. **Actions returned** - Log shows action count > 0
3. **Executor runs** - Log shows "action failed" or success
4. **GitLab comment posted** - Check GitLab issue for Relay's comment
5. **Gaps created** - Check database `gaps` table
6. **Findings updated** - Check `issues.code_findings` JSON

## Common Issues

1. **"external project id is required"** - Issue missing `external_project_id`
   - Solution: Ensure webhook handler populates this field
   
2. **"no issue tracker for provider"** - Missing GitLab service in map
   - Solution: Check `cmd/worker/main.go` has GitLab in `issueTrackers` map

3. **GitLab 401** - Invalid credentials
   - Solution: Check `integration_credentials` has valid token

4. **Planner timeout** - LLM taking too long
   - Solution: Check Anthropic API key, network

## Quick Smoke Test

Test just the executor without full pipeline:

```go
// relay/internal/brain/action_executor_test.go
func TestExecutePostComment(t *testing.T) {
    // Mock issue tracker, issue, gaps stores
    // Create executor
    // Call Execute with PostCommentAction
    // Verify tracker.CreateDiscussion was called
}
```

Run: `go test ./internal/brain -v -run TestExecutePostComment`
