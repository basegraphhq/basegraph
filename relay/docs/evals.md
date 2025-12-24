# LLM Pipeline Quality & Evaluation Guide

> How to systematically measure, improve, and maintain quality in the Relay pipeline.

## Philosophy

**You can't improve what you don't measure.**

Every LLM call is logged to `llm_evals`. This creates a feedback loop:
1. **Capture** → Log every input/output
2. **Rate** → Human review of samples
3. **Measure** → Automated metrics against ground truth
4. **Iterate** → Improve prompts, re-evaluate

---

## For Backend Engineers New to ML/LLM Evals

If you're coming from a traditional backend background, here's how to think about this.

### Why Evals? (The "Unit Tests" Analogy)

| Traditional Code | LLM Pipelines |
|-----------------|---------------|
| Unit tests catch regressions | Evals catch quality drops |
| Type system enforces contracts | Schema validation enforces structure |
| Logs help debug production issues | Captured inputs/outputs help debug bad responses |
| Metrics track performance | Ratings track quality |

**The core problem:** LLMs are non-deterministic. The same input can produce different outputs. You can't write `assert output == "exact string"` like you would for a function.

Instead, you need to ask: *"Is this output good enough?"* — which requires:
1. **Capturing** what went in and what came out
2. **Measuring** quality (human ratings, automated metrics)
3. **Comparing** across prompt versions

### The Feedback Loop Visualized

```
┌─────────────────────────────────────────────────────────────┐
│                                                             │
│   Production Pipeline                                       │
│   ┌──────────┐    ┌──────────┐    ┌──────────┐             │
│   │ Issue    │ → │ Keywords │ → │ Planner  │ → ...        │
│   │ comes in │    │ Extract  │    │          │             │
│   └──────────┘    └────┬─────┘    └──────────┘             │
│                        │                                    │
│                        ▼                                    │
│                   llm_evals table                          │
│                   (input, output, model, latency)          │
│                        │                                    │
└────────────────────────┼────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│   Evaluation Layer (offline)                                │
│                                                             │
│   Human Rating:     "This extraction missed 'auth'"  → 3/5 │
│   Automated Metric: precision=0.8, recall=0.6, F1=0.69    │
│                                                             │
│   Insight: "Keywords prompt misses security-related terms" │
│                                                             │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│   Iteration                                                 │
│                                                             │
│   1. Update prompt: "Pay attention to security terms..."   │
│   2. Bump version: keywords_v1 → keywords_v2               │
│   3. Run against same inputs                               │
│   4. Compare: Did F1 improve? Did ratings go up?           │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### Why Now? (Building the Foundation)

You're at the **perfect moment** to set this up:

1. **No production data yet** — You can modify the schema freely
2. **First pipeline stage** — Keywords extractor is simple, good for learning
3. **Cheap to log** — Adding one DB insert per LLM call is trivial
4. **Expensive to retrofit** — If you wait until you have 10 pipeline stages and production traffic, you'll wish you had this data

**The ML equivalent of:** "Always set up logging/monitoring before going to production, not after things break."

### What's Captured Now vs What Comes Later

**Now (Infrastructure) ✅**
```
llm_evals table:
├── input_text      → What prompt was sent
├── output_json     → What the LLM returned
├── model           → "gpt-4o-mini"
├── temperature     → 0.1
├── prompt_version  → "keywords_v1"
├── latency_ms      → 450
└── created_at      → When it happened
```

This is **observability**. Like having logs, but for LLM calls.

**Next (Human Evaluation)**
```
├── rating          → 1-5 score from human reviewer
├── rating_notes    → "Missed 'rate_limiting' keyword"
└── rated_by_user_id
```

This is **labeling**. You (or someone) looks at samples and says "good/bad/okay."

**Then (Automated Metrics)**
```
├── expected_json   → Ground truth (the "correct" answer)
└── eval_score      → Computed F1/precision/recall
```

This is **regression testing for prompts**. You build a "golden dataset" of (input, expected_output) pairs, then run new prompts against it.

### Practical Example

Let's say your keywords extractor processes this issue:

> **Input:** "Add Twilio integration for SMS notifications with rate limiting"

**Output v1:**
```json
{"keywords": ["twilio", "sms", "notifications"]}
```

A human rates this **3/5** with note: *"Missed 'rate_limiting' - important for architecture"*

You update the prompt to emphasize technical terms → **keywords_v2**

**Output v2:**
```json
{"keywords": ["twilio", "sms", "notifications", "rate_limiting", "integration"]}
```

Same human rates this **5/5**.

Now you have:
- Evidence that v2 is better
- A "golden" example to test future versions against
- Data to catch if v3 regresses

### The Mindset Shift

| Backend Mindset | ML/LLM Mindset |
|----------------|----------------|
| "Does the code compile?" | "Does the output make sense?" |
| "Does the test pass?" | "Is the output good enough?" |
| "One correct answer" | "Range of acceptable answers" |
| "Fix bugs in code" | "Fix bugs in prompts" |
| "Deterministic" | "Probabilistic" |

**Key insight:** You can't debug a prompt by reading it. You debug by looking at examples of what it produced and asking "why did it do that?"

### Your Learning Path

```
Week 1-2 (Now)
├── Wire logging into KeywordsExtractor
├── Run pipeline on 20-30 real issues
└── Look at the outputs manually

