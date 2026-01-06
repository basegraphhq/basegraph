package brain

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

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

// ExploreTools provides Claude Code-style tools for the ExploreAgent.
type ExploreTools struct {
	repoRoot    string
	definitions []llm.Tool
}

// NewExploreTools creates tools for code exploration (Claude Code style).
func NewExploreTools(repoRoot string) *ExploreTools {
	t := &ExploreTools{
		repoRoot: repoRoot,
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
}

// bashBlockedPrefixes defines write operations that are always blocked.
var bashBlockedPrefixes = []string{
	"rm ", "mv ", "cp ", "mkdir ", "touch ", "chmod ", "chown ",
	"git push", "git commit", "git checkout", "git reset", "git rebase",
	"git merge", "git pull", "git stash", "git clean", "git add",
	"echo ", "printf ", "cat ", "head ", "tail ", "sed ", "awk ",
	"grep ", "rg ", // Use the grep tool instead
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
		return fmt.Sprintf("Command blocked: %s\n\nAllowed: git log/show/diff/blame/status, ls, tree, find\nUse 'read' tool for file contents, 'grep' tool for searching.", reason), nil
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
