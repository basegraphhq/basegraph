# CLI AI Coding Assistant – Relay v1

Brutal honesty: “CLI Codex/Claude Code” is a graveyard of half-baked toys. If you can’t beat Aider/Continue/Cursor on reliability and repo-scale context, it’s trash. If you ship a chat box that can’t safely edit code, it’s trash. If you can’t measure win-rate, it’s trash.

## Reality Check
- Claude Code/Cursor win on inline edits + fast context + eval-proven reliability.
- A CLI only wins if it’s surgical, scriptable, and trustworthy with diffs.
- General “AI helper” with no killer wedge = commodity.

## Pick a Wedge (choose ONE)
- Repo-aware refactors that “just work” with safe diffs and reversible commits.
- Test authoring/repair loop that gets red→green automatically.
- PR automation: summary, risk flags, targeted reviewers, dead-code diffs.
- Monorepo graph navigation: “jump to impact of change” + code maps.
If you say “all of the above,” that’s unfocused trash. Pick one.

## V1 Non‑Negotiables
- Deterministic edits: always output a minimal diff, never freestyle file dumps.
- Strict tools: read/search/edit/run gated behind policies; no shell YOLO.
- Context discipline: only relevant files; explain why each chunk is included.
- Repro logs: every action saved; single command to replay.
- Offline/local model option (Ollama) to pass the “no source leaves laptop” test.

## Architecture (CLI-first, robust)
- Frontend: Ink TUI for sessions; commands: `ai ask|edit|review|test|pr`.
- Orchestrator: Node/TS worker with structured tool-calls, not ad-hoc prompts.
- Tools:
  - Repo: `rg` search, path allowlist/denylist, binary filters, secret scanner.
  - Editor: AST-aware patcher (tree-sitter) → `git apply` diff; conflict resolver.
  - Runner: `npm test`/`go test`/`pytest` adapters; capture artifacts.
  - Context: chunker (semantic+path heuristics), recency cache, embedding index (SQLite+FAISS/Qdrant optional).
- Models: pluggable drivers (OpenAI, Anthropic, DeepSeek, Local via Ollama), streaming, JSON-mode for tool calls.
- Policy: per-repo config (`.ai.yml`) for model, privacy, tool permissions.

## Model Choices (pragmatic)
- Cloud: `gpt-4.1/4.1-mini` or `claude-3.5-sonnet/haiku` for coding + tool use.
- Local: `qwen2.5-coder / deepseek-coder / mistral-nemo` via Ollama. Expect lower edit precision; cover with stronger retrieval + constrained format.
- Don’t chase “reasoning” buzz unless it lifts your evals.

## Context & Retrieval
- Multi-stage: heuristic file shortlist → `rg` hits → embedding rerank → line spans.
- Keep context budget: target <60KB per call; never dump repo.
- Track “evidence” for every answer: file:line included, why it was selected.

## Editing & Safety
- Never write files directly from free-text. Force model to emit a unified diff.
- Validate diff applies cleanly; auto-merge if trivial; otherwise open a conflict pane.
- Gate high-risk ops (delete/mass rename) behind explicit confirmation; dry-run by default.
- Secrets guard: block outbound tokens if diff touches `.env`, keys, or large chunks of `package-lock.json`.

## CLI UX (no fluff)
- `ai ask "Why is test X flaky?"` → shows files it read.
- `ai edit src/foo.ts --goal "Add retry with backoff"` → preview diff → apply.
- `ai test` → runs focused tests based on changed files; model proposes fixes.
- `ai review` → PR summary + risk hotspots + “tests most likely missing.”
- `ai pr --title "feat: …"` → generates body from diffs; links risks.

## Evaluation (without this, it’s vibes = trash)
- Golden tasks per repo: 30–100 scripted prompts (edits, fixes, refactors).
- Metrics:
  - Edit success rate (diff applies, builds, tests pass).
  - Overwrite rate (percent of diff unrelated; target <10%).
  - Latency budget: P95 < 8s ask, < 20s edit+test.
  - Token cost per successful change.