Week 3-4
├── Rate 30 samples (just eyeball: 1-5)
├── Notice patterns ("it keeps missing X")
└── Try one prompt improvement

Month 2
├── Build simple rating UI
├── Export 5-star samples as "golden set"
└── Create automated eval script

Later
├── A/B test prompt versions
├── Track quality trends weekly
├── Set up alerts for quality drops
```

### Schema Design Note

The `llm_evals` table includes `issue_id` and `workspace_id` for context. You can always JOIN to get more details (repo, integration type, etc.). If analytics queries become painful due to frequent JOINs, consider denormalizing later. Start normalized.

---

## 1. Data Collection (Already Built)

Every pipeline stage logs to `llm_evals`:

```sql
-- See what keywords extraction produced
SELECT
    id,
    LEFT(input_text, 100) as input_preview,
    output_json,
    latency_ms,
    rating
FROM llm_evals
WHERE stage = 'keywords'
ORDER BY created_at DESC
LIMIT 10;
```

**What's captured:**
- `input_text` — The exact prompt sent to LLM
- `output_json` — The structured response
- `model`, `temperature`, `prompt_version` — Reproducibility
- `latency_ms`, `prompt_tokens`, `completion_tokens` — Cost tracking
- `rating`, `rating_notes` — Human evaluation
- `expected_json`, `eval_score` — Automated comparison

---

## 2. Human Evaluation (Rating Flow)

### When to Rate

Rate samples when:
- **First launch** — Rate 20-30 samples to establish baseline
- **After prompt changes** — Compare before/after
- **Customer complaints** — Investigate specific issues
- **Weekly sampling** — Ongoing quality monitoring

### Rating Scale

| Rating | Meaning | Action |
|--------|---------|--------|
| 5 | Perfect | Use as ground truth |
| 4 | Good | Minor issues, acceptable |
| 3 | Acceptable | Works but could be better |
| 2 | Poor | Missing important keywords |
| 1 | Failed | Wrong or useless output |

### Rating SQL

```sql
-- Get unrated samples
SELECT id, input_text, output_json
FROM llm_evals
WHERE stage = 'keywords' AND rating IS NULL
ORDER BY created_at DESC
LIMIT 10;

-- Rate a sample
UPDATE llm_evals
SET rating = 4,
    rating_notes = 'Missed rate_limiting but got main terms',
    rated_by_user_id = 1,
    rated_at = NOW()
