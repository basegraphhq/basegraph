package brain

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"basegraph.app/relay/common/arangodb"
	"basegraph.app/relay/common/llm"
)

const (
	bashTimeout      = 10    // Bash command timeout in seconds
	maxBashOutput    = 10000 // Max bash output bytes (10KB)
	maxGlobResults   = 100   // Max files returned by glob
	maxGrepMatches   = 50    // Max matches returned by grep
	maxReadLines     = 500   // Max lines returned by read
	defaultReadLines = 200   // Default lines if not specified
	maxLineLength    = 2000  // Truncate lines longer than this

	// Codegraph constants
	maxSearchResults  = 10 // Max symbols returned by search
	defaultGraphDepth = 1  // Default traversal depth
	maxGraphDepth     = 3  // Max traversal depth for callers/callees
)

// Tool parameter structs - Claude Code style

// GlobParams for file pattern matching.
type GlobParams struct {
	Pattern string `json:"pattern" jsonschema:"required,description=Glob pattern to match files (e.g. '**/*.go', 'internal/**/*.ts')"`
	Path    string `json:"path,omitempty" jsonschema:"description=Directory to search in. Defaults to repo root."`
}

// GrepParams for content search.
type GrepParams struct {
	Pattern    string `json:"pattern" jsonschema:"required,description=Regex pattern to search for in file contents"`
	Path       string `json:"path,omitempty" jsonschema:"description=File or directory to search. Defaults to repo root."`
	Glob       string `json:"glob,omitempty" jsonschema:"description=Filter files by glob pattern (e.g. '*.go', '*.ts')"`
	IgnoreCase bool   `json:"ignore_case,omitempty" jsonschema:"description=Case insensitive search"`
	Context    int    `json:"context,omitempty" jsonschema:"description=Lines of context around matches (default 0)"`
}

// ReadParams for reading files.
type ReadParams struct {
	FilePath string `json:"file_path" jsonschema:"required,description=Path to the file to read (relative to repo root)"`
	Offset   int    `json:"offset,omitempty" jsonschema:"description=Line number to start reading from (1-indexed)"`
	Limit    int    `json:"limit,omitempty" jsonschema:"description=Number of lines to read (default 200, max 500)"`
}

// BashParams for shell commands.
type BashParams struct {
	Command string `json:"command" jsonschema:"required,description=Bash command to execute (read-only: git log/diff/blame, ls, find)"`
}

// CodegraphParams for querying code relationships.
type CodegraphParams struct {
	Operation string `json:"operation" jsonschema:"required,enum=search,enum=resolve,enum=file_symbols,enum=callers,enum=callees,enum=implementations,enum=usages,enum=trace,description=Codegraph operation"`

	// Symbol selector (used by search/resolve, and as a convenience for relationship ops when qname is unknown)
	Name string `json:"name,omitempty" jsonschema:"description=Symbol name or glob pattern (e.g. 'Plan', 'Handler*')."`
	Kind string `json:"kind,omitempty" jsonschema:"enum=function,enum=method,enum=struct,enum=interface,enum=class,description=Optional kind filter. Supported kinds: function, method, struct, interface, class."`
	File string `json:"file,omitempty" jsonschema:"description=Optional file filter (suffix match, e.g. 'planner.go' or 'internal/brain/planner.go'). Required for file_symbols."`

	// Relationship operations
	QName string `json:"qname,omitempty" jsonschema:"description=Fully qualified symbol name (qname). If set, used directly."`
	Depth int    `json:"depth,omitempty" jsonschema:"description=Traversal depth for callers/callees (1-3, default 1)"`

	// Trace operation (call path)
	FromName  string `json:"from_name,omitempty" jsonschema:"description=Trace start symbol name (alternative to from_qname)."`
	FromQName string `json:"from_qname,omitempty" jsonschema:"description=Trace start symbol qname."`
	FromKind  string `json:"from_kind,omitempty" jsonschema:"enum=function,enum=method,description=Optional kind for resolving from_name (function/method)."`
	FromFile  string `json:"from_file,omitempty" jsonschema:"description=Optional file filter for resolving from_name."`

	ToName  string `json:"to_name,omitempty" jsonschema:"description=Trace target symbol name (alternative to to_qname)."`
	ToQName string `json:"to_qname,omitempty" jsonschema:"description=Trace target symbol qname."`
	ToKind  string `json:"to_kind,omitempty" jsonschema:"enum=function,enum=method,description=Optional kind for resolving to_name (function/method)."`
	ToFile  string `json:"to_file,omitempty" jsonschema:"description=Optional file filter for resolving to_name."`

	MaxDepth int `json:"max_depth,omitempty" jsonschema:"description=Max call depth for trace (1-10, default 4)"`
}

// ExploreTools provides Claude Code-style tools for the ExploreAgent.
type ExploreTools struct {
	repoRoot    string
	arango      arangodb.Client // nil = codegraph unavailable
	definitions []llm.Tool
}

