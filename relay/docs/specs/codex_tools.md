# Codex Explore/Context Tools (Local): Definitions, Prompt Wiring, and API

This spec documents only Codex’s *local* context-gathering tools: the ones used to explore a repo and retrieve code context for reasoning. It omits execution, patching, planning, UI-only tools, and all MCP-related behavior.

Sources of truth are in `read-only/codex/`:

- Tool schemas: `codex-rs/core/src/tools/spec.rs`
- Tool handlers (behavior/output): `codex-rs/core/src/tools/handlers/grep_files.rs`, `codex-rs/core/src/tools/handlers/read_file.rs`, `codex-rs/core/src/tools/handlers/list_dir.rs`
- Tool inclusion gating: `codex-rs/core/src/tools/spec.rs` (`build_specs`, `experimental_supported_tools`)
- Tool-call item shapes + call-id pairing: `codex-rs/protocol/src/models.rs`
- Prompt/context injection: `codex-rs/core/src/codex.rs`, `codex-rs/core/src/environment_context.rs`, `codex-rs/core/src/user_instructions.rs`, `codex-rs/core/src/models_manager/model_family.rs`

---

## 1) Tool list (what Codex uses for “explore/read/search”)

### 1.1 Local repo exploration tools (conditional)

These exist in the codebase but are only exposed to the model when enabled for the current model family via `experimental_supported_tools`:

- `grep_files` — regex search that returns matching file paths (not matches)
- `read_file` — read a file with line numbers (slice or indentation-aware block)
- `list_dir` — list directory entries (depth-limited)

### 1.2 No standalone `glob` tool

Codex does not ship a `glob_files`/`find_files` tool. The only “glob” support is `grep_files.include`, which is passed to ripgrep as `rg --glob <glob>`.

---

## 2) Tool definitions (schemas)

All of these tools are exposed to the model as Responses API “function tools”:
```json
{ "type": "function", "name": "...", "description": "...", "strict": false, "parameters": { ... } }
```

### 2.1 `grep_files`

Definition: `create_grep_files_tool()` (`read-only/codex/codex-rs/core/src/tools/spec.rs`).

Parameters:
```json
{
  "type": "object",
  "properties": {
    "pattern": { "type": "string", "description": "Regular expression pattern to search for." },
    "include": { "type": "string", "description": "Optional glob that limits which files are searched (e.g. \"*.rs\" or \"*.{ts,tsx}\")." },
    "path": { "type": "string", "description": "Directory or file path to search. Defaults to the session's working directory." },
    "limit": { "type": "number", "description": "Maximum number of file paths to return (defaults to 100)." }
  },
  "required": ["pattern"],
  "additionalProperties": false
}
```

### 2.2 `read_file`

Definition: `create_read_file_tool()` (`read-only/codex/codex-rs/core/src/tools/spec.rs`).

Parameters:
```json
{
  "type": "object",
  "properties": {
    "file_path": { "type": "string", "description": "Absolute path to the file" },
    "offset": { "type": "number", "description": "The line number to start reading from. Must be 1 or greater." },
    "limit": { "type": "number", "description": "The maximum number of lines to return." },
    "mode": { "type": "string", "description": "Optional mode selector: \"slice\" for simple ranges (default) or \"indentation\" to expand around an anchor line." },
    "indentation": {
      "type": "object",
      "properties": {
        "anchor_line": { "type": "number", "description": "Anchor line to center the indentation lookup on (defaults to offset)." },
        "max_levels": { "type": "number", "description": "How many parent indentation levels (smaller indents) to include." },
        "include_siblings": { "type": "boolean", "description": "When true, include additional blocks that share the anchor indentation." },
        "include_header": { "type": "boolean", "description": "Include doc comments or attributes directly above the selected block." },
        "max_lines": { "type": "number", "description": "Hard cap on the number of lines returned when using indentation mode." }
      },
      "additionalProperties": false
    }
  },
  "required": ["file_path"],
  "additionalProperties": false
}
```

### 2.3 `list_dir`

Definition: `create_list_dir_tool()` (`read-only/codex/codex-rs/core/src/tools/spec.rs`).