WHERE id = 123;
```

### Building a Rating UI

Add an endpoint to your dashboard:
```
GET  /api/evals/unrated?stage=keywords&limit=10
POST /api/evals/:id/rate  {rating: 4, notes: "..."}
```

---

## 3. Ground Truth Dataset

### What is Ground Truth?

A set of (input, expected_output) pairs that define "correct" behavior.

### Building Ground Truth

**Step 1: Start with 5-star ratings**
```sql
-- Export 5-star rated samples as ground truth
SELECT id, input_text, output_json as expected_json
FROM llm_evals
WHERE stage = 'keywords'
  AND rating = 5
ORDER BY created_at DESC;
```

**Step 2: Create a golden dataset file**
```json
// relay/evals/keywords_golden.json
[
  {
    "id": "GT-001",
    "input": "Add Twilio support for SMS notifications...",
    "expected": {
      "keywords": [
        {"value": "twilio", "weight": 0.9, "source": "title"},
        {"value": "sms", "weight": 0.85, "source": "title"},
        {"value": "notifications", "weight": 0.8, "source": "title"}
      ]
    }
  }
]
```

**Step 3: Run evals against it**
```bash
go run cmd/eval/main.go --stage=keywords --golden=evals/keywords_golden.json
```

---

## 4. Automated Metrics

### Keywords Extraction Metrics

| Metric | Formula | Target |
|--------|---------|--------|
| **Precision** | (correct keywords) / (returned keywords) | > 0.8 |
| **Recall** | (correct keywords) / (expected keywords) | > 0.7 |
| **F1 Score** | 2 * (P * R) / (P + R) | > 0.75 |
| **Weight MAE** | Mean absolute error of weights | < 0.15 |

### Eval Script Structure

```go
// cmd/eval/keywords.go
type KeywordsEval struct {
    Golden   []GoldenCase
    LLM      llm.Client
}

func (e *KeywordsEval) Run(ctx context.Context) EvalReport {
    var results []CaseResult

    for _, golden := range e.Golden {
        // Run extraction
        actual := e.extract(ctx, golden.Input)

        // Compute metrics
        precision := computePrecision(actual, golden.Expected)
        recall := computeRecall(actual, golden.Expected)
        f1 := 2 * precision * recall / (precision + recall)

        results = append(results, CaseResult{
            ID:        golden.ID,
            Precision: precision,
            Recall:    recall,
            F1:        f1,
            Passed:    f1 >= 0.75,
        })
    }

    return EvalReport{
        Stage:     "keywords",
        Cases:     results,
        AvgF1:     avg(results, "f1"),
        PassRate:  passRate(results),
    }
}
```

---

## 5. Prompt Versioning

Track which prompt produced which results:

```go
const keywordsPromptV1 = "keywords_v1"

// In keywords.go
err = e.llm.Chat(ctx, llm.Request{
    // ...
    PromptVersion: &keywordsPromptV1,  // Track version
}, &response)
```

**Compare versions:**
```sql
SELECT
    prompt_version,
    COUNT(*) as total,
    AVG(rating) as avg_rating,
    AVG(eval_score) as avg_f1
FROM llm_evals
WHERE stage = 'keywords'
  AND created_at > NOW() - INTERVAL '7 days'
GROUP BY prompt_version
ORDER BY avg_rating DESC;
```

---

## 6. Weekly Quality Dashboard

Run these queries weekly:

```sql
-- Quality trends
SELECT
    DATE_TRUNC('day', created_at) as day,
    stage,
    COUNT(*) as calls,
    AVG(rating) as avg_rating,
    AVG(latency_ms) as avg_latency,
    SUM(prompt_tokens + completion_tokens) as total_tokens
FROM llm_evals
WHERE created_at > NOW() - INTERVAL '7 days'
GROUP BY 1, 2
ORDER BY 1, 2;

