# Explore Agent Tool Simplification

## Context

The explore agent was hitting 100k+ token limits. Analysis of logs showed:
- `symbols` operation doubled token usage (agent calls both symbols AND read on same files)
- Custom tools add complexity without reducing tokens
- Claude Code works great with just bash - model already knows grep, find, head, etc.

**Strategy:** "Write better tools and get out of the model's way"
1. Add bash tool (universal, model knows it) ✓ DONE
2. Remove symbols operation (data shows it increases tokens)
3. Add result limits to graph operations
4. Update system prompt

## Status

| Task | Status | Notes |
|------|--------|-------|
| Add bash tool | ✓ Complete | See implementation below |
| Remove symbols operation | ✓ Complete | Removed symbols from graph tool |
| Add graph result limits (30) | ✓ Complete | Added LIMIT 30 to callers/callees/search |
| Update system prompt | ✓ Complete | Simplified prompt, added bash mention |
| Test changes | ✓ Complete | Build successful, lint clean |

---

## Task 1: Bash Tool (COMPLETE)

Added a sandboxed bash tool to the explore agent that allows read-only command execution.

## Implementation

### Files Modified

**`internal/brain/explore_tools.go`**

1. **New constants** (lines 30-32):
   ```go
   maxGraphResults   = 30    // Limit graph search/relationship results
   bashTimeout       = 10    // Bash command timeout in seconds
   maxBashOutput     = 30000 // Max bash output bytes (30KB)
   ```

2. **New struct** (lines 74-77):
   ```go
   type BashParams struct {
       Command string `json:"command" jsonschema:"required,description=Bash command to execute (read-only commands only)"`
   }
   ```

3. **Tool definition** (lines 230-235): Minimal description - "Execute bash commands (read-only). Write operations blocked. Output limited to 30KB."

4. **Execute switch** (line 278): Added `case "bash"` routing

5. **Permission system** (lines 711-732):
   - `bashAllowedPrefixes`: cat, head, tail, grep, find, ls, wc, git read-only commands
   - `bashBlockedPrefixes`: rm, mv, cp, mkdir, git write commands, redirects

6. **Core functions**:
   - `executeBash()` (lines 734-791): Main execution with timeout, error handling
   - `isBashCommandAllowed()` (lines 793-823): Prefix-based permission checking
   - `validateBashPaths()` (lines 825-848): Ensures paths stay within repo root
   - `truncateBashOutput()` (lines 850-863): Output limiting at 30KB

## Security Model

### Allowed Commands (prefix matching)
| Category | Commands |
|----------|----------|
| File reading | `cat `, `head `, `tail `, `less ` |
| Search | `grep `, `rg `, `find `, `fd `, `ag ` |
| File info | `wc `, `ls `, `file `, `stat ` |
| Git read-only | `git log`, `git show`, `git diff`, `git blame`, `git status`, `git branch`, `git tag`, `git remote` |

### Blocked Commands
| Category | Commands |
|----------|----------|
| File writes | `rm `, `mv `, `cp `, `mkdir `, `touch `, `chmod `, `chown ` |
| Git writes | `git push`, `git commit`, `git checkout`, `git reset`, `git rebase`, `git merge`, `git pull`, `git stash`, `git clean` |
| Redirects | `echo `, `printf `, `>`, `>>` |

### Directory Containment
- Absolute paths in commands are validated against repo root
- Uses `filepath.Abs` and prefix matching
- Blocks access to paths outside repository

### Limits
- **Timeout**: 10 seconds
- **Output**: 30KB max, truncated at newline boundary
- **Error handling**: Returns exit code 1 for grep/rg as "no matches"

---

## Task 2: Remove Symbols Operation (PENDING)

**Why:** Log analysis showed agent calls both `symbols(file)` AND `read(file)` on same files, doubling token usage.

**Files to modify:**

1. `internal/brain/explore_tools.go`:
   - Line 53: Remove `enum=symbols` from GraphParams.Operation
   - Line 56-57: Remove File field description mentioning symbols
   - Lines 166-169: Remove symbols from graph tool description
   - Lines 527-529: Remove `case "symbols"` from executeGraph switch
   - Lines 537-578: Delete `executeGraphSymbols` function

2. `internal/brain/explore_agent.go`:
   - Lines 530-531: Remove "symbols(file)" from system prompt
   - Lines 537-538: Remove symbols guidance

---

## Task 3: Add Graph Result Limits (PENDING)

**Why:** Graph operations currently return unbounded results. `callers` can return 200+ results.

**Files to modify:**

1. `common/arangodb/client.go`:
   - `SearchSymbols`: Add `LIMIT 30` to query (currently returns up to 50)
   - `GetCallers`: Add `LIMIT 30`
   - `GetCallees`: Add `LIMIT 30`
   - Return total count so output can show "Found N total, showing first 30"

2. `internal/brain/explore_tools.go`:
   - Use new `maxGraphResults = 30` constant (already added)
   - Update output messages to show truncation

---

## Task 4: Update System Prompt (PENDING)

**File:** `internal/brain/explore_agent.go` lines 522-562

**Changes:**
1. Remove all mentions of `symbols` operation
2. Remove "symbols(file)" from workflow guidance
3. Add brief bash mention (keep it minimal - model knows bash)
4. Keep graph for semantic operations (callers, callees, search)

**Suggested simplified prompt structure:**
```
## Tools
**bash** - Run read-only commands (grep, find, head, git log, etc.)
**graph** - Semantic queries: search(name), callers(qname), callees(qname), methods(qname)
**read** - File contents with line numbers
**tree** - Directory structure
```

---

## Task 5: Test (PENDING)

```bash
# Build
make build

# Run worker against testdata
make run-worker-testdata

# Trigger explore query and check logs
# Verify:
# 1. bash tool appears in tool calls
# 2. symbols operation is gone
# 3. graph results are limited to 30
# 4. No blocked commands succeed
```

## Design Decisions

1. **Simple prefix matching over AST parsing**: Explore agent is read-only with deny-by-default. AST parsing can be added later if needed.

2. **Allow list approach**: Explicit allowlist safer than blocklist for a sandboxed environment.

3. **30KB output limit**: Matches opencode's implementation, prevents context explosion.

4. **10 second timeout**: Explore doesn't need long-running commands. Forces use of `| head` for large results.

5. **Directory validation on absolute paths only**: Relative paths resolve within repo root by default via `cmd.Dir`.

## Testing Commands

```bash
# Build
go build ./internal/brain/...

# Test allowed command
# bash(command="head -50 internal/brain/explore_tools.go")

# Test blocked command
# bash(command="rm -rf /")  → Should return "Command blocked: write operation 'rm ' not allowed"

# Test path outside repo
# bash(command="cat /etc/passwd")  → Should return "Command blocked: path outside repository"
```