// NewExploreTools creates tools for code exploration (Claude Code style).
// arango can be nil - codegraph tool will gracefully degrade.
func NewExploreTools(repoRoot string, arango arangodb.Client) *ExploreTools {
	t := &ExploreTools{
		repoRoot: repoRoot,
		arango:   arango,
	}

	t.definitions = []llm.Tool{
		{
			Name: "glob",
			Description: `Find files by pattern. Returns file paths sorted by modification time (newest first).

Examples:
  glob(pattern="**/*.go")                    # All Go files
  glob(pattern="internal/**/*.go")           # Go files in internal/
  glob(pattern="*_test.go", path="pkg/")     # Test files in pkg/

Use this to discover files before reading them.`,
			Parameters: llm.GenerateSchemaFrom(GlobParams{}),
		},
		{
			Name: "grep",
			Description: `Search file contents with regex. Returns matching lines with file:line references.

Examples:
  grep(pattern="func.*Plan")                      # Find Plan functions
  grep(pattern="TODO|FIXME", glob="*.go")         # TODOs in Go files
  grep(pattern="error", path="internal/", context=2)  # Errors with context

Use this to find where patterns occur in code.`,
			Parameters: llm.GenerateSchemaFrom(GrepParams{}),
		},
		{
			Name: "read",
			Description: `Read a file with optional line range. Returns numbered lines.

Examples:
  read(file_path="internal/brain/planner.go")              # First 200 lines
  read(file_path="pkg/api/handler.go", offset=50, limit=100)  # Lines 50-149

Use this after glob/grep to examine specific code.`,
			Parameters: llm.GenerateSchemaFrom(ReadParams{}),
		},
		{
			Name: "bash",
			Description: `Execute read-only bash commands. Use for git history and directory listing.

Allowed:
  git log --oneline -10 file.go    # Recent commits
  git diff HEAD~5 file.go          # Recent changes
  git blame -L 50,70 file.go       # Line history
  ls -la internal/                 # Directory contents

NOT allowed: rm, mv, cp, echo, write operations.`,
			Parameters: llm.GenerateSchemaFrom(BashParams{}),
		},
		{
			Name: "codegraph",
			Description: `Query code structure graph for relationships and call flow. SUPPORTED: Go (.go) and Python (.py) only.

SUPPORTED KINDS (strict): function, method, struct, interface, class.
If you pass any other kind, you will get an error listing supported kinds.

KEY IDEA: Relationship operations accept either:
- qname (preferred when known), OR
- name (+ optional kind/file), and the tool will resolve to qname internally.

OPERATIONS:

- resolve: Resolve name -> single qname (or show candidates)
  codegraph(operation="resolve", name="ActionExecutor", kind="interface")

- search: List matching symbols by name/glob
  codegraph(operation="search", name="Plan", kind="method")

- file_symbols: List symbols defined in a file
  codegraph(operation="file_symbols", file="internal/brain/planner.go")

- callers: Find callers of a function/method
  codegraph(operation="callers", name="Plan", kind="method", depth=2)

- callees: Find callees from a function/method
  codegraph(operation="callees", qname="basegraph.app/relay/internal/brain.Planner.Plan", depth=2)

- implementations: Find types that implement an interface/class
  codegraph(operation="implementations", name="IssueStore", kind="interface")

- usages: Find functions/methods that use a type (param/return)
  codegraph(operation="usages", name="Issue", kind="struct")

- trace: Find a DIRECT call path between two functions/methods.
  Only works within the same process - will NOT find paths that cross async
  boundaries (queues, IPC, HTTP). For those, use callers/callees + grep.
  codegraph(operation="trace", from_name="HandleWebhook", to_name="Plan", to_kind="method", max_depth=6)

For text search or unsupported languages, use grep/read instead.`,
			Parameters: llm.GenerateSchemaFrom(CodegraphParams{}),
		},
	}

	return t
}

// Definitions returns tool definitions for the LLM.
func (t *ExploreTools) Definitions() []llm.Tool {
	return t.definitions
}

// Execute runs a tool by name and returns output.
func (t *ExploreTools) Execute(ctx context.Context, name, arguments string) (string, error) {
	switch name {
	case "glob":
		return t.executeGlob(ctx, arguments)
	case "grep":
		return t.executeGrep(ctx, arguments)
	case "read":
		return t.executeRead(ctx, arguments)
	case "bash":
		return t.executeBash(ctx, arguments)
	case "codegraph":
		return t.executeCodegraph(ctx, arguments)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

// executeGlob finds files matching a glob pattern using fd (fast find).
func (t *ExploreTools) executeGlob(ctx context.Context, arguments string) (string, error) {
	params, err := llm.ParseToolArguments[GlobParams](arguments)
	if err != nil {
		return "", fmt.Errorf("parse glob params: %w", err)
	}

	if params.Pattern == "" {
		return "Error: pattern is required", nil
	}

	searchPath := t.repoRoot
	if params.Path != "" {
		searchPath = filepath.Join(t.repoRoot, params.Path)
	}

	// Validate path is within repo
	if !pathWithinRoot(t.repoRoot, searchPath) {
		return "Error: path outside repository", nil
	}

	// Use fd for fast glob matching (falls back to find if fd not available)
	var matches []fileMatch

	// Try fd first (much faster)
	args := []string{
		"--type", "f",
		"--hidden",
		"--no-ignore",
		"--exclude", ".git",
		"--exclude", "node_modules",
		"--exclude", "vendor",
		"--exclude", "__pycache__",
		"--glob", params.Pattern,
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "fd", args...)
	cmd.Dir = searchPath
	output, err := cmd.Output()
	if err != nil {
		// Fall back to find command
		findArgs := []string{
			searchPath,
			"-type", "f",
			"-name", params.Pattern,
			"-not", "-path", "*/.git/*",
			"-not", "-path", "*/node_modules/*",
			"-not", "-path", "*/vendor/*",
		}
		cmd = exec.CommandContext(timeoutCtx, "find", findArgs...)
		output, err = cmd.Output()
		if err != nil {
			if timeoutCtx.Err() == context.DeadlineExceeded {
				return "Search timed out. Use more specific pattern.", nil
			}
			return fmt.Sprintf("Error: glob failed: %s", err), nil
		}
	}

	// Parse output and get file info
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		fullPath := line
		if !filepath.IsAbs(fullPath) {
			fullPath = filepath.Join(searchPath, line)
		}

		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}

		relPath, _ := filepath.Rel(t.repoRoot, fullPath)
		if shouldSkipFile(relPath) {
			continue
		}

		matches = append(matches, fileMatch{
			path:    relPath,
			modTime: info.ModTime(),
		})

		if len(matches) >= maxGlobResults*2 {
			break
		}
	}

	if len(matches) == 0 {
		return fmt.Sprintf("No files match pattern: %s", params.Pattern), nil
	}

	// Sort by modification time (newest first)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].modTime.After(matches[j].modTime)
	})

	// Truncate to max results
	truncated := len(matches) > maxGlobResults
	if truncated {
		matches = matches[:maxGlobResults]
	}

	var result strings.Builder
	for _, m := range matches {
		result.WriteString(m.path)
		result.WriteString("\n")
	}

	if truncated {
		result.WriteString(fmt.Sprintf("\n[Showing %d of %d+ matches. Refine pattern for more specific results.]", maxGlobResults, len(matches)))
	}

	return withTokenEstimate(result.String()), nil
}

