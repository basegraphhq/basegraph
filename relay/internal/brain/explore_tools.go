package brain

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"basegraph.app/relay/common/arangodb"
	"basegraph.app/relay/common/llm"
)

const (
	defaultGrepLimit  = 30  // Max grep matches - keep focused
	maxGrepLimit      = 50  // Hard limit
	defaultReadLines  = 100 // Default lines to read
	maxReadLines      = 300 // Increased from 200 - let model read more when needed
	maxLineLength     = 500 // Truncate long lines
	maxGlobResults    = 50  // Max files from glob
	defaultGraphDepth = 1
	maxGraphDepth     = 3 // Increased from 2 - deeper call chain analysis
	defaultTreeDepth  = 2
	maxTreeDepth      = 4
	maxTreeEntries    = 200 // Prevent token explosion on large repos
)

// GrepParams for the Grep tool.
type GrepParams struct {
	Pattern string `json:"pattern" jsonschema:"required,description=Regex pattern to search for"`
	Include string `json:"include,omitempty" jsonschema:"description=File glob pattern (e.g. '*.go', '*.ts'). Default: all files"`
	Limit   int    `json:"limit,omitempty" jsonschema:"description=Max results (default 50, max 100)"`
}

// GlobParams for the Glob tool.
type GlobParams struct {
	Pattern string `json:"pattern" jsonschema:"required,description=Glob pattern (e.g. '**/*.go', '**/retriever*.go', 'internal/brain/*.go')"`
}

// ReadParams for the Read tool.
type ReadParams struct {
	File      string `json:"file" jsonschema:"required,description=File path to read"`
	StartLine int    `json:"start_line,omitempty" jsonschema:"description=Line to start from (1-indexed, default 1)"`
	NumLines  int    `json:"num_lines,omitempty" jsonschema:"description=Number of lines to read (default 100, max 300)"`
}

// GraphParams for the Graph tool.
type GraphParams struct {
	Operation string `json:"operation" jsonschema:"required,enum=callers,enum=callees,enum=implementations,enum=methods,enum=usages,enum=inheritors,description=Graph operation: callers, callees, implementations, methods, usages, inheritors"`
	Target    string `json:"target" jsonschema:"required,description=Qualified name of the entity to query (e.g. 'basegraph.app/relay/internal/brain.Retriever')"`
	Depth     int    `json:"depth,omitempty" jsonschema:"description=Traversal depth for callers/callees (default 1, max 3)"`
}

// TreeParams for the Tree tool.
type TreeParams struct {
	Path  string `json:"path,omitempty" jsonschema:"description=Directory to list (default: repo root)"`
	Depth int    `json:"depth,omitempty" jsonschema:"description=Max depth (default 2, max 4)"`
}

// ExploreTools provides Grep, Glob, Read, and Graph tools for the ExploreAgent sub-agent.
type ExploreTools struct {
	repoRoot    string
	arango      arangodb.Client
	definitions []llm.Tool
}