- Benchmarks to sanity check: SWE-bench-lite, HumanEval+Py, AiderBench-like edit tasks. Build your own repo-specific harness.

## Distribution & GTM
- Target heavy terminal users and CI bots; don’t chase IDE parity.
- Ship single binary via `npm i -g` and `brew tap`. Zero-config defaults.
- Integrate with `pre-commit`, `gh` CLI; dogfood in real OSS repos and publish win-rate dashboards.

## Pricing Reality
- Cloud-only = margin hostage. Offer:
  - Free local (Ollama) + “bring your own API key”.
  - Pro: hosted context index + eval harness + team policies.
- If you can’t show time saved and break-even on model costs, you don’t have a business.

## Security/Privacy
- Opt-out by default for code upload; opt-in per path.
- Redact secrets; chunk smartly; log all outbound bytes and where they came from.
- Enterprise story: on-prem vector store + local LLM or customer’s keys.

## 6‑Week Ruthless Roadmap
- Week 1: CLI skeleton (Ink), config, logging, adapters for `rg`, `git`, runners.
- Week 2: Tool-calling agent with strict schema; unified diff emitter+applier.
- Week 3: Context pipeline (heuristics+embeddings); evidence view in TUI.
- Week 4: Edit loop solid on 5 seed repos; implement `ai test` focused runs.
- Week 5: Eval harness + 50 golden tasks; measure, regressions block merges.
- Week 6: Polish: retries, partial context refresh, cache; brew tap; docs; demo.

## Killer Demo (what convinces me)
- Live on a medium repo: ask to fix a known flaky test; shows exact files used; proposes minimal diff; runs tests; passes; cost+latency printed; single undo.

---

## Domination Plan: Graph‑Powered Go Edits

Brutal thesis: you win by shipping the most reliable, measurable, graph‑aware edit assistant for Go. Everything else is noise.

- Moat: Neo4j code graph drives context, impact, and write set. Competitors guess; you have topology.
- Product wedge: impact‑aware edits/refactors that update all callers/implementors with minimal blast radius, plus targeted builds/tests.
- Non‑negotiables (Go‑specific):
  - Unified diffs only; AST‑validated for Go; unrelated diff < 10%.
  - Graph‑constrained context and write set; evidence for every chunk (file:line, why included).
  - Focused `go build`/`go test` from impact set, not whole repo.
  - Session ledger: graph queries, context, model output, diffs, test results; replayable.

### Graph‑Powered Commands (V1)
- `ai impact func <pkg>.<Func>`: direct/transitive callers, return/param types, owning packages, tests to run.
- `ai impls <pkg>.<Interface>`: structs implementing, missing methods, bulk‑add method skeletons.
- `ai edit <file[:line]> --goal "..."`: context via graph (def + callers/impls), propose unified diff, preview, apply, run focused tests.
- `ai refactor rename func <pkg>.<Old> <New>`: update def + all call sites; suggest shims if external callers exist.
- `ai refactor signature <pkg>.<Func> --to "(ctx context.Context, id string) (User, error)"`: patch def + callers; add adapters if cross‑module.
- `ai deadcode`: zero fan‑in private symbols; propose deletion diff with risk notes.

### Architecture That Wins
- CLI/TUI: Ink shell with `ask|impact|impls|edit|test|review|pr`.
- Orchestrator: strict JSON tool‑calls; zero free‑form writes.
- Tools: Neo4j driver (canned Cypher), tree‑sitter‑go patcher → diff, `git apply`, Go runner for focused builds/tests, `rg` search/filters.
- Models: pluggable (OpenAI/Anthropic/DeepSeek/Ollama). Default BYO key + local fallback.

### UX That Converts
- Evidence view toggle: shows exactly what the model saw and why.
- Diff preview: minimal, colorized; block risky ops unless confirmed.
- Targeted test run: prints impacted packages and time saved.

### Safety Gates
- Block deleting public symbols with external callers (outside module) unless overridden.
- Protect `.env`, keys, `go.sum`, vendored dirs.
- No outbound code without explicit path allowlist.