type fileMatch struct {
	path    string
	modTime time.Time
}

// shouldSkipFile returns true for files that should be excluded from glob results.
func shouldSkipFile(path string) bool {
	// Skip hidden files/dirs
	parts := strings.Split(path, string(filepath.Separator))
	for _, p := range parts {
		if strings.HasPrefix(p, ".") && p != "." && p != ".." {
			return true
		}
	}

	// Skip common noise directories
	skipDirs := []string{"node_modules", "vendor", "__pycache__", ".git", "dist", "build"}
	for _, skip := range skipDirs {
		if strings.Contains(path, string(filepath.Separator)+skip+string(filepath.Separator)) {
			return true
		}
	}

	return false
}

// executeGrep searches file contents with ripgrep.
func (t *ExploreTools) executeGrep(ctx context.Context, arguments string) (string, error) {
	params, err := llm.ParseToolArguments[GrepParams](arguments)
	if err != nil {
		return "", fmt.Errorf("parse grep params: %w", err)
	}

	if params.Pattern == "" {
		return "Error: pattern is required", nil
	}

	// Build ripgrep command
	args := []string{
		"-n",           // Line numbers
		"--no-heading", // File:line format
		"--color=never",
	}

	if params.IgnoreCase {
		args = append(args, "-i")
	}

	if params.Context > 0 {
		args = append(args, fmt.Sprintf("-C%d", params.Context))
	}

	if params.Glob != "" {
		args = append(args, "-g", params.Glob)
	}

	// Add pattern
	args = append(args, params.Pattern)

	// Add path
	searchPath := t.repoRoot
	if params.Path != "" {
		searchPath = filepath.Join(t.repoRoot, params.Path)
	}
	if !pathWithinRoot(t.repoRoot, searchPath) {
		return "Error: path outside repository", nil
	}
	args = append(args, searchPath)

	// Execute ripgrep with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(bashTimeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "rg", args...)
	output, err := cmd.Output()

	if timeoutCtx.Err() == context.DeadlineExceeded {
		return "Search timed out. Use more specific pattern or path.", nil
	}

	// ripgrep returns exit code 1 for no matches
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return fmt.Sprintf("No matches for pattern: %s", params.Pattern), nil
		}
		// Other errors might still have useful output
		if len(output) == 0 {
			return fmt.Sprintf("Search error: %s", err), nil
		}
	}

	// Truncate results
	lines := strings.Split(string(output), "\n")
	truncated := len(lines) > maxGrepMatches
	if truncated {
		lines = lines[:maxGrepMatches]
	}

	// Make paths relative
	var result strings.Builder
	for _, line := range lines {
		if line == "" {
			continue
		}
		// Convert absolute paths to relative
		if strings.HasPrefix(line, t.repoRoot) {
			line = strings.TrimPrefix(line, t.repoRoot+"/")
		}
		result.WriteString(line)
		result.WriteString("\n")
	}

	if truncated {
		result.WriteString(fmt.Sprintf("\n[Showing %d matches. Add glob filter or refine pattern.]", maxGrepMatches))
	}

	return withTokenEstimate(result.String()), nil
}

// executeRead reads a file with optional line range.
func (t *ExploreTools) executeRead(ctx context.Context, arguments string) (string, error) {
	params, err := llm.ParseToolArguments[ReadParams](arguments)
	if err != nil {
		return "", fmt.Errorf("parse read params: %w", err)
	}

	if params.FilePath == "" {
		return "Error: file_path is required", nil
	}

	// Resolve and validate path
	fullPath := filepath.Join(t.repoRoot, params.FilePath)
	if !pathWithinRoot(t.repoRoot, fullPath) {
		return "Error: path outside repository", nil
	}

	// Open file
	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("Error: file not found: %s", params.FilePath), nil
		}
		return fmt.Sprintf("Error: cannot read file: %s", err), nil
	}
	defer file.Close()

	// Set defaults
	offset := params.Offset
	if offset < 1 {
		offset = 1
	}
	limit := params.Limit
	if limit < 1 {
		limit = defaultReadLines
	}
	if limit > maxReadLines {
		limit = maxReadLines
	}

	// Read lines
	scanner := bufio.NewScanner(file)
	var result strings.Builder
	lineNum := 0
	linesRead := 0

	for scanner.Scan() {
		lineNum++

		// Skip until offset
		if lineNum < offset {
			continue
		}

		// Stop at limit
		if linesRead >= limit {
			break
		}

		line := scanner.Text()
		// Truncate long lines
		if len(line) > maxLineLength {
			line = line[:maxLineLength] + "..."
		}

		// Format with line number (cat -n style)
		result.WriteString(fmt.Sprintf("%6d\t%s\n", lineNum, line))
		linesRead++
	}

	if err := scanner.Err(); err != nil {
		return fmt.Sprintf("Error reading file: %s", err), nil
	}

	if linesRead == 0 {
		if lineNum == 0 {
			return "File is empty", nil
		}
		return fmt.Sprintf("No lines at offset %d (file has %d lines)", offset, lineNum), nil
	}

	// Add info about what was read
	info := fmt.Sprintf("\n[Read lines %d-%d of %s", offset, offset+linesRead-1, params.FilePath)
	if lineNum > offset+linesRead-1 {
		info += fmt.Sprintf(". File continues to line %d.]", lineNum)
	} else {
		info += ". End of file.]"
	}
	result.WriteString(info)

	return withTokenEstimate(result.String()), nil
}

