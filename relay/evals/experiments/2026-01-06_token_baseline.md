# Explore Agent Token Consumption Baseline

**Analysis Date:** 2026-01-07
**Data Source:** `relay/debug_logs/` (32 explore sessions, 29 planner sessions)

## Executive Summary

The explore agent's cold-start problem is clearly visible in the data:
- **72% of sessions hit the soft token limit** (18/25 valid sessions)
- Average context window at termination: **15,850 tokens**
- Average **16 iterations** per exploration
- Heavy tool usage for discovery: **258 grep, 140 glob, 135 read** calls total

## Explore Agent Metrics

### Token Consumption

| Metric | Min | Max | Avg | Total |
|--------|-----|-----|-----|-------|
| Context Window (final) | 2,173 | 25,872 | 15,850 | 396,256 |
| Completion Tokens | 366 | 5,904 | 3,027 | 75,683 |

### Iterations & Duration

| Metric | Min | Max | Avg |
|--------|-----|-----|-----|
| Iterations | 3 | 33 | 16.0 |
| Duration (ms) | 12,244 | 142,117 | 70,395 |

### Termination Reasons

| Reason | Count |
|--------|-------|
| natural | 21 |
| error | 3 |
| iteration_limit | 1 |

### Limit Hits (25 valid sessions)

| Limit Type | Hit Count |
|------------|-----------|
| Soft limit (80% of target) | 18 (72%) |
| Hard limit | 0 |
| Iteration limit | 1 |
| Doom loop | 0 |

### Tool Usage (total across all sessions)

| Tool | Call Count |
|------|------------|
| grep | 258 |
| glob | 140 |
| read | 135 |
| bash | 69 |

**Observation:** High grep/glob usage indicates significant time spent on *discovery* rather than *reading known locations*.

## Planner Metrics

### Token Consumption

| Metric | Min | Max | Avg | Total |
|--------|-----|-----|-----|-------|
| Prompt Tokens | 2,323 | 23,744 | 7,599 | 220,385 |
| Completion Tokens | 234 | 2,761 | 976 | 28,317 |

### Explore Calls

- Sessions with explore calls: **10/29** (34%)
- Avg explore calls when exploring: **2.8**
- Avg prompt tokens when exploring: **13,403** (vs 7,599 overall)

## Cost Implications

Assuming Sonnet 3.5 pricing ($3/1M input, $15/1M output):

### Per Explore Call (avg)
- Input: 15,850 tokens × $0.000003 = **$0.048**
- Output: 3,027 tokens × $0.000015 = **$0.045**
- **Total: ~$0.09 per explore**

### Per Planner Session with Explores (avg 2.8 explores)
- Planner: $0.04 input + $0.015 output = $0.055
- Explores: 2.8 × $0.09 = $0.25
- **Total: ~$0.31 per planning session**

## Cold-Start Pattern Evidence

From debug log `explore_20260106-213430.112.txt`:

```
Query: How does service.EventIngestService.Ingest handle incoming mention events...

ITERATION 1: 3 grep calls to FIND where EventIngestService is defined
ITERATION 2: read the file after discovering its location
ITERATION 3+: actual analysis work
```

The agent spends **iterations 1-2 on pure discovery** before doing useful work.

## Projected Impact of Index

With a file → type index, the agent could skip discovery:
- Current first useful read: iteration 2-3
- With index: iteration 1

**Conservative estimate:**
- Save 2-3 iterations × ~800 tokens/iteration = **~2,000 tokens per explore**
- At 2.8 explores per planning session = **~5,600 tokens saved**
- ~$0.02 saved per session

**More importantly:** Faster time-to-answer. Currently 70 seconds avg, could reduce by 10-20 seconds.

## Recommendations

1. **MVP Index:** File paths + exported type names, ~500-1000 tokens
2. **Injection Point:** System prompt or first user message
3. **Expected Savings:** 10-15% token reduction, 15-25% latency reduction
4. **Measurement:** Track `iterations` and `context_window_tokens` before/after