// NewExploreTools creates tools for code exploration.
// repoRoot is the root directory of the repository to search.
// modulePath is the Go module path for qualified name examples in tool descriptions.
func NewExploreTools(repoRoot string, arango arangodb.Client) *ExploreTools {
	t := &ExploreTools{
		repoRoot: repoRoot,
		arango:   arango,
	}

	// Enhanced tool descriptions following Anthropic's guidance:
	// "We put a lot of effort into the descriptions and specs for these tools...
	// We believe that much more attention should go into designing tool interfaces
	// for models, in the same way that attention goes into designing tool interfaces for humans."

	t.definitions = []llm.Tool{
		{
			Name: "grep",
			Description: `Search file contents using regex pattern.

WHEN TO USE:
- Finding where something is defined or used
- Locating specific strings, function names, error messages
- Discovering patterns across the codebase

USAGE TIPS:
* Be specific - if you get >30 results, your pattern is too broad
* Use 'include' to filter: include="*.go" for Go, include="*.ts" for TypeScript
* Regex supported: "func.*Handler", "error.*timeout", etc.
* Case-sensitive by default

RETURNS: file:line matches sorted by modification time (most recent first)
Each line truncated at 500 chars. Shows match context.

COMMON PATTERNS:
- Find function: "func\s+FunctionName"
- Find struct: "type\s+StructName\s+struct"
- Find interface impl: "func\s+\([^)]+\)\s+MethodName\("
- Find imports: "import.*packagename"`,
			Parameters: llm.GenerateSchemaFrom(GrepParams{}),
		},
		{
			Name: "glob",
			Description: `Find files by path pattern.

WHEN TO USE:
- Discovering file structure
- Finding all files of a type
- Locating files by naming convention

PATTERN SYNTAX:
* "**/*.go" - all Go files recursively
* "internal/**/*.go" - Go files under internal/
* "**/test_*.py" - test files
* "cmd/*/main.go" - main files in cmd subdirs

RETURNS: File paths sorted by modification time (most recent first)
Limited to 50 results. Use more specific patterns if truncated.`,
			Parameters: llm.GenerateSchemaFrom(GlobParams{}),
		},
		{
			Name: "read",
			Description: `Read file contents with line numbers.

WHEN TO USE:
- After grep/glob found a relevant file
- Understanding implementation details
- Reading specific functions or sections

PARAMETERS:
* file: Path to file (required)
* start_line: Line to start from (1-indexed, default 1)
* num_lines: Lines to read (default 100, max 300)

EFFICIENCY TIP: Use start_line and num_lines to read only what you need.
If grep found a match at line 150, read lines 140-180, not the whole file.

RETURNS: Lines with 4-digit line numbers. Long lines truncated at 500 chars.`,
			Parameters: llm.GenerateSchemaFrom(ReadParams{}),
		},
		{
			Name: "graph",
			Description: `Query code structure relationships from the semantic graph.

THIS IS POWERFUL - use it to understand code relationships without grepping.

OPERATIONS:
* callers(target, depth): Who calls this function? (depth 1-3)
* callees(target, depth): What does this function call? (depth 1-3)
* implementations(target): What types implement this interface?
* methods(target): What methods does this type have?
* usages(target): Where is this type/function referenced?
* inheritors(target): What interfaces embed this interface?

QUALIFIED NAME FORMAT:
Pattern: {module}/{package}.{Type}.{Method}

EXAMPLES:
* Function: basegraph.app/relay/internal/brain.Planner.Plan
* Method: basegraph.app/relay/internal/brain.ExploreAgent.Explore
* Type: basegraph.app/relay/internal/brain.Planner
* Interface: basegraph.app/relay/common/llm.AgentClient

WHEN TO USE GRAPH vs GREP:
* "What calls this function?" → graph(callers, ...)
* "What implements this interface?" → graph(implementations, ...)
* "Where is this string literal?" → grep
* "What files match this pattern?" → glob

Graph queries are faster and more accurate for structural questions.`,
			Parameters: llm.GenerateSchemaFrom(GraphParams{}),
		},
		{
			Name: "tree",
			Description: `Show directory structure.

WHEN TO USE:
- First step when exploring unfamiliar area
- Before grep/glob to understand where to look
- When you need to find the right directory

PARAMETERS:
* path: Directory to list (default: repo root)
* depth: How deep to go (default 2, max 4)

RETURNS: Tree view of directories and files.
Directories shown with trailing /, sorted before files.
Limited to 200 entries. Use path param to focus on specific areas.

EXAMPLE: After tree(path="internal"), you know to grep in "internal/brain/" not "**/*brain*"

EFFICIENCY: Use this BEFORE glob/grep to understand project structure.
Saves tokens by helping you target searches.`,
			Parameters: llm.GenerateSchemaFrom(TreeParams{}),
		},
	}

	return t
}

// Definitions returns tool definitions for the LLM.
func (t *ExploreTools) Definitions() []llm.Tool {
	return t.definitions
}

