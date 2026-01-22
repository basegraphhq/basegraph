# Codegraph Tool API (v2)

This document specifies the `codegraph` tool contract used by the ExploreAgent.

## Goals

- **High effectiveness for relationship queries**: callers/callees, implementations, usages, and call-flow tracing.
- **Fast + accurate**: minimize tool calls and avoid text-based approximations when the code graph can answer.
- **Self-correcting**: invalid inputs (especially `kind`) return explicit, actionable errors.

## Scope

- **Supported languages**: Go (`.go`) and Python (`.py`) only.
- **Supported kinds** (strict): `function`, `method`, `struct`, `interface`, `class`.
  - Any other `kind` MUST return an error listing supported kinds.

## Common Concepts

### QName

`qname` is the fully-qualified, unique identifier for a symbol in the code graph.
Relationship queries are performed using `qname`.

### Name patterns

`name` uses glob-style matching (`*` wildcard). Examples: `Plan`, `Handle*`, `*Validator*`.

### File filter

`file` filters by file path using suffix matching (e.g. `internal/brain/planner.go` or `planner.go`).

## Parameters (JSON)

All operations use a single parameter object:

```json
{
  "operation": "search|resolve|file_symbols|callers|callees|implementations|usages|trace",

  "name": "...",        // optional: symbol name/glob pattern
  "kind": "...",        // optional: strict enum (function/method/struct/interface/class)
  "file": "...",        // optional: file filter; required for file_symbols

  "qname": "...",       // optional: fully qualified name (preferred when known)
  "depth": 2,            // optional: callers/callees depth (default 1; clamped)

  "from_name": "...",   // trace only: start symbol name (alternative to from_qname)
  "from_qname": "...",  // trace only: start symbol qname
  "from_kind": "...",   // trace only: optional kind (function/method)
  "from_file": "...",   // trace only: optional file filter for resolving from_name

  "to_name": "...",     // trace only: target symbol name (alternative to to_qname)
  "to_qname": "...",    // trace only: target symbol qname
  "to_kind": "...",     // trace only: optional kind (function/method)
  "to_file": "...",     // trace only: optional file filter for resolving to_name

  "max_depth": 6         // trace only: max call depth (default 4; clamped)
}
```

## Operations

### 1) `resolve`

Resolve a symbol to a single `qname`.

- Requires: `name`
- Optional: `kind`, `file`
- Behavior:
  - If exactly one match exists, returns that symbol with `file:line`.
  - If ambiguous, returns an error listing a small set of candidates with `file:line`.

Example:

```json
{"operation":"resolve","name":"ActionExecutor","kind":"interface"}
```

### 2) `search`

List matching symbols.

- Requires: `name`
- Optional: `kind`, `file`
- Output is one-line-per-result with `file:line`, `kind`, `qname`, and a truncated signature.

Example:

```json
{"operation":"search","name":"Plan","kind":"method"}
```

### 3) `file_symbols`

List symbols defined in a file.

- Requires: `file`
- Optional: `kind`

Example:

```json
{"operation":"file_symbols","file":"internal/brain/planner.go"}
```

### 4) `callers` / `callees`

Query call graph relationships.

- Preferred input: `qname`
- Convenience input (recommended for models): provide `name` (+ optional `kind`/`file`) and the tool will `resolve` internally.
- Optional: `depth`

Examples:

```json
{"operation":"callers","name":"Plan","kind":"method","depth":2}
```

```json
{"operation":"callees","qname":"basegraph.co/relay/internal/brain.Planner.Plan","depth":3}
```

### 5) `implementations`

Find types that implement an interface/class.

- Preferred: `qname`
- Convenience: `name` (+ optional `kind`/`file`) with internal resolve.

Example:

```json
{"operation":"implementations","name":"IssueStore","kind":"interface"}
```

### 6) `usages`

Find functions/methods that use a type as a parameter or return value.

- Preferred: `qname`
- Convenience: `name` (+ optional `kind`/`file`) with internal resolve.

Example:

```json
{"operation":"usages","name":"Issue","kind":"struct"}
```

### 7) `trace`

Find a call-flow path from one function/method to another.

- Requires either:
  - `from_qname` + `to_qname`, OR
  - `from_name` + `to_name` (each resolved internally; use `from_file`/`to_file` to disambiguate)
- Optional: `max_depth` (clamped)

Example:

```json
{"operation":"trace","from_name":"HandleWebhook","to_name":"Plan","to_kind":"method","max_depth":6}
```

## Output Requirements

- Always include `file:line` when a location is known.
- Use a compact, grep-like format:
  - `path/to/file.go:123\tkind\tqname\tsignature`
- Errors must be explicit and self-correcting.
  - Invalid kind error must list supported kinds.
  - Ambiguous resolution must list candidates with `file:line`.

## Effectiveness Metrics (ExploreAgent)

The ExploreAgent should log structured metrics to evaluate whether the model uses codegraph effectively:

- Per-operation call counts (search/resolve/callers/callees/implementations/usages/trace)
- Count of relationship ops invoked with `name` (auto-resolve path) vs with `qname`
- Invalid kind attempts
- Ambiguous resolves
- Trace successes vs not found
- Overall tool mix: codegraph vs grep/read (already tracked)