-- Failure rate (ratings 1-2)
SELECT
    stage,
    COUNT(*) FILTER (WHERE rating <= 2) as failures,
    COUNT(*) FILTER (WHERE rating IS NOT NULL) as rated,
    ROUND(100.0 * COUNT(*) FILTER (WHERE rating <= 2) /
          NULLIF(COUNT(*) FILTER (WHERE rating IS NOT NULL), 0), 1) as failure_pct
FROM llm_evals
WHERE created_at > NOW() - INTERVAL '7 days'
GROUP BY stage;
```

---

## 7. Customer Feedback Loop

When customers report bad specs:

1. **Find the issue** → Get issue_id from support ticket
2. **Pull all evals** → `SELECT * FROM llm_evals WHERE issue_id = X`
3. **Review each stage** → Keywords → Planner → Gap Detector → Spec
4. **Identify root cause** → Which stage failed?
5. **Add to ground truth** → Create a golden case for this failure
6. **Fix & validate** → Update prompt, run eval, confirm improvement

---

## 8. Cost Tracking

```sql
-- Daily cost estimate (rough)
SELECT
    DATE_TRUNC('day', created_at) as day,
    stage,
    SUM(prompt_tokens) as total_prompt_tokens,
    SUM(completion_tokens) as total_completion_tokens,
    -- gpt-4o-mini pricing: $0.15/1M input, $0.60/1M output
    ROUND((SUM(prompt_tokens) * 0.15 + SUM(completion_tokens) * 0.60) / 1000000, 4) as est_cost_usd
FROM llm_evals
WHERE created_at > NOW() - INTERVAL '30 days'
GROUP BY 1, 2
ORDER BY 1, 2;
```

---

## 9. Alerts & Thresholds

Set up monitoring for:

| Metric | Warning | Critical |
|--------|---------|----------|
| Avg rating (7d) | < 3.5 | < 3.0 |
| Failure rate | > 10% | > 20% |
| Avg latency | > 3s | > 5s |
| Daily cost | > $10 | > $50 |

---

## 10. Iteration Workflow

### The Quality Loop

```
┌─────────────────────────────────────────────────────────────┐
│                                                             │
│   1. Measure                                                │
│      └─ Run evals, check ratings                           │
│              ↓                                              │
│   2. Identify                                               │
│      └─ Find failure patterns                               │
│              ↓                                              │
│   3. Hypothesize                                            │
│      └─ "Adding examples will improve recall"               │
│              ↓                                              │
│   4. Implement                                              │
│      └─ Update prompt (new version)                         │
│              ↓                                              │
│   5. Evaluate                                               │
│      └─ Run against golden set                              │
│              ↓                                              │
│   6. Deploy (if improved) ──────────────────────────→ Loop  │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### Before Any Prompt Change

1. Run eval against current golden set → Record baseline F1
2. Make change with new `prompt_version`
3. Run eval again → Compare F1
4. Only deploy if F1 improves or stays same

---

## Quick Start Checklist

- [ ] Run `make migrate-up` to create `llm_evals` table
- [ ] Wire up eval logging in keywords extractor (TODO)
- [ ] Rate 20 samples manually
- [ ] Export 5-star samples as golden set
- [ ] Create eval script for keywords
- [ ] Set up weekly quality review

---

## Files Created

| File | Purpose |
|------|---------|
| `migrations/.../init_schema.sql` | Added `llm_evals` table |
| `core/db/queries/llm_evals.sql` | SQLC queries |
| `internal/model/llm_eval.go` | Domain model |
| `internal/store/llm_eval.go` | Store implementation |
| `internal/store/interfaces.go` | Added `LLMEvalStore` interface |

---

## Next Steps

1. **Wire logging into KeywordsExtractor** — Log every call to `llm_evals`
2. **Build rating UI** — Simple dashboard page for rating samples
3. **Create `cmd/eval/`** — CLI for running automated evals
4. **Populate golden set** — Start with 10 hand-crafted examples