### GTM
- OSS‑first with public eval dashboards vs Cursor/Aider on real repos.
- Local‑first privacy; enterprise: on‑prem Neo4j + customer model/keys.
- Distribution: `brew` + `npm i -g`; CI and `gh` integration.

### 90‑Day Kill Plan
- Weeks 1–2: `impact`, `impls`, context assembler, ledger.
- Weeks 3–4: diff‑only `edit`, AST guardrails, focused Go runner.
- Weeks 5–6: eval harness + 50 goldens; caching/retries; partial context refresh.
- Weeks 7–8: PR review with graph risk hotspots; dead code; PR generator.
- Weeks 9–10: stability, latency/cost tuning, Windows; brew tap.
- Weeks 11–12: public bake‑off on 3 OSS repos with dashboards.

### Hard‑to‑Copy Differentiators
- Graph‑constrained context/write set with proofs.
- Targeted test selection with measurable speedup.
- AST‑diff fusion: model proposes, AST validates/constrains.
- Session replay and determinism for enterprise trust.

---

## Evaluation Harness (v0)

Goal: replace vibes with numbers. Measure edit reliability, unrelated diff %, compile/test pass rate, latency, and token cost on realistic Go tasks.

- Metrics (per task and aggregate):
  - Edit success (diff applies cleanly)
  - Build/test pass (focused and/or full depending on task)
  - Unrelated diff rate (<10% target)
  - P50/P95 latency per phase (context, model, apply, test)
  - Token cost per successful change

- Harness structure (research draft—implement later):
  - `benchmarks/` — task specs in YAML; repo fixture metadata.
  - `scripts/eval` — CLI runner that: resets repo, invokes `ai …`, captures artifacts, runs focused tests, writes JSONL results.
  - `results/` — JSONL per run + aggregate markdown with tables/sparklines.

- Task spec (YAML sketch):
  - `id`: stable slug
  - `repo`: path or git URL + commit
  - `goal`: high‑level instruction shown to the CLI
  - `edits_allowed`: paths allowlist/denylist
  - `impacted_symbols`: optional seeds to pre‑compute impact set
  - `accept`: criteria (compile/test pass, file predicates, max unrelated %)
  - `tests`: packages or regex to run; or `impact: true` for auto selection

- Runner loop (deterministic):
  1) Checkout baseline commit; clean working tree.
  2) Precompute impact set via graph if configured.
  3) Invoke CLI with `--dry-run` first; validate diff scope; then apply.
  4) Run `go build` on impacted packages; run targeted tests; fall back to full if configured.
  5) Record metrics, tokens, latencies, artifacts (diff, logs, evidence context).

- Targeted test selection (graph): packages containing callers/implementors + neighbors; prefer regex to narrow test names.

### Seed Golden Tasks (10)
1) Rename exported function across packages
   - Success: def + all call sites updated; compile; tests green; unrelated diff <10%.
2) Add `context.Context` param to hot function; propagate
   - Success: def + callers updated; ctx passed through; compile/tests pass.
3) Change return type from `error` to `*TypedError`
   - Success: def + callers handle new type; adapters where needed; tests pass.
4) Implement new method on interface across all implementors
   - Success: method added with stub or real logic; compile/tests pass.
5) Rename struct field; update constructors/usages
   - Success: all references updated; JSON tags considered; tests pass.
6) Replace deprecated import with new package
   - Success: imports and qualifiers updated; behavior preserved; tests pass.
7) Extract function from complex block; add unit tests
   - Success: new function created; original simplified; tests added/passing.
8) Delete dead private function (fan‑in = 0)
   - Success: only dead code removed; compile/tests pass.
9) Convert method receiver value↔pointer consistently
   - Success: receiver updated; call sites adjusted; interface impls fixed; tests pass.
10) Add retry with backoff to network call path
    - Success: minimal diff; correct backoff; tests prove behavior; no unrelated churn.

Notes:
- All tasks enforce graph‑constrained context/write set and evidence logging.
- Store each run’s artifacts for replay and regression analysis.
