# Testing & Reset Guide

Quick reference for resetting your local development environment.

## Architecture Overview

**Current Stack:**
- **Redis**: Message queue for events
- **ArangoDB**: Code graph (relationships, nodes)
- **Filesystem**: Code search via `grep`, `glob`, `read` tools

**No Typesense!** Search is now done directly on the filesystem with ripgrep/fd.

---

## Quick Reset Commands

```bash
# Complete reset (Redis + ArangoDB + Re-index)
make reset

# Only clear Redis streams
make reset-redis

# Only clear ArangoDB
make reset-db

# Re-index codegraph only
make reindex-codegraph
```

---

## Detailed Options

### Full Reset
```bash
./scripts/reset.sh
# or
make reset
```
**Does:**
- ✓ Acknowledges all pending Redis messages
- ✓ Trims Redis stream to 0
- ✓ Deletes Redis DLQ stream
- ✓ Deletes ArangoDB database
- ✓ Re-indexes codegraph automatically

**Use when:** Starting fresh for a new test run

---

### Redis Only
```bash
./scripts/reset.sh --redis-only
# or
make reset-redis
```
**Does:**
- ✓ Acknowledges all pending messages
- ✓ Clears main stream (`relay_events`)
- ✓ Clears DLQ stream (`relay_events_dlq`)
- ✗ Leaves ArangoDB untouched

**Use when:** Testing worker message processing without re-indexing

---

### ArangoDB Only
```bash
./scripts/reset.sh --db-only
# or
make reset-db
```
**Does:**
- ✓ Deletes ArangoDB database
- ✗ Leaves Redis streams untouched

**Use when:** Testing codegraph extraction/ingestion without clearing Redis

---

### Reset Without Re-index
```bash
./scripts/reset.sh --no-reindex
```
**Does:**
- ✓ Clears Redis
- ✓ Deletes ArangoDB database
- ✗ Does NOT re-index

**Use when:** You want a clean slate but will manually re-index later

---

## Common Testing Workflows

### 1. Test Worker Processing (Full Stack)
```bash
# Clean slate
make reset

# Start worker with debug logging
BRAIN_DEBUG_DIR=debug_logs make run-worker

# Trigger an event (via API or webhook)
# Watch debug_logs/ folder for planner/retriever output
```

### 2. Test Retriever Only
```bash
# Ensure ArangoDB is indexed
make reindex-codegraph

# Clear only Redis (keep indexed data)
make reset-redis

# Start worker
BRAIN_DEBUG_DIR=debug_logs make run-worker
```

### 3. Test Codegraph Extraction
```bash
# Clear database only
make reset-db

# Re-extract and ingest
make reindex-codegraph

# Verify graph data (see verification section below)
```

### 4. Quick Iteration (No Re-index)
```bash
# Clear Redis only between test runs
make reset-redis

# Worker picks up new messages with existing codegraph data
make run-worker
```

---

## Environment Variables

**Required:**
- `REPO_ROOT` - Path to the repository for filesystem search (e.g., `/Users/you/basegraph/relay`)

**Redis:**
- `REDIS_URL` (default: `redis://localhost:6379/0`)
- `REDIS_STREAM` (default: `relay_events`)
- `REDIS_CONSUMER_GROUP` (default: `relay_group`)
- `REDIS_DLQ_STREAM` (default: `relay_events_dlq`)

**ArangoDB:**
- `ARANGO_URL` (default: `http://localhost:8529`)
- `ARANGO_USERNAME` (default: `root`)
- `ARANGO_PASSWORD` (default: `your-secret-password`)
- `ARANGO_DATABASE` (default: `codegraph`)

---

## Verification Commands

### Check Redis Status
```bash
# Stream length
redis-cli -u redis://localhost:6379/0 XLEN relay_events

# Pending messages
redis-cli -u redis://localhost:6379/0 XPENDING relay_events relay_group
```

### Check ArangoDB
```bash
# List databases
curl -s "http://localhost:8529/_db/_system/_api/database" \
  -u "root:your-secret-password" | jq '.result'

# Count nodes in graph
curl -s "http://localhost:8529/_db/codegraph/_api/cursor" \
  -u "root:your-secret-password" \
  -H "Content-Type: application/json" \
  -d '{"query": "RETURN {functions: LENGTH(functions), types: LENGTH(types), calls: LENGTH(calls)}"}' \
  | jq '.result'
```

### Test Filesystem Tools (without worker)
```bash
# Test ripgrep
rg "NewRetriever" /Users/nithin/basegraph/relay --max-count 5

# Test fd
fd "retriever" /Users/nithin/basegraph/relay --type f

# Test glob
ls /Users/nithin/basegraph/relay/internal/brain/*.go
```

---

## Debug Logs

Enable debug logging to see retriever and planner internals:

```bash
export BRAIN_DEBUG_DIR=debug_logs
make run-worker
```

Logs are written to:
- `debug_logs/planner_TIMESTAMP.txt` - Shows planner iterations and retrieve calls
- `debug_logs/retriever_TIMESTAMP.txt` - Shows grep/glob/read/graph tool calls and results

**What to look for:**
- Iteration count (should be < 20 for retriever)
- Tool calls and their arguments
- Verbose "thinking" output after soft limit (indicates bug)
- Search patterns and results

---

## Retriever Tools Reference

The retriever has 4 tools:

1. **grep** - Search file contents with regex
   ```json
   {"pattern": "func.*Retriever", "include": "*.go", "limit": 50}
   ```

2. **glob** - Find files by path pattern
   ```json
   {"pattern": "**/brain/*.go"}
   ```

3. **read** - Read file contents with line ranges
   ```json
   {"file": "internal/brain/retriever.go", "start_line": 1, "num_lines": 200}
   ```

4. **graph** - Query code relationships
   ```json
   {"operation": "callers", "target": "basegraph.app/relay/internal/brain.NewRetriever", "depth": 1}
   ```

---

## Tips

1. **Always reset before important tests** to ensure clean state
2. **Use `--redis-only` for fast iteration** when codegraph data is stable
3. **Check debug logs first** if retriever behavior seems off
4. **Verify REPO_ROOT is set** - retriever will fail without it
5. **Install ripgrep and fd** for best performance:
   ```bash
   brew install ripgrep fd  # macOS
   apt install ripgrep fd-find  # Ubuntu
   ```
6. **Use DLQ stream** to inspect failed messages:
   ```bash
   redis-cli XREAD STREAMS relay_events_dlq 0
   ```

---

## Troubleshooting

### "REPO_ROOT environment variable is required"
```bash
export REPO_ROOT=/Users/nithin/basegraph/relay
make run-worker
```

### "Graph error: collection or view not found"
```bash
# Re-index the codegraph
make reindex-codegraph
```

### Retriever hitting iteration limit
- Check debug logs for repeated failed searches
- May indicate missing data in ArangoDB (re-index)
- Or search patterns that don't match codebase

### Worker not processing messages
```bash
# Check Redis stream has messages
redis-cli XLEN relay_events

# Check consumer group status
redis-cli XINFO GROUPS relay_events
```