// Execute runs a tool by name and returns prose output.
func (t *ExploreTools) Execute(ctx context.Context, name, arguments string) (string, error) {
	switch name {
	case "grep":
		return t.executeGrep(ctx, arguments)
	case "glob":
		return t.executeGlob(ctx, arguments)
	case "read":
		return t.executeRead(ctx, arguments)
	case "graph":
		return t.executeGraph(ctx, arguments)
	case "tree":
		return t.executeTree(ctx, arguments)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

func (t *ExploreTools) executeGrep(ctx context.Context, arguments string) (string, error) {
	params, err := llm.ParseToolArguments[GrepParams](arguments)
	if err != nil {
		return "", fmt.Errorf("parse grep params: %w", err)
	}

	limit := params.Limit
	if limit <= 0 {
		limit = defaultGrepLimit
	}
	if limit > maxGrepLimit {
		limit = maxGrepLimit
	}

	// Build ripgrep command
	args := []string{
		"--line-number",
		"--no-heading",
		"--color=never",
		"--max-count", strconv.Itoa(limit),
	}

	if params.Include != "" {
		args = append(args, "--glob", params.Include)
	}

	// Exclude common non-code directories
	args = append(args,
		"--glob", "!.git",
		"--glob", "!node_modules",
		"--glob", "!vendor",
		"--glob", "!*.min.js",
	)

	args = append(args, params.Pattern, ".")

	cmd := exec.CommandContext(ctx, "rg", args...)
	cmd.Dir = t.repoRoot

	output, err := cmd.Output()
	if err != nil {
		// Exit code 1 means no matches (not an error)
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return fmt.Sprintf("No matches found for pattern '%s'", params.Pattern), nil
		}
		return fmt.Sprintf("Grep error: %s", err), nil
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return fmt.Sprintf("No matches found for pattern '%s'", params.Pattern), nil
	}

	var out strings.Builder
	out.WriteString(fmt.Sprintf("Found %d matches for '%s':\n\n", len(lines), params.Pattern))

	for _, line := range lines {
		// Truncate long lines to prevent context bloat
		if len(line) > maxLineLength {
			line = line[:maxLineLength] + "..."
		}
		out.WriteString(line)
		out.WriteString("\n")
	}

	if len(lines) >= limit {
		out.WriteString(fmt.Sprintf("\n(Results limited to %d. Use a more specific pattern or include filter.)\n", limit))
	}

	slog.DebugContext(ctx, "grep executed",
		"pattern", params.Pattern,
		"include", params.Include,
		"matches", len(lines))

	return out.String(), nil
}

func (t *ExploreTools) executeGlob(ctx context.Context, arguments string) (string, error) {
	params, err := llm.ParseToolArguments[GlobParams](arguments)
	if err != nil {
		return "", fmt.Errorf("parse glob params: %w", err)
	}

	// Use find with glob pattern or filepath.Glob
	pattern := filepath.Join(t.repoRoot, params.Pattern)

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Sprintf("Glob error: %s", err), nil
	}

	if len(matches) == 0 {
		// Try with fd for more flexible globbing
		args := []string{
			"--type", "f",
			"--glob", params.Pattern,
			"--color=never",
		}

		cmd := exec.CommandContext(ctx, "fd", args...)
		cmd.Dir = t.repoRoot

		output, err := cmd.Output()
		if err != nil {
			return fmt.Sprintf("No files found matching '%s'", params.Pattern), nil
		}

		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
			return fmt.Sprintf("No files found matching '%s'", params.Pattern), nil
		}

		truncated := len(lines) > maxGlobResults
		if truncated {
			lines = lines[:maxGlobResults]
		}

		var out strings.Builder
		out.WriteString(fmt.Sprintf("Found %d files matching '%s':\n\n", len(lines), params.Pattern))
		for _, line := range lines {
			out.WriteString(line)
			out.WriteString("\n")
		}
		if truncated {
			out.WriteString(fmt.Sprintf("\n(Results limited to %d. Use a more specific pattern.)\n", maxGlobResults))
		}
		return out.String(), nil
	}

	truncated := len(matches) > maxGlobResults
	if truncated {
		matches = matches[:maxGlobResults]
	}

	var out strings.Builder
	out.WriteString(fmt.Sprintf("Found %d files matching '%s':\n\n", len(matches), params.Pattern))

	for _, match := range matches {
		// Make path relative to repo root
		rel, _ := filepath.Rel(t.repoRoot, match)
		out.WriteString(rel)
		out.WriteString("\n")
	}

	if truncated {
		out.WriteString(fmt.Sprintf("\n(Results limited to %d. Use a more specific pattern.)\n", maxGlobResults))
	}

	slog.DebugContext(ctx, "glob executed",
		"pattern", params.Pattern,
		"matches", len(matches))

	return out.String(), nil
}

