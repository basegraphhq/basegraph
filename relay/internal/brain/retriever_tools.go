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
	defaultGrepLimit  = 50  // Max grep matches
	maxGrepLimit      = 100 // Hard limit
	defaultReadLines  = 200 // Default lines to read
	maxReadLines      = 500 // Max lines per read
	defaultGraphDepth = 1
	maxGraphDepth     = 3
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
	NumLines  int    `json:"num_lines,omitempty" jsonschema:"description=Number of lines to read (default 200, max 500)"`
}

// GraphParams for the Graph tool.
type GraphParams struct {
	Operation string `json:"operation" jsonschema:"required,enum=callers,enum=callees,enum=implementations,enum=methods,enum=usages,enum=inheritors,description=Graph operation: callers, callees, implementations, methods, usages, inheritors"`
	Target    string `json:"target" jsonschema:"required,description=Qualified name of the entity to query (e.g. 'basegraph.app/relay/internal/brain.Retriever')"`
	Depth     int    `json:"depth,omitempty" jsonschema:"description=Traversal depth for callers/callees (default 1, max 3)"`
}

// RetrieverTools provides Grep, Glob, Read, and Graph tools for the Retriever sub-agent.
type RetrieverTools struct {
	repoRoot    string
	arango      arangodb.Client
	definitions []llm.Tool
}

// NewRetrieverTools creates tools for code exploration.
// repoRoot is the root directory of the repository to search.
func NewRetrieverTools(repoRoot string, arango arangodb.Client) *RetrieverTools {
	t := &RetrieverTools{
		repoRoot: repoRoot,
		arango:   arango,
	}

	t.definitions = []llm.Tool{
		{
			Name:        "grep",
			Description: "Search file contents using regex. Returns file:line matches with context.",
			Parameters:  llm.GenerateSchemaFrom(GrepParams{}),
		},
		{
			Name:        "glob",
			Description: "Find files by path pattern. Use to discover file structure.",
			Parameters:  llm.GenerateSchemaFrom(GlobParams{}),
		},
		{
			Name:        "read",
			Description: "Read file contents. Use after grep/glob to see full code.",
			Parameters:  llm.GenerateSchemaFrom(ReadParams{}),
		},
		{
			Name:        "graph",
			Description: "Query code relationships (callers, callees, implementations, methods, usages, inheritors). Use to find who calls a function, what implements an interface, or trace call chains. Requires a qualified name (qname) - see system prompt for format.",
			Parameters:  llm.GenerateSchemaFrom(GraphParams{}),
		},
	}

	return t
}

// Definitions returns tool definitions for the LLM.
func (t *RetrieverTools) Definitions() []llm.Tool {
	return t.definitions
}

// Execute runs a tool by name and returns prose output.
func (t *RetrieverTools) Execute(ctx context.Context, name, arguments string) (string, error) {
	switch name {
	case "grep":
		return t.executeGrep(ctx, arguments)
	case "glob":
		return t.executeGlob(ctx, arguments)
	case "read":
		return t.executeRead(ctx, arguments)
	case "graph":
		return t.executeGraph(ctx, arguments)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

func (t *RetrieverTools) executeGrep(ctx context.Context, arguments string) (string, error) {
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
		out.WriteString(line)
		out.WriteString("\n")
	}

	slog.DebugContext(ctx, "grep executed",
		"pattern", params.Pattern,
		"include", params.Include,
		"matches", len(lines))

	return out.String(), nil
}

func (t *RetrieverTools) executeGlob(ctx context.Context, arguments string) (string, error) {
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

		var out strings.Builder
		out.WriteString(fmt.Sprintf("Found %d files matching '%s':\n\n", len(lines), params.Pattern))
		for _, line := range lines {
			out.WriteString(line)
			out.WriteString("\n")
		}
		return out.String(), nil
	}

	var out strings.Builder
	out.WriteString(fmt.Sprintf("Found %d files matching '%s':\n\n", len(matches), params.Pattern))

	for _, match := range matches {
		// Make path relative to repo root
		rel, _ := filepath.Rel(t.repoRoot, match)
		out.WriteString(rel)
		out.WriteString("\n")
	}

	slog.DebugContext(ctx, "glob executed",
		"pattern", params.Pattern,
		"matches", len(matches))

	return out.String(), nil
}

func (t *RetrieverTools) executeRead(ctx context.Context, arguments string) (string, error) {
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

		out.WriteString(fmt.Sprintf("%4d | %s\n", lineNum, scanner.Text()))
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

func (t *RetrieverTools) executeGraph(ctx context.Context, arguments string) (string, error) {
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

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