Parameters:
```json
{
  "type": "object",
  "properties": {
    "dir_path": { "type": "string", "description": "Absolute path to the directory to list." },
    "offset": { "type": "number", "description": "The entry number to start listing from. Must be 1 or greater." },
    "limit": { "type": "number", "description": "The maximum number of entries to return." },
    "depth": { "type": "number", "description": "The maximum directory depth to traverse. Must be 1 or greater." }
  },
  "required": ["dir_path"],
  "additionalProperties": false
}
```

---

## 3) Runtime behavior (what the tools actually do + output shapes)

### 3.1 `grep_files` behavior

Handler: `read-only/codex/codex-rs/core/src/tools/handlers/grep_files.rs`.

- Uses ripgrep (`rg`) with `--files-with-matches` and `--sortr=modified`.
- Supports an optional glob filter: `--glob <include>`.
- `limit` defaults to 100, capped at 2000.
- Timeout is 30 seconds.

Output:

- Success: newline-separated list of file paths.
- No matches: the literal string `No matches found.` (tool output `success=false`).

### 3.2 `read_file` behavior

Handler: `read-only/codex/codex-rs/core/src/tools/handlers/read_file.rs`.

Important constraint: **requires absolute paths** (rejects non-absolute `file_path`), even though the schema only describes it as “absolute” in prose.

Modes:

- `slice` (default): return `limit` lines from `offset`.
- `indentation`: select an indentation-aware block around `anchor_line` (defaults to `offset`) with optional sibling/header inclusion.

Output:

- Lines formatted as: `L<line_number>: <line content>`

### 3.3 `list_dir` behavior

Handler: `read-only/codex/codex-rs/core/src/tools/handlers/list_dir.rs`.

Important constraint: **requires absolute paths** (rejects non-absolute `dir_path`).

Defaults:

- `offset=1`, `limit=25`, `depth=2`

Output:

- First line: `Absolute path: <dir_path>`
- Then an indented listing with suffix markers:
  - directory ends with `/`
  - symlink ends with `@`
  - unknown ends with `?`

---

## 4) Prompt wiring (how the model learns cwd + uses these tools)

### 4.1 Base “system prompt”

Codex sends a plain string `instructions` in the `/responses` request, chosen from the model family’s `base_instructions` (e.g. `read-only/codex/codex-rs/core/prompt.md`, `read-only/codex/codex-rs/core/gpt_5_2_prompt.md`).

Codex does *not* hardcode these explore tools into the system prompt. The model learns what’s available from the `tools=[...]` list in the API request.

### 4.2 Injected environment context (critical for absolute paths)

Codex injects an `<environment_context>...</environment_context>` message into the `input` transcript containing (at minimum) the current `cwd` (`read-only/codex/codex-rs/core/src/environment_context.rs`).

Because `read_file`/`list_dir` require absolute paths, the intended model flow is:

1. Read cwd from environment context
2. Construct absolute paths under that cwd
3. Call `list_dir` / `read_file`

### 4.3 User instructions (AGENTS.md)

Codex also injects repo instructions (AGENTS.md) as a user message prefixed with `# AGENTS.md instructions for ...` (`read-only/codex/codex-rs/core/src/user_instructions.rs`). This often guides what to inspect.

---

## 5) API (tool calling over Responses)

### 5.1 Where tool definitions go

In each Responses API request, Codex passes:

- `tools`: JSON array of tool definitions (including these explore tools when enabled)
- `parallel_tool_calls`: boolean (model capability + feature)
- `input`: transcript containing context, messages, and prior tool outputs

### 5.2 Tool call and tool output item shapes

These are the minimum shapes you need to mirror for context tools (from `read-only/codex/codex-rs/protocol/src/models.rs`):

Tool call (from model):
```json
{ "type": "function_call", "name": "read_file", "arguments": "{...json string...}", "call_id": "..." }
```

Tool output (to model, in the next request’s `input`):
```json
{ "type": "function_call_output", "call_id": "...", "output": "...." }
```

The output is a plain string for these context tools.

### 5.3 Parallelism

Codex marks all the context tools above as parallel-safe in its internal scheduler (they can run concurrently if multiple calls arrive in the same turn).

---

## 6) What Codex does *not* include as explore tools

- No dedicated `glob_files` / “find by glob” tool.
- No dedicated “read project tree as JSON” tool.
- No “search in file with line matches” tool; `grep_files` returns file paths only.