func (t *ExploreTools) executeRead(ctx context.Context, arguments string) (string, error) {
	params, err := llm.ParseToolArguments[ReadParams](arguments)
	if err != nil {
		return "", fmt.Errorf("parse read params: %w", err)
	}

	// Resolve file path
	filePath := params.File
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(t.repoRoot, filePath)
	}

	// Security check: ensure path is within repo root
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Sprintf("Invalid path: %s", err), nil
	}
	absRoot, _ := filepath.Abs(t.repoRoot)
	if !strings.HasPrefix(absPath, absRoot) {
		return "Error: path outside repository", nil
	}

	file, err := os.Open(absPath)
	if err != nil {
		return fmt.Sprintf("Cannot open file: %s", err), nil
	}
	defer file.Close()

	startLine := params.StartLine
	if startLine <= 0 {
		startLine = 1
	}

	numLines := params.NumLines
	if numLines <= 0 {
		numLines = defaultReadLines
	}
	if numLines > maxReadLines {
		numLines = maxReadLines
	}

	var out strings.Builder
	relPath, _ := filepath.Rel(t.repoRoot, absPath)
	out.WriteString(fmt.Sprintf("## %s (lines %d-%d)\n\n```\n", relPath, startLine, startLine+numLines-1))

	scanner := bufio.NewScanner(file)
	lineNum := 0
	linesRead := 0

	for scanner.Scan() {
		lineNum++
		if lineNum < startLine {
			continue
		}
		if linesRead >= numLines {
			break
		}

		line := scanner.Text()
		// Truncate long lines to prevent context bloat
		if len(line) > maxLineLength {
			line = line[:maxLineLength] + "..."
		}
		out.WriteString(fmt.Sprintf("%4d | %s\n", lineNum, line))
		linesRead++
	}

	out.WriteString("```\n")

	if err := scanner.Err(); err != nil {
		return fmt.Sprintf("Error reading file: %s", err), nil
	}

	slog.DebugContext(ctx, "read executed",
		"file", relPath,
		"start", startLine,
		"lines", linesRead)

	return out.String(), nil
}

func (t *ExploreTools) executeGraph(ctx context.Context, arguments string) (string, error) {
	params, err := llm.ParseToolArguments[GraphParams](arguments)
	if err != nil {
		return "", fmt.Errorf("parse graph params: %w", err)
	}

	depth := params.Depth
	if depth <= 0 {
		depth = defaultGraphDepth
	}
	if depth > maxGraphDepth {
		depth = maxGraphDepth
	}

	var nodes []arangodb.GraphNode
	var opErr error

	switch params.Operation {
	case "callers":
		nodes, opErr = t.arango.GetCallers(ctx, params.Target, depth)
	case "callees":
		nodes, opErr = t.arango.GetCallees(ctx, params.Target, depth)
	case "implementations":
		nodes, opErr = t.arango.GetImplementations(ctx, params.Target)
	case "methods":
		nodes, opErr = t.arango.GetMethods(ctx, params.Target)
	case "usages":
		nodes, opErr = t.arango.GetUsages(ctx, params.Target)
	case "inheritors":
		nodes, opErr = t.arango.GetInheritors(ctx, params.Target)
	default:
		return fmt.Sprintf("Unknown graph operation: %s. Valid: callers, callees, implementations, methods, usages, inheritors", params.Operation), nil
	}

	if opErr != nil {
		return fmt.Sprintf("Graph error: %s", opErr), nil
	}

	if len(nodes) == 0 {
		return fmt.Sprintf("%s of %s: No results found.\n", capitalize(params.Operation), params.Target), nil
	}

	var out strings.Builder
	out.WriteString(fmt.Sprintf("%s of %s", capitalize(params.Operation), params.Target))
	if params.Operation == "callers" || params.Operation == "callees" {
		out.WriteString(fmt.Sprintf(" (depth %d)", depth))
	}
	out.WriteString(fmt.Sprintf(" - %d results:\n\n", len(nodes)))

	for _, n := range nodes {
		out.WriteString(fmt.Sprintf("- %s (%s)\n", n.Name, n.Kind))
		out.WriteString(fmt.Sprintf("  qname: %s\n", n.QName))
		if n.Filepath != "" {
			out.WriteString(fmt.Sprintf("  file: %s\n", n.Filepath))
		}
		out.WriteString("\n")
	}

	out.WriteString("Use read(file) to see the code for any of these.\n")

	slog.DebugContext(ctx, "graph executed",
		"operation", params.Operation,
		"target", params.Target,
		"depth", depth,
		"results", len(nodes))

	return out.String(), nil
}