// bashAllowedPrefixes defines read-only commands that are allowed.
var bashAllowedPrefixes = []string{
	// Git read-only
	"git log", "git show", "git diff", "git blame", "git status",
	"git branch", "git tag", "git remote", "git grep", "git rev-parse",
	// File info
	"ls ", "ls", "wc ", "file ", "stat ", "tree ",
	// Find (useful for complex searches)
	"find ",
	// File reading
	"cat ", "head ", "tail ", "grep ", "rg ",
}

// bashBlockedPrefixes defines write operations that are always blocked.
var bashBlockedPrefixes = []string{
	"rm ", "mv ", "cp ", "mkdir ", "touch ", "chmod ", "chown ",
	"git push", "git commit", "git checkout", "git reset", "git rebase",
	"git merge", "git pull", "git stash", "git clean", "git add",
	"echo ", "printf ", "sed ", "awk ",
	">", ">>",
}

func (t *ExploreTools) executeBash(ctx context.Context, arguments string) (string, error) {
	params, err := llm.ParseToolArguments[BashParams](arguments)
	if err != nil {
		return "", fmt.Errorf("parse bash params: %w", err)
	}

	command := strings.TrimSpace(params.Command)
	if command == "" {
		return "Error: command is required", nil
	}

	// Check if command is allowed
	allowed, reason := t.isBashCommandAllowed(command)
	if !allowed {
		slog.DebugContext(ctx, "bash command blocked",
			"command", command,
			"reason", reason)
		return fmt.Sprintf("Command blocked: %s\n\nAllowed: git log/show/diff/blame/status, ls, tree, find, cat, head, tail, grep, rg", reason), nil
	}

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(bashTimeout)*time.Second)
	defer cancel()

	// Execute command
	cmd := exec.CommandContext(timeoutCtx, "bash", "-c", command)
	cmd.Dir = t.repoRoot

	output, err := cmd.CombinedOutput()

	// Handle timeout
	if timeoutCtx.Err() == context.DeadlineExceeded {
		return fmt.Sprintf("Command timed out after %d seconds.", bashTimeout), nil
	}

	// Handle other errors (but still return output if available)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Non-zero exit code - might just be "no matches" for find
			if exitErr.ExitCode() == 1 && strings.HasPrefix(command, "find") {
				return "No matches found", nil
			}
		}
		if len(output) > 0 {
			return fmt.Sprintf("Command failed: %s\nOutput:\n%s", err, t.truncateOutput(output)), nil
		}
		return fmt.Sprintf("Command failed: %s", err), nil
	}

	result := t.truncateOutput(output)

	slog.DebugContext(ctx, "bash executed",
		"command", command,
		"output_len", len(output))

	return withTokenEstimate(result), nil
}

// isBashCommandAllowed checks if a command is allowed.
func (t *ExploreTools) isBashCommandAllowed(command string) (bool, string) {
	cmd := strings.TrimSpace(command)

	// Check blocked prefixes first
	for _, prefix := range bashBlockedPrefixes {
		if strings.HasPrefix(cmd, prefix) {
			return false, fmt.Sprintf("'%s' not allowed - use dedicated tools", strings.TrimSpace(prefix))
		}
	}

	// Check for blocked patterns
	if strings.Contains(cmd, " > ") || strings.Contains(cmd, " >> ") {
		return false, "output redirection not allowed"
	}

	if ok, reason := t.areBashPathsAllowed(cmd); !ok {
		return false, reason
	}

	// Check allowed prefixes
	for _, prefix := range bashAllowedPrefixes {
		if strings.HasPrefix(cmd, prefix) {
			return true, ""
		}
	}

	return false, "command not in allowed list"
}

var absPathPattern = regexp.MustCompile(`(?:^|[\s'"])(/[^\\s'"]+)`)

func (t *ExploreTools) areBashPathsAllowed(command string) (bool, string) {
	if strings.HasPrefix(command, "..") || strings.Contains(command, "../") {
		return false, "path traversal outside repository not allowed"
	}

	matches := absPathPattern.FindAllStringSubmatch(command, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		pathToken := strings.TrimRight(match[1], ".,;:")
		if !pathWithinRoot(t.repoRoot, pathToken) {
			return false, "absolute path outside repository not allowed"
		}
	}

	return true, ""
}

