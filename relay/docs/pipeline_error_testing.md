# Testing Error Handling & Crashes

This guide shows how to test the worker's robustness using VS Code debugger.

## Setup

1. Ensure `.env` has:
```bash
OPENAI_API_KEY=sk-...
DATABASE_URL=postgres://...
REDIS_ADDR=localhost:6379
```

2. Start dependencies:
```bash
make dev-db
```

3. Apply migrations:
```bash
make migrate-up
make sqlc-generate
```

---

## Test Scenarios

### 1. Test Panic Recovery

**Goal**: Verify `processMessageSafe` catches panics and doesn't crash the worker.

**Steps**:
1. Open [relay/internal/pipeline/keywords.go](relay/internal/pipeline/keywords.go)
2. Set breakpoint at line 48 (inside the retry loop)
3. Add conditional breakpoint:
   - Right-click breakpoint → **Edit Breakpoint → Expression**
   - Enter: `attempt == 0`
   - **Action**: `panic("test panic")`
   - Check **"Log message and continue"** ✅ (off)

4. Launch **"Worker"** from VS Code Debug panel
5. Trigger processing (insert test data - see below)
6. **Expected**:
   - Panic is caught
   - Log: `"panic recovered in message processing"`
   - Message goes to retry/DLQ
   - Worker continues running (doesn't exit)

---

### 2. Test LLM Retry Logic

**Goal**: Verify exponential backoff works for retryable errors.

**Steps**:
1. Use **invalid API key** to trigger 401:
   ```bash
   # In .env
   OPENAI_API_KEY=sk-invalid
   ```

2. Set breakpoint at [keywords.go:59](relay/internal/pipeline/keywords.go#L59) (`if !llm.IsRetryable`)
3. Launch Worker debugger
4. Trigger processing
5. **Expected**:
   - First attempt fails
   - `IsRetryable` returns `false` (401 is non-retryable)
   - Error logged: `"llm client error, not retryable"`
   - No retries (exits immediately)

**Test 429 Rate Limit**:
1. Restore valid API key
2. Set breakpoint at [keywords.go:66](relay/internal/pipeline/keywords.go#L66) (`time.Sleep`)
3. Watch the `attempt` variable increment
4. **Expected**: Sleeps 1s, 2s, 4s before retries

---

### 3. Test Worker Death & Redis Reclaim

**Goal**: Verify Redis PEL reclaims messages when worker dies.

**Steps**:
1. Launch Worker in debugger
2. Trigger processing (insert test data)
3. Set breakpoint at [worker.go:143](relay/internal/worker/worker.go#L143) (inside `Process`)
4. When breakpoint hits, **stop the debugger** (simulates crash)
5. Check Redis:
   ```bash
   redis-cli
   > XPENDING relay:issue:events relay-workers - + 10
   # Should show 1 pending message
   ```
6. Restart worker
7. **Expected**:
   - Worker calls `consumer.ClaimStale()` (reclaimer)
   - Message is reprocessed
   - `ClaimQueued` prevents duplicate work

---

### 4. Test Transaction Rollback

**Goal**: Verify DB rollback works on error.

**Steps**:
1. Set breakpoint at [worker.go:159](relay/internal/worker/worker.go#L159) (after `updatedIssue`)
2. Use **Evaluate Expression** in VS Code:
   ```go
   return fmt.Errorf("simulated error")
   ```
3. **Expected**:
   - Transaction rolls back
   - Issue status returns to `queued`
   - Message is retried (or DLQ'd after max attempts)

---

### 5. Test Graceful Shutdown

**Goal**: Verify worker stops cleanly.

**Steps**:
1. Launch Worker
2. Trigger processing
3. Press **Ctrl+C** in terminal (or Stop in VS Code)
4. **Expected**:
   - Log: `"worker stopping"`
   - In-flight message completes (transaction commits or rolls back)
   - No data corruption

---

## Inserting Test Data

```sql
-- Insert test issue
INSERT INTO issues (
    id, project_id, external_id, provider,
    title, description, status, created_at, updated_at
) VALUES (
    1000000000001, 1, 'DEBUG-1', 'linear',
    'Fix authentication bug in TwilioProvider',
    'Users report intermittent auth failures when rate limit is hit.',
    'queued', NOW(), NOW()
);

-- Insert event
INSERT INTO event_logs (
    id, project_id, issue_id, event_type, source,
    payload, created_at
) VALUES (
    2000000000001, 1, 1000000000001, 'issue.created', 'linear',
    '{}', NOW()
);

-- Publish to Redis (worker auto-consumes from stream)
```

Or use the **server webhook** endpoint:
```bash
curl -X POST http://localhost:8080/webhooks/linear \
  -H "Content-Type: application/json" \
  -d '{"action":"create","data":{"id":"DEBUG-1","title":"Test issue"}}'
```

---

## VS Code Debug Tips

### Conditional Breakpoints
- Right-click line → **Add Conditional Breakpoint**
- **Expression**: `issue.ID == 1000000000001`
- **Hit Count**: Break on 3rd hit

### Logpoints
- Right-click line → **Add Logpoint**
- Message: `Keywords extracted: {len(keywords)}`
- Logs without stopping execution

### Watch Variables
Add to **Watch** panel:
- `msg.Attempt` - current retry count
- `issue.Status` - issue state
- `err` - last error

---

## Expected Logs

**Success**:
```
INFO  llm client initialized model=gpt-4o-mini
INFO  processing message issue_id=1000000000001 attempt=1
INFO  keywords extracted issue_id=1000000000001 keyword_count=5
INFO  successfully processed events
```

**Retry (429)**:
```
WARN  llm rate limited, will retry status_code=429
WARN  keywords extraction retry attempt=1
WARN  llm rate limited, will retry status_code=429
WARN  keywords extraction retry attempt=2
INFO  keywords extracted (after retry)
```

**Panic**:
```
ERROR panic recovered in message processing panic="test panic"
ERROR message processing failed error="panic: test panic"
WARN  requeuing failed message attempt=1
```

---

## Clean Up

```bash
# Reset test data
DELETE FROM event_logs WHERE id >= 2000000000000;
DELETE FROM issues WHERE id >= 1000000000000;

# Clear Redis
redis-cli FLUSHDB
```