func (t *ExploreTools) executeTree(ctx context.Context, arguments string) (string, error) {
	params, err := llm.ParseToolArguments[TreeParams](arguments)
	if err != nil {
		return "", fmt.Errorf("parse tree params: %w", err)
	}

	depth := params.Depth
	if depth <= 0 {
		depth = defaultTreeDepth
	}
	if depth > maxTreeDepth {
		depth = maxTreeDepth
	}

	// Security check: reject absolute paths immediately
	if params.Path != "" && filepath.IsAbs(params.Path) {
		return "Error: path outside repository", nil
	}

	// Resolve path
	rootPath := t.repoRoot
	if params.Path != "" {
		rootPath = filepath.Join(t.repoRoot, params.Path)
	}

	// Security check: ensure path is within repo root
	absPath, err := filepath.Abs(rootPath)
	if err != nil {
		return fmt.Sprintf("Invalid path: %s", err), nil
	}
	absRoot, _ := filepath.Abs(t.repoRoot)
	// Use filepath.Rel to properly check containment (handles /repo vs /repo-evil)
	relPath, err := filepath.Rel(absRoot, absPath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return "Error: path outside repository", nil
	}

	// Check if path exists and is a directory
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("Directory not found: %s", params.Path), nil
		}
		return fmt.Sprintf("Cannot access path: %s", err), nil
	}
	if !info.IsDir() {
		return fmt.Sprintf("Not a directory: %s", params.Path), nil
	}

	// Build tree
	entries, truncated := t.buildTree(absPath, depth)

	// Determine display path
	displayPath := params.Path
	if displayPath == "" {
		displayPath = "."
	}

	if len(entries) == 0 {
		return fmt.Sprintf("Directory is empty: %s", displayPath), nil
	}

	var out strings.Builder
	out.WriteString(fmt.Sprintf("%s/\n", displayPath))

	for _, entry := range entries {
		out.WriteString(entry)
		out.WriteString("\n")
	}

	if truncated {
		out.WriteString(fmt.Sprintf("\n(Truncated at %d entries. Use path param to focus on a subdirectory.)\n", maxTreeEntries))
	}

	slog.DebugContext(ctx, "tree executed",
		"path", params.Path,
		"depth", depth,
		"entries", len(entries))

	return out.String(), nil
}

// buildTree recursively builds a tree view of the directory structure.
// Returns the formatted entries and whether the result was truncated.
func (t *ExploreTools) buildTree(dirPath string, maxDepth int) ([]string, bool) {
	var entries []string
	truncated := false

	var walk func(path string, depth int, prefix string)
	walk = func(path string, depth int, prefix string) {
		if depth > maxDepth || len(entries) >= maxTreeEntries {
			if len(entries) >= maxTreeEntries {
				truncated = true
			}
			return
		}

		items, err := os.ReadDir(path)
		if err != nil {
			return
		}

		// Separate dirs and files, filter excluded directories
		var dirs, files []os.DirEntry
		for _, item := range items {
			if item.IsDir() {
				if !isExcludedDir(item.Name()) {
					dirs = append(dirs, item)
				}
			} else {
				files = append(files, item)
			}
		}

		// Process directories first (sorted), then files (sorted)
		for i, dir := range dirs {
			if len(entries) >= maxTreeEntries {
				truncated = true
				return
			}

			isLast := i == len(dirs)-1 && len(files) == 0
			connector := "├── "
			if isLast {
				connector = "└── "
			}

			entries = append(entries, prefix+connector+dir.Name()+"/")

			// Recurse into subdirectory
			newPrefix := prefix + "│   "
			if isLast {
				newPrefix = prefix + "    "
			}
			walk(filepath.Join(path, dir.Name()), depth+1, newPrefix)
		}

		for i, file := range files {
			if len(entries) >= maxTreeEntries {
				truncated = true
				return
			}

			isLast := i == len(files)-1
			connector := "├── "
			if isLast {
				connector = "└── "
			}

			entries = append(entries, prefix+connector+file.Name())
		}
	}

	walk(dirPath, 1, "")
	return entries, truncated
}

// excludedDirs contains directories to skip in tree output.
var excludedDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"__pycache__":  true,
	".next":        true,
	"dist":         true,
	"build":        true,
	".idea":        true,
	".vscode":      true,
	".cache":       true,
	"coverage":     true,
	".turbo":       true,
	"target":       true, // Rust
}

// isExcludedDir returns true for directories that should be excluded from tree output.
func isExcludedDir(name string) bool {
	return excludedDirs[name]
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
