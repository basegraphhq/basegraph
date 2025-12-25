# Keywords Extractor Design

> Design decisions and iteration log for the keywords extraction stage.

## Problem Statement

**Input:** Software issue (title, description, discussions)
**Output:** Keywords for code search
**Goal:** Extract keywords that will find relevant code in Typesense → ArangoDB pipeline

### The Core Challenge

```
Natural Language          →    Code Identifiers
"rate limiting"           →    RateLimiter, throttle, rate_limit_middleware
"authentication issue"    →    auth_handler, login, session_manager
```

Keywords must bridge this gap. If we miss a keyword, graph traversal cannot recover.

---

## Early-Stage Approach (Current Phase)

**Philosophy:** Don't add complexity until you have evidence you need it.

### Phase 1: Collect Ground Truth (NOW)

Before iterating on prompts, build a dataset of (issue, expected_keywords) pairs.

**Process:**
1. Take 10-20 real issues from GitLab/GitHub
2. For each issue, manually extract keywords YOU would use to find code
3. Run the extractor
4. Compare: What did it get? What did it miss? What garbage?

**Template:**
```
Issue #1: "Login fails when user has special characters in password"

Human keywords:
- login (concept)
- authentication (concept)
- password (concept)
- special_characters (concept)
- validation (concept)
- user_auth (entity - likely function/class name)

LLM keywords:
[run extractor, paste output]

Analysis:
- Got: ...
- Missed: ...
- Garbage: ...
```

### Phase 2: Identify Failure Patterns

After 10-20 examples, patterns will emerge:
- "It always misses X type of keyword"
- "It keeps extracting Y which is useless"
- "It doesn't understand Z"

**Only then** modify the prompt to address specific failures.

### Phase 3: Measure Improvement

Before/after comparison:
- Run same 20 issues through old prompt
- Run same 20 issues through new prompt
- Count: improvements, regressions, no change

---

## Ground Truth Dataset

| Issue | Source | Human Keywords | Notes |
|-------|--------|----------------|-------|
| | | | |
| | | | |

(Fill this in as you collect examples)

---

## Observed Failure Patterns

| Pattern | Example | Frequency | Fix |
|---------|---------|-----------|-----|
| | | | |

(Fill this in as you observe failures)

---

## Design Decisions

### Decision 1: Recall > Precision

**Choice:** Optimize for recall (don't miss relevant code) over precision (avoid noise).

**Rationale:**
- Typesense can handle some noise in search
- Missing a keyword = missing code = incomplete analysis
- Downstream filtering is easier than recovering missed context

### Decision 2: Keyword Categories

**Choice:** Three categories: `entity`, `concept`, `library`

| Category | Description | Examples |
|----------|-------------|----------|
| `entity` | Code identifiers (functions, classes, files) | `user_service`, `handleAuth`, `config.yaml` |
| `concept` | Technical patterns that map to code areas | `authentication`, `rate_limiting`, `caching` |
| `library` | External dependencies | `redis`, `twilio`, `postgres` |

**Rationale:** Enables filtered search (e.g., "only search for library matches first").

### Decision 3: Weight = Relevance, Not Confidence

**Choice:** Weight (0-1) represents relevance to the issue, not likelihood of existing in code.

**Current Scale:**
- 0.9-1.0: Very specific identifier (`SendGridProvider`)
- 0.7-0.8: Specific technical term (`rate_limiter`)
- 0.5-0.6: Broader concept (`api`, `service`)
- 0.3-0.4: Generic but potentially useful (`config`)

**Open Question:** Should we add a separate `confidence` field?

### Decision 4: Normalization

**Choice:** Lowercase, snake_case for multi-word terms.

```
"Rate Limiting" → rate_limiting
"UserService"   → user_service
"SMS"           → sms
```

### Decision 5: Retriever Handles Naming Conventions (Not Extractor)

**Choice:** Keywords extractor outputs canonical snake_case. The codegraph retriever handles matching different naming conventions (camelCase, kebab-case, PascalCase).

**Rationale:**
- Separation of concerns: extractor extracts *what*, retriever handles *how*
- Retriever knows the codebase's conventions (can learn or be configured)
- Typesense/search layer can do fuzzy matching
- Simpler keyword schema, less data duplication

**How Claude Code does it:**
1. Extract the semantic concept ("rate limiting")
2. Search with patterns or exact match
3. If no results, try variations at the search layer

This is cleaner than generating all variants upfront.

---

## Current Implementation

**File:** `internal/pipeline/keywords.go`

**Schema:**
```go
type KeywordItem struct {
    Value    string  // lowercase, snake_case
    Weight   float64 // 0.0-1.0 relevance
    Category string  // entity, concept, library
    Source   string  // title, description, discussion
}
```

**Prompt Strategy:**
- Explicit guidance on what categories mean
- Weight scale with examples
- Negative examples (what NOT to extract)
- Max 15 keywords

---

## Open Questions

### Q1: Should we add variants?

**Problem:** Issue says "rate limiting", code might use `RateLimiter`, `throttle`, `rate_limit`.

**Option A:** Generate variants in prompt
```json
{
  "value": "rate_limiting",
  "variants": ["rate_limiter", "throttle", "RateLimiter"]
}
```

**Option B:** Generate variants post-extraction (deterministic transformation)

**Option C:** Let Typesense fuzzy matching handle it

### Q2: Should we add confidence?

**Problem:** Explicitly mentioned terms vs inferred concepts have different reliability.

```
"Add Twilio integration"
  → "twilio" (HIGH confidence - explicit)
  → "sms" (MEDIUM confidence - inferred from Twilio)
  → "notification" (LOW confidence - speculative)
```

**Tradeoff:** More complex schema vs better search prioritization.

### Q3: Should we extract in layers?

**Current:** Single flat extraction.

**Alternative:** Layered approach:
1. **Explicit:** Verbatim technical terms (high confidence)
2. **Inferred:** Concepts implied by context (medium confidence)
3. **Patterns:** Likely code structures (lower confidence)

---

## Iteration Log

### v1 (Initial)
- Basic prompt: "Extract technical keywords"
- No categories, simple weight
- Issues: Too many generic words, no guidance on what's useful for code search

### v2 (Current)
- Added categories: entity, concept, library
- Explicit negative examples (don't extract: add, fix, implement...)
- Better weight guidance with examples
- snake_case normalization
- Max 15 keywords

### v3 (Planned)
- [ ] Add variants for high-weight terms
- [ ] Consider confidence field
- [ ] Test with real issues and measure against Typesense results

---

## Evaluation Plan

Once Typesense is set up:

1. **Collect samples:** Run extractor on 50 real issues
2. **Measure recall:** Do extracted keywords find the expected code?
3. **Identify failures:** What code was missed? Why?
4. **Iterate prompt:** Add examples of failure cases

### Metrics to Track

| Metric | Definition | Target |
|--------|------------|--------|
| Recall@10 | % of relevant code found in top 10 Typesense results | > 90% |
| Keyword precision | % of keywords that match something in codebase | > 70% |
| Avg keywords per issue | Fewer = more focused | 8-12 |

---

## References

- [EVAL_QUALITY_GUIDE.md](./EVAL_QUALITY_GUIDE.md) - How to evaluate and iterate on LLM outputs
- `internal/pipeline/keywords.go` - Implementation
- `internal/model/issue.go` - Keyword struct definition