func pathWithinRoot(root, path string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

// truncateOutput limits output size.
func (t *ExploreTools) truncateOutput(output []byte) string {
	if len(output) <= maxBashOutput {
		return string(output)
	}

	truncated := output[:maxBashOutput]
	if lastNewline := strings.LastIndex(string(truncated), "\n"); lastNewline > maxBashOutput/2 {
		truncated = truncated[:lastNewline]
	}

	return string(truncated) + "\n\n[Output truncated]"
}

// withTokenEstimate appends a token cost estimate.
func withTokenEstimate(output string) string {
	tokenEstimate := len(output) / 4 // ~4 chars per token
	lineCount := strings.Count(output, "\n")
	return output + fmt.Sprintf("\n\n[~%d tokens, %d lines]", tokenEstimate, lineCount)
}

// executeCodegraph handles the codegraph tool.
func (t *ExploreTools) executeCodegraph(ctx context.Context, arguments string) (string, error) {
	params, err := llm.ParseToolArguments[CodegraphParams](arguments)
	if err != nil {
		return "", fmt.Errorf("parse codegraph params: %w", err)
	}

	// Check if codegraph is available
	if t.arango == nil {
		return "Codegraph is not available. Use grep and read tools instead.", nil
	}

	params.Operation = strings.ToLower(strings.TrimSpace(params.Operation))
	params.Kind = normalizeCodegraphKind(params.Kind)
	params.FromKind = normalizeCodegraphKind(params.FromKind)
	params.ToKind = normalizeCodegraphKind(params.ToKind)

	if errMsg := validateCodegraphKind(params.Kind); errMsg != "" {
		return errMsg, nil
	}
	if params.FromKind != "" && !isSupportedTraceEndpointKind(params.FromKind) {
		return fmt.Sprintf("Error: invalid from_kind %q. Supported kinds: function, method.", params.FromKind), nil
	}
	if params.ToKind != "" && !isSupportedTraceEndpointKind(params.ToKind) {
		return fmt.Sprintf("Error: invalid to_kind %q. Supported kinds: function, method.", params.ToKind), nil
	}

	// Validate and clamp depth
	depth := params.Depth
	if depth < 1 {
		depth = defaultGraphDepth
	}
	if depth > maxGraphDepth {
		depth = maxGraphDepth
	}

	switch params.Operation {
	case "search":
		return t.executeCodegraphSearch(ctx, params)
	case "resolve":
		return t.executeCodegraphResolve(ctx, params)
	case "file_symbols":
		return t.executeCodegraphFileSymbols(ctx, params)

	case "callers":
		qname, errMsg := t.resolveQNameForOperation(ctx, "callers", params)
		if errMsg != "" {
			return errMsg, nil
		}
		nodes, err := t.arango.GetCallers(ctx, qname, depth)
		if err != nil {
			slog.ErrorContext(ctx, "codegraph callers failed", "qname", qname, "error", err)
			return fmt.Sprintf("Error querying callers: %s", err), nil
		}
		return t.formatRelationshipResults("Callers", qname, depth, nodes), nil

	case "callees":
		qname, errMsg := t.resolveQNameForOperation(ctx, "callees", params)
		if errMsg != "" {
			return errMsg, nil
		}
		nodes, err := t.arango.GetCallees(ctx, qname, depth)
		if err != nil {
			slog.ErrorContext(ctx, "codegraph callees failed", "qname", qname, "error", err)
			return fmt.Sprintf("Error querying callees: %s", err), nil
		}
		return t.formatRelationshipResults("Callees", qname, depth, nodes), nil

	case "implementations":
		qname, errMsg := t.resolveQNameForOperation(ctx, "implementations", params)
		if errMsg != "" {
			return errMsg, nil
		}
		nodes, err := t.arango.GetImplementations(ctx, qname)
		if err != nil {
			slog.ErrorContext(ctx, "codegraph implementations failed", "qname", qname, "error", err)
			return fmt.Sprintf("Error querying implementations: %s", err), nil
		}
		return t.formatRelationshipResults("Implementations", qname, 1, nodes), nil

	case "usages":
		qname, errMsg := t.resolveQNameForOperation(ctx, "usages", params)
		if errMsg != "" {
			return errMsg, nil
		}
		nodes, err := t.arango.GetUsages(ctx, qname)
		if err != nil {
			slog.ErrorContext(ctx, "codegraph usages failed", "qname", qname, "error", err)
			return fmt.Sprintf("Error querying usages: %s", err), nil
		}
		return t.formatRelationshipResults("Usages", qname, 1, nodes), nil

	case "trace":
		params.Depth = depth
		return t.executeCodegraphTrace(ctx, params)

	default:
		return "Error: invalid operation. Valid operations: search, resolve, file_symbols, callers, callees, implementations, usages, trace", nil
	}
}

// executeCodegraphSearch handles symbol search.
func (t *ExploreTools) executeCodegraphSearch(ctx context.Context, params CodegraphParams) (string, error) {
	if params.Name == "" {
		return "Error: name parameter required for search operation", nil
	}

	results, total, err := t.arango.SearchSymbols(ctx, arangodb.SearchOptions{
		Name: params.Name,
		Kind: params.Kind,
		File: params.File,
	})
	if err != nil {
		slog.ErrorContext(ctx, "codegraph search failed", "name", params.Name, "error", err)
		return fmt.Sprintf("Error searching symbols: %s", err), nil
	}

	return t.formatSearchResults(params, results, total), nil
}

// formatSearchResults formats symbol search results.
func (t *ExploreTools) formatSearchResults(params CodegraphParams, results []arangodb.SearchResult, total int) string {
	if total == 0 {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("No symbols found matching %q", params.Name))
		if params.Kind != "" {
			sb.WriteString(fmt.Sprintf(" (kind=%s)", params.Kind))
		}
		if params.File != "" {
			sb.WriteString(fmt.Sprintf(" (file=%s)", params.File))
		}
		sb.WriteString(".")
		return sb.String()
	}

	filtered := make([]arangodb.SearchResult, 0, len(results))
	for _, r := range results {
		kind := normalizeCodegraphKind(r.Kind)
		if !isSupportedCodegraphKind(kind) {
			continue
		}
		r.Kind = kind
		filtered = append(filtered, r)
	}
	if len(filtered) == 0 {
		return fmt.Sprintf("No supported symbols found matching %q. Supported kinds: function, method, struct, interface, class.", params.Name)
	}

	displayResults := filtered
	truncated := false
	if len(displayResults) > maxSearchResults {
		displayResults = displayResults[:maxSearchResults]
		truncated = true
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d symbol(s) matching %q", len(filtered), params.Name))
	if params.Kind != "" {
		sb.WriteString(fmt.Sprintf(" (kind=%s)", params.Kind))
	}
	if params.File != "" {
		sb.WriteString(fmt.Sprintf(" (file=%s)", params.File))
	}
	sb.WriteString(":\n")

	for _, r := range displayResults {
		sb.WriteString(t.formatCodegraphLine(r.Filepath, r.Pos, r.Kind, r.QName, r.Signature))
		sb.WriteString("\n")
	}

	if truncated {
		sb.WriteString(fmt.Sprintf("\n[Showing %d of %d. Refine with kind/file to see more.]\n", len(displayResults), len(filtered)))
	}

	return strings.TrimSpace(sb.String())
}

// formatRelationshipResults formats callers/callees/implementations/usages results.
func (t *ExploreTools) formatRelationshipResults(operation string, qname string, depth int, nodes []arangodb.GraphNode) string {
	filtered := make([]arangodb.GraphNode, 0, len(nodes))
	for _, node := range nodes {
		if node.QName == "" {
			continue
		}
		node.Kind = normalizeCodegraphKind(node.Kind)
		filtered = append(filtered, node)
	}
	if len(filtered) == 0 {
		return fmt.Sprintf("No %s found for %s.", strings.ToLower(operation), qname)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s of %s (depth %d) - %d result(s):\n", operation, qname, depth, len(filtered)))
	for _, node := range filtered {
		sb.WriteString(t.formatCodegraphLine(node.Filepath, node.Pos, node.Kind, node.QName, node.Signature))
		sb.WriteString("\n")
	}

	return strings.TrimSpace(sb.String())
}

const (
	defaultTraceDepth        = 4
	maxTraceDepth            = 10
	maxCodegraphSignatureLen = 220
	maxFileSymbolsResults    = 50
)

var codegraphSupportedKindSet = map[string]struct{}{
	"function":  {},
	"method":    {},
	"struct":    {},
	"interface": {},
	"class":     {},
}

func isSupportedCodegraphKind(kind string) bool {
	_, ok := codegraphSupportedKindSet[kind]
	return ok
}

func normalizeCodegraphKind(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}

func validateCodegraphKind(kind string) string {
	if kind == "" {
		return ""
	}
	if isSupportedCodegraphKind(kind) {
		return ""
	}
	return fmt.Sprintf("Error: invalid kind %q. Supported kinds: function, method, struct, interface, class.", kind)
}

func isSupportedTraceEndpointKind(kind string) bool {
	return kind == "function" || kind == "method"
}

func (t *ExploreTools) formatCodegraphLine(filePath string, pos int, kind, qname, signature string) string {
	path := t.makeCodegraphPathRelative(filePath)
	if path == "" {
		path = filePath
	}
	if path == "" {
		path = "<unknown>"
	}

	location := path
	if pos > 0 {
		location = fmt.Sprintf("%s:%d", path, pos)
	}

	sig := sanitizeCodegraphSignature(signature)
	if sig == "" {
		return fmt.Sprintf("%s\t%s\t%s", location, kind, qname)
	}
	return fmt.Sprintf("%s\t%s\t%s\t%s", location, kind, qname, sig)
}

func (t *ExploreTools) makeCodegraphPathRelative(path string) string {
	if path == "" {
		return ""
	}

	prefix := strings.TrimRight(t.repoRoot, string(filepath.Separator)) + string(filepath.Separator)
	if strings.HasPrefix(path, prefix) {
		return strings.TrimPrefix(path, prefix)
	}
	return path
}

func sanitizeCodegraphSignature(sig string) string {
	sig = strings.ReplaceAll(sig, "\n", " ")
	sig = strings.Join(strings.Fields(sig), " ")
	sig = strings.TrimSpace(sig)
	if sig == "" {
		return ""
	}
	if len(sig) > maxCodegraphSignatureLen {
		return sig[:maxCodegraphSignatureLen] + "..."
	}
	return sig
}

func (t *ExploreTools) executeCodegraphResolve(ctx context.Context, params CodegraphParams) (string, error) {
	if params.Name == "" {
		return "Error: name parameter required for resolve operation", nil
	}

	symbol, err := t.resolveSymbol(ctx, params.Name, params.Kind, params.File)
	if err != nil {
		return t.formatResolveError(params.Name, params.Kind, params.File, err), nil
	}

	symbol.Kind = normalizeCodegraphKind(symbol.Kind)
	if symbol.Kind != "" && !isSupportedCodegraphKind(symbol.Kind) {
		return fmt.Sprintf("Error: resolved kind %q is unsupported. Supported kinds: function, method, struct, interface, class.", symbol.Kind), nil
	}

	return t.formatCodegraphLine(symbol.Filepath, symbol.Pos, symbol.Kind, symbol.QName, symbol.Signature), nil
}

func (t *ExploreTools) executeCodegraphFileSymbols(ctx context.Context, params CodegraphParams) (string, error) {
	if params.File == "" {
		return "Error: file parameter required for file_symbols operation", nil
	}

	symbols, err := t.arango.GetFileSymbols(ctx, arangodb.FileSymbolsOptions{Filepath: params.File, Kind: params.Kind})
	if err != nil {
		slog.ErrorContext(ctx, "codegraph file_symbols failed", "file", params.File, "error", err)
		return fmt.Sprintf("Error querying file symbols: %s", err), nil
	}

	filtered := make([]arangodb.FileSymbol, 0, len(symbols))
	for _, s := range symbols {
		kind := normalizeCodegraphKind(s.Kind)
		if !isSupportedCodegraphKind(kind) {
			continue
		}
		s.Kind = kind
		filtered = append(filtered, s)
	}

	if len(filtered) == 0 {
		return fmt.Sprintf("No supported symbols found in %s.", params.File), nil
	}

	display := filtered
	truncated := false
	if len(display) > maxFileSymbolsResults {
		display = display[:maxFileSymbolsResults]
		truncated = true
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Symbols in %s:\n", params.File))
	for _, s := range display {
		sb.WriteString(t.formatCodegraphLine(params.File, s.Pos, s.Kind, s.QName, s.Signature))
		sb.WriteString("\n")
	}
	if truncated {
		sb.WriteString(fmt.Sprintf("\n[Showing %d of %d. Use kind filter to narrow.]\n", len(display), len(filtered)))
	}

	return strings.TrimSpace(sb.String()), nil
}

func (t *ExploreTools) resolveQNameForOperation(ctx context.Context, operation string, params CodegraphParams) (string, string) {
	if params.QName != "" {
		return params.QName, ""
	}

	name := strings.TrimSpace(params.Name)
	if name == "" {
		return "", fmt.Sprintf("Error: %s requires qname or name.", operation)
	}

	file := strings.TrimSpace(params.File)

	switch operation {
	case "callers", "callees":
		if params.Kind != "" && !isSupportedTraceEndpointKind(params.Kind) {
			return "", fmt.Sprintf("Error: %s with name requires kind=function or kind=method (or omit kind).", operation)
		}
		if params.Kind != "" {
			symbol, err := t.resolveSymbol(ctx, name, params.Kind, file)
			if err != nil {
				return "", t.formatResolveError(name, params.Kind, file, err)
			}
			symbol.Kind = normalizeCodegraphKind(symbol.Kind)
			if !isSupportedTraceEndpointKind(symbol.Kind) {
				return "", fmt.Sprintf("Error: %s resolved %q to kind=%s, but requires function or method.", operation, name, symbol.Kind)
			}
			return symbol.QName, ""
		}
		symbol, errMsg := t.resolveSymbolForKinds(ctx, name, file, []string{"method", "function"})
		if errMsg != "" {
			return "", errMsg
		}
		return symbol.QName, ""

	case "implementations":
		if params.Kind != "" && params.Kind != "interface" && params.Kind != "class" {
			return "", "Error: implementations with name requires kind=interface or kind=class (or omit kind)."
		}
		if params.Kind != "" {
			symbol, err := t.resolveSymbol(ctx, name, params.Kind, file)
			if err != nil {
				return "", t.formatResolveError(name, params.Kind, file, err)
			}
			symbol.Kind = normalizeCodegraphKind(symbol.Kind)
			if symbol.Kind != "interface" && symbol.Kind != "class" {
				return "", fmt.Sprintf("Error: implementations resolved %q to kind=%s, but requires interface or class.", name, symbol.Kind)
			}
			return symbol.QName, ""
		}
		symbol, errMsg := t.resolveSymbolForKinds(ctx, name, file, []string{"interface", "class"})
		if errMsg != "" {
			return "", errMsg
		}
		return symbol.QName, ""

	case "usages":
		if params.Kind != "" && params.Kind != "struct" && params.Kind != "interface" && params.Kind != "class" {
			return "", "Error: usages with name requires kind=struct, kind=interface, or kind=class (or omit kind)."
		}
		if params.Kind != "" {
			symbol, err := t.resolveSymbol(ctx, name, params.Kind, file)
			if err != nil {
				return "", t.formatResolveError(name, params.Kind, file, err)
			}
			symbol.Kind = normalizeCodegraphKind(symbol.Kind)
			if symbol.Kind != "struct" && symbol.Kind != "interface" && symbol.Kind != "class" {
				return "", fmt.Sprintf("Error: usages resolved %q to kind=%s, but requires struct, interface, or class.", name, symbol.Kind)
			}
			return symbol.QName, ""
		}
		symbol, errMsg := t.resolveSymbolForKinds(ctx, name, file, []string{"struct", "interface", "class"})
		if errMsg != "" {
			return "", errMsg
		}
		return symbol.QName, ""

	default:
		symbol, err := t.resolveSymbol(ctx, name, params.Kind, file)
		if err != nil {
			return "", t.formatResolveError(name, params.Kind, file, err)
		}
		return symbol.QName, ""
	}
}

func (t *ExploreTools) resolveSymbolForKinds(ctx context.Context, name, file string, kinds []string) (arangodb.ResolvedSymbol, string) {
	for _, kind := range kinds {
		symbol, err := t.resolveSymbol(ctx, name, kind, file)
		if err == nil {
			symbol.Kind = normalizeCodegraphKind(symbol.Kind)
			return symbol, ""
		}

		var amb arangodb.AmbiguousSymbolError
		if errors.As(err, &amb) {
			return arangodb.ResolvedSymbol{}, t.formatAmbiguousSymbolError(amb)
		}
		if errors.Is(err, arangodb.ErrNotFound) {
			continue
		}
		return arangodb.ResolvedSymbol{}, t.formatResolveError(name, kind, file, err)
	}

	return arangodb.ResolvedSymbol{}, fmt.Sprintf("Error: no symbol found matching %q for kinds: %s. Add file to disambiguate or pass qname.", name, strings.Join(kinds, ", "))
}

func (t *ExploreTools) resolveSymbol(ctx context.Context, name, kind, file string) (arangodb.ResolvedSymbol, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return arangodb.ResolvedSymbol{}, fmt.Errorf("name is required")
	}

	opts := arangodb.SearchOptions{
		Name: name,
		Kind: kind,
		File: file,
	}
	symbol, err := t.arango.ResolveSymbol(ctx, opts)
	if err == nil {
		return symbol, nil
	}

	if errors.Is(err, arangodb.ErrNotFound) && !strings.Contains(name, "*") {
		opts.Name = "*" + name + "*"
		symbol, err2 := t.arango.ResolveSymbol(ctx, opts)
		if err2 == nil {
			return symbol, nil
		}
		err = err2
	}

	return arangodb.ResolvedSymbol{}, err
}

func (t *ExploreTools) formatResolveError(name, kind, file string, err error) string {
	var amb arangodb.AmbiguousSymbolError
	if errors.As(err, &amb) {
		return t.formatAmbiguousSymbolError(amb)
	}
	if errors.Is(err, arangodb.ErrNotFound) {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Error: no symbol found matching %q", name))
		if kind != "" {
			sb.WriteString(fmt.Sprintf(" (kind=%s)", kind))
		}
		if file != "" {
			sb.WriteString(fmt.Sprintf(" (file=%s)", file))
		}
		sb.WriteString(".")
		return sb.String()
	}
	return fmt.Sprintf("Error resolving %q: %s", name, err)
}

func (t *ExploreTools) formatAmbiguousSymbolError(err arangodb.AmbiguousSymbolError) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Error: ambiguous symbol %q. Candidates:\n", err.Query))
	for _, c := range err.Candidates {
		kind := normalizeCodegraphKind(c.Kind)
		if !isSupportedCodegraphKind(kind) {
			continue
		}
		sb.WriteString(t.formatCodegraphLine(c.Filepath, c.Pos, kind, c.QName, c.Signature))
		sb.WriteString("\n")
	}
	sb.WriteString("Refine with kind/file, or pass qname.")
	return strings.TrimSpace(sb.String())
}

func (t *ExploreTools) executeCodegraphTrace(ctx context.Context, params CodegraphParams) (string, error) {
	maxDepth := params.MaxDepth
	if maxDepth < 1 {
		maxDepth = defaultTraceDepth
	}
	if maxDepth > maxTraceDepth {
		maxDepth = maxTraceDepth
	}

	fromQName, errMsg := t.resolveTraceEndpointQName(ctx, "from", params.FromName, params.FromQName, params.FromKind, params.FromFile)
	if errMsg != "" {
		return errMsg, nil
	}
	toQName, errMsg := t.resolveTraceEndpointQName(ctx, "to", params.ToName, params.ToQName, params.ToKind, params.ToFile)
	if errMsg != "" {
		return errMsg, nil
	}

	path, err := t.arango.FindCallPath(ctx, fromQName, toQName, maxDepth)
	if err != nil {
		slog.ErrorContext(ctx, "codegraph trace failed", "from", fromQName, "to", toQName, "error", err)
		return fmt.Sprintf("Error tracing call path: %s", err), nil
	}
	if len(path) == 0 {
		return t.formatTraceNotFound(fromQName, toQName, maxDepth), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Trace path from %s to %s (max_depth=%d) - %d step(s):\n", fromQName, toQName, maxDepth, len(path)))
	for _, node := range path {
		if node.QName == "" {
			continue
		}
		kind := normalizeCodegraphKind(node.Kind)
		sb.WriteString(t.formatCodegraphLine(node.Filepath, node.Pos, kind, node.QName, node.Signature))
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String()), nil
}

// formatTraceNotFound generates an actionable error message when trace finds no path.
func (t *ExploreTools) formatTraceNotFound(fromQName, toQName string, maxDepth int) string {
	return fmt.Sprintf("No direct call path found from %s to %s (max_depth=%d). Try callers/callees + grep.",
		fromQName, toQName, maxDepth)
}

func (t *ExploreTools) resolveTraceEndpointQName(ctx context.Context, label, name, qname, kind, file string) (string, string) {
	if strings.TrimSpace(qname) != "" {
		return strings.TrimSpace(qname), ""
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Sprintf("Error: trace requires %s_qname or %s_name.", label, label)
	}

	file = strings.TrimSpace(file)

	if kind != "" {
		symbol, err := t.resolveSymbol(ctx, name, kind, file)
		if err != nil {
			return "", t.formatResolveError(name, kind, file, err)
		}
		symbol.Kind = normalizeCodegraphKind(symbol.Kind)
		if !isSupportedTraceEndpointKind(symbol.Kind) {
			return "", fmt.Sprintf("Error: trace %s resolved %q to kind=%s, but requires function or method.", label, name, symbol.Kind)
		}
		return symbol.QName, ""
	}

	symbol, errMsg := t.resolveSymbolForKinds(ctx, name, file, []string{"method", "function"})
	if errMsg != "" {
		return "", errMsg
	}
	if !isSupportedTraceEndpointKind(symbol.Kind) {
		return "", fmt.Sprintf("Error: trace %s resolved %q to kind=%s, but requires function or method.", label, name, symbol.Kind)
	}
	return symbol.QName, ""
}
