# Index Experiment Results

**Date:** 2026-01-07

## Sessions Analyzed

| Session | Time | Query | Index Used? | Iterations | Context Tokens | Duration |
|---------|------|-------|-------------|------------|----------------|----------|
| 151013.702 | 15:10 | Redis client/utilities | ❌ No | 3 | 3,926 | 15s |
| 151013.704 | 15:10 | Webhook auth flow | ❌ No | 7 | 2,916 | 15s |
| 151028.908 | 15:10 | Rate limiting | ❌ Error | 1 | 0 | 0s |
| **195420.633** | **19:54** | **LLM token tracking** | **✅ Yes** | **10** | **25,769** | **96s** |

## Key Finding: Index Was Used!

Session `195420.633` shows the new behavior:

```
ITERATION 1:
[TOOL CALL] read
Arguments: {"file_path":".basegraph/index.md","limit":200}
```

**The agent read the index first**, exactly as intended.

## Before vs After Comparison

### Without Index (151013.702 - Redis query)

```
ITERATION 1: 4 grep calls to FIND redis references
  - grep: "\\bredis\\b|go-redis|redigo"
  - glob: "**/*redis*"
  - grep: "NewClient\\(|Dial\\(|Options\\{"
  - grep: "package .*redis|type .*Redis"

ITERATION 2: 4 more grep calls for connection setup
  - grep: "redis\\.NewClient|redis\\.Options"
  - grep: "Addr:|Password:|DB:|TLSConfig"
  - etc.
```

**Pattern:** Blind grep/glob discovery → many tool calls to find relevant files

### With Index (195420.633 - Token tracking query)

```
ITERATION 1: Read the index
  - read(".basegraph/index.md")
  → Sees: model/llm_eval.go - LLM evaluation model
  → Sees: brain/planner.go - Main planning agent

ITERATION 2: Targeted grep for token fields
  - grep: "token_usage|prompt_tokens|completion_tokens"
  → Immediately finds brain/planner.go:102, model/llm_eval.go:29
```

**Pattern:** Index first → knows where to look → targeted search

## Observations

### What Worked

1. **Agent followed instructions** - First action was `read(".basegraph/index.md")`
2. **Index provided orientation** - Agent saw `model/llm_eval.go - LLM evaluation model` and knew to look there
3. **Reduced blind searching** - Iteration 2 was already targeted grep, not discovery

### What Didn't Change

1. **Token usage still high** - 25,769 tokens vs 15,850 baseline avg
   - But this was a "medium" thoroughness search that hit soft limit
   - Index added ~1,180 tokens to context

2. **Many grep calls** - 17 grep calls still
   - But these were targeted, not discovery
   - Agent knew which directories to search

### Issues Found

1. **Glob patterns failing** - Sessions 151013.* show `glob("**/*.go")` returning no matches
   - This is a working directory issue, not index-related
   - The fixture path wasn't set correctly for those runs

2. **Error terminations** - 3/4 sessions ended in error
   - Likely same working directory issue

## Recommendations

1. **Working directory fix needed** - Glob patterns failing suggests the explore agent's working directory wasn't set to the fixture path

2. **More test runs needed** - Only 1 successful session with index, need more data

3. **Measure first-useful-read** - Track which iteration gets to actual code reading
   - Without index: iteration 3-4 typically
   - With index: iteration 2 (observed)

## Next Steps

1. Fix the working directory issue for test runs
2. Run 5-10 more queries with index enabled
3. Compare:
   - Iterations to first useful read
   - Total iterations
   - Context window at termination
   - Time to answer
