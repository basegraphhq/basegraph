package brain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"basegraph.app/relay/common/arangodb"
	"basegraph.app/relay/common/llm"
)

const (
	defaultCodeDepth = 1
	maxCodeResults   = 20    // Limit code tool results
	bashTimeout      = 10    // Bash command timeout in seconds
	maxBashOutput    = 10000 // Max bash output bytes (10KB - forces focused queries)
)

// Supported languages for CodeGraph.
var supportedLanguages = []string{"Go", "Python"}

// languageByExt maps file extensions to language names.
var languageByExt = map[string]string{
	".go":    "Go",
	".py":    "Python",
	".ts":    "TypeScript",
	".tsx":   "TypeScript",
	".js":    "JavaScript",
	".jsx":   "JavaScript",
	".rs":    "Rust",
	".java":  "Java",
	".rb":    "Ruby",
	".cpp":   "C++",
	".c":     "C",
	".cs":    "C#",
	".php":   "PHP",
	".kt":    "Kotlin",
	".swift": "Swift",
}

// CodegraphParams for the Codegraph tool.
type CodegraphParams struct {
	Operation string `json:"operation" jsonschema:"required,enum=find,enum=callers,enum=callees,enum=implementations,enum=methods,enum=usages,enum=symbols,description=Operation to perform"`
	Symbol    string `json:"symbol,omitempty" jsonschema:"description=Symbol name or pattern (e.g. 'Plan', '*Service*', 'Planner.Execute'). Required for all operations except 'symbols'."`
	File      string `json:"file,omitempty" jsonschema:"description=File path. Required for 'symbols' operation, optional filter for others."`
	Kind      string `json:"kind,omitempty" jsonschema:"description=Filter by kind: function, method, struct, interface"`
}

// BashParams for the Bash tool.
type BashParams struct {
	Command string `json:"command" jsonschema:"required,description=Bash command to execute (read-only commands only)"`
}

// ExploreTools provides bash and codegraph tools for the ExploreAgent.
type ExploreTools struct {
	repoRoot    string
	arango      arangodb.Client
	definitions []llm.Tool
}

// NewExploreTools creates tools for code exploration.
func NewExploreTools(repoRoot string, arango arangodb.Client) *ExploreTools {
	t := &ExploreTools{
		repoRoot: repoRoot,
		arango:   arango,
	}

	t.definitions = []llm.Tool{
		{
			Name: "bash",
			Description: `Execute read-only bash commands. Use ONLY when codegraph cannot help.

WHEN TO USE BASH:
  ✓ Reading specific code sections:  head -50 file.go, sed -n '100,150p' file.go
  ✓ Git history:                     git log --oneline -10, git blame file.go
  ✓ Quick text search:               rg -n "TODO" | head -20

WHEN TO USE CODEGRAPH INSTEAD:
  ✗ File overview        → codegraph(symbols, file="...")  NOT  head -200 file.go
  ✗ Finding symbols      → codegraph(find, symbol="...")   NOT  rg -n "SymbolName"
  ✗ Call graph           → codegraph(callers, symbol="...") ONLY codegraph can do this
  ✗ Type relationships   → codegraph(implementations/methods/usages)

ALWAYS limit bash output:
  rg -n "pattern" | head -30
  sed -n '1,50p' file.go  (NOT sed -n '1,500p')

Output limited to 10KB. Write operations blocked.`,
			Parameters: llm.GenerateSchemaFrom(BashParams{}),
		},
		{
			Name: "codegraph",
			Description: `Query the code graph for semantic relationships. Compiler-level accuracy.

OPERATIONS:
  find             - Find symbols by name/pattern
  callers          - Who calls this function?
  callees          - What does this function call?
  implementations  - What types implement this interface?
  methods          - What methods does this type have?
  usages           - Where is this type used as param/return?
  symbols          - All symbols in a file (with optional kind filter)

USE WHEN:
  - You need call graph traversal (bash can't do this)
  - You need type hierarchy (implementations, methods)
  - You need semantic understanding, not just text matching

EXAMPLES:
  codegraph(operation="callers", symbol="Execute")
  codegraph(operation="implementations", symbol="Store")
  codegraph(operation="symbols", file="internal/brain/planner.go")
  codegraph(operation="symbols", file="internal/model/issue.go", kind="struct")
  codegraph(operation="find", symbol="*Service*", kind="struct")

Supported languages: Go, Python. More coming soon.
Returns structured XML with hints for next steps.`,
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
	case "bash":
		return t.executeBash(ctx, arguments)
	case "codegraph":
		return t.executeCodegraph(ctx, arguments)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

// bashAllowedPrefixes defines read-only commands that are allowed.
var bashAllowedPrefixes = []string{
	// File reading
	"cat ", "head ", "tail ", "less ", "sed ",
	// Search
	"grep ", "rg ", "find ", "fd ", "ag ",
	// File info
	"wc ", "ls ", "ls", "file ", "stat ", "tree ",
	// Git read-only
	"git log", "git show", "git diff", "git blame", "git status",
	"git branch", "git tag", "git remote", "git grep",
}

// bashBlockedPrefixes defines write operations that are always blocked.
var bashBlockedPrefixes = []string{
	"rm ", "mv ", "cp ", "mkdir ", "touch ", "chmod ", "chown ",
	"git push", "git commit", "git checkout", "git reset", "git rebase",
	"git merge", "git pull", "git stash", "git clean",
	"echo ", "printf ",
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
		return fmt.Sprintf("Command blocked: %s\nAllowed: cat, head, tail, rg, grep, find, ls, tree, wc, git log/show/diff/blame/status", reason), nil
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
		return fmt.Sprintf("Command timed out after %d seconds. Use more specific patterns or add | head -N", bashTimeout), nil
	}

	// Handle other errors (but still return output if available)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Non-zero exit code - might just be "no matches" for grep
			if exitErr.ExitCode() == 1 && (strings.HasPrefix(command, "grep") || strings.HasPrefix(command, "rg")) {
				return "No matches found", nil
			}
		}
		// Return both error and any output
		if len(output) > 0 {
			return fmt.Sprintf("Command failed: %s\nOutput:\n%s", err, t.truncateBashOutput(output)), nil
		}
		return fmt.Sprintf("Command failed: %s", err), nil
	}

	result := t.truncateBashOutput(output)

	slog.DebugContext(ctx, "bash executed",
		"command", command,
		"output_len", len(output))

	return result, nil
}

// isBashCommandAllowed checks if a command is allowed based on prefix matching.
func (t *ExploreTools) isBashCommandAllowed(command string) (bool, string) {
	cmd := strings.TrimSpace(command)

	// Check blocked prefixes first (higher priority)
	for _, prefix := range bashBlockedPrefixes {
		if strings.HasPrefix(cmd, prefix) {
			return false, fmt.Sprintf("write operation '%s' not allowed", prefix)
		}
	}

	// Check for blocked patterns anywhere in command (redirects)
	if strings.Contains(cmd, " > ") || strings.Contains(cmd, " >> ") {
		return false, "output redirection not allowed"
	}

	// Validate paths in command stay within repo root
	if !t.validateBashPaths(cmd) {
		return false, "path outside repository"
	}

	// Check allowed prefixes
	for _, prefix := range bashAllowedPrefixes {
		if strings.HasPrefix(cmd, prefix) {
			return true, ""
		}
	}

	return false, "command not in allowed list"
}

// validateBashPaths checks that any absolute paths in the command are within repo root.
func (t *ExploreTools) validateBashPaths(command string) bool {
	absRoot, _ := filepath.Abs(t.repoRoot)

	parts := strings.Fields(command)
	for _, part := range parts {
		if strings.HasPrefix(part, "-") {
			continue
		}
		if strings.HasPrefix(part, "/") {
			absPath, err := filepath.Abs(part)
			if err != nil {
				continue
			}
			if !strings.HasPrefix(absPath, absRoot) {
				return false
			}
		}
	}
	return true
}

// truncateBashOutput limits output size and adds truncation message if needed.
// Also adds token estimate to help the model understand context cost.
func (t *ExploreTools) truncateBashOutput(output []byte) string {
	lineCount := strings.Count(string(output), "\n")

	if len(output) <= maxBashOutput {
		// Add token estimate even for non-truncated output
		tokenEstimate := len(output) / 4 // ~4 chars per token
		return string(output) + fmt.Sprintf("\n\n[~%d tokens added to context (%d lines)]", tokenEstimate, lineCount)
	}

	truncated := output[:maxBashOutput]
	// Try to truncate at a newline for cleaner output
	if lastNewline := strings.LastIndex(string(truncated), "\n"); lastNewline > maxBashOutput/2 {
		truncated = truncated[:lastNewline]
	}

	truncatedLineCount := strings.Count(string(truncated), "\n")
	tokenEstimate := len(truncated) / 4 // ~4 chars per token

	return string(truncated) + fmt.Sprintf("\n\n[Output truncated: showing %d of ~%d lines. ~%d tokens added. Use | head -N or more specific patterns.]",
		truncatedLineCount, lineCount, tokenEstimate)
}

// executeCodegraph handles the codegraph tool with XML output.
func (t *ExploreTools) executeCodegraph(ctx context.Context, arguments string) (string, error) {
	params, err := llm.ParseToolArguments[CodegraphParams](arguments)
	if err != nil {
		return "", fmt.Errorf("parse codegraph params: %w", err)
	}

	switch params.Operation {
	case "symbols":
		return t.executeSymbols(ctx, params)
	case "find":
		if params.Symbol == "" {
			return t.xmlError("missing_param", "'symbol' parameter required for find.", "Example: codegraph(operation=\"find\", symbol=\"Plan\")"), nil
		}
		return t.executeFind(ctx, params)
	case "callers", "callees", "implementations", "methods", "usages":
		if params.Symbol == "" {
			return t.xmlError("missing_param", fmt.Sprintf("'symbol' parameter required for %s.", params.Operation),
				fmt.Sprintf("Example: codegraph(operation=\"%s\", symbol=\"Execute\")", params.Operation)), nil
		}
		return t.executeRelationship(ctx, params)
	default:
		return t.xmlError("invalid_operation", fmt.Sprintf("Unknown operation \"%s\".", params.Operation),
			"Valid operations: find, callers, callees, implementations, methods, usages, symbols"), nil
	}
}

// executeFind handles the find operation.
func (t *ExploreTools) executeFind(ctx context.Context, params CodegraphParams) (string, error) {
	opts := arangodb.SearchOptions{
		Name: params.Symbol,
		Kind: params.Kind,
		File: params.File,
	}

	results, total, err := t.arango.SearchSymbols(ctx, opts)
	if err != nil {
		return t.xmlError("query_error", err.Error(), ""), nil
	}

	if total == 0 {
		return t.xmlSymbolNotFound(params.Symbol, params.File), nil
	}

	truncated := total > maxCodeResults
	if len(results) > maxCodeResults {
		results = results[:maxCodeResults]
	}

	var xml strings.Builder
	xml.WriteString(fmt.Sprintf("<code_result operation=\"find\" symbol=\"%s\">\n", escapeXML(params.Symbol)))
	xml.WriteString(fmt.Sprintf("  <results count=\"%d\"", len(results)))
	if truncated {
		xml.WriteString(fmt.Sprintf(" total=\"%d\" truncated=\"true\"", total))
	}
	xml.WriteString(">\n")

	for _, r := range results {
		xml.WriteString(fmt.Sprintf("    <symbol name=\"%s\" kind=\"%s\" file=\"%s\" line=\"%d\" qname=\"%s\"",
			escapeXML(r.Name), escapeXML(r.Kind), escapeXML(r.Filepath), r.Pos, escapeXML(r.QName)))
		if r.Signature != "" {
			xml.WriteString(fmt.Sprintf(" signature=\"%s\"", escapeXML(r.Signature)))
		}
		xml.WriteString(" />\n")
	}

	xml.WriteString("  </results>\n")

	if truncated {
		xml.WriteString(fmt.Sprintf("  <hint>Showing %d of %d results. Add kind or file filter to narrow.</hint>\n", len(results), total))
	}

	xml.WriteString("</code_result>")

	slog.DebugContext(ctx, "codegraph find executed",
		"symbol", params.Symbol,
		"results", len(results),
		"total", total)

	return withTokenEstimate(xml.String()), nil
}

// executeRelationship handles callers, callees, implementations, methods, usages.
func (t *ExploreTools) executeRelationship(ctx context.Context, params CodegraphParams) (string, error) {
	opts := arangodb.SearchOptions{
		Name: params.Symbol,
		Kind: params.Kind,
		File: params.File,
	}

	resolved, err := t.arango.ResolveSymbol(ctx, opts)
	if err != nil {
		var ambigErr arangodb.AmbiguousSymbolError
		if errors.As(err, &ambigErr) {
			return t.xmlAmbiguousSymbol(params.Symbol, ambigErr.Candidates), nil
		}
		if errors.Is(err, arangodb.ErrNotFound) {
			return t.xmlSymbolNotFound(params.Symbol, params.File), nil
		}
		return t.xmlError("query_error", err.Error(), ""), nil
	}

	// Execute the relationship query
	var nodes []arangodb.GraphNode
	var opErr error

	switch params.Operation {
	case "callers":
		nodes, opErr = t.arango.GetCallers(ctx, resolved.QName, defaultCodeDepth)
	case "callees":
		nodes, opErr = t.arango.GetCallees(ctx, resolved.QName, defaultCodeDepth)
	case "implementations":
		nodes, opErr = t.arango.GetImplementations(ctx, resolved.QName)
	case "methods":
		nodes, opErr = t.arango.GetMethods(ctx, resolved.QName)
	case "usages":
		nodes, opErr = t.arango.GetUsages(ctx, resolved.QName)
	}

	if opErr != nil {
		return t.xmlError("query_error", opErr.Error(), ""), nil
	}

	// Build XML output
	var xml strings.Builder
	xml.WriteString(fmt.Sprintf("<code_result operation=\"%s\" symbol=\"%s\">\n", params.Operation, escapeXML(params.Symbol)))

	// Resolved symbol
	xml.WriteString(fmt.Sprintf("  <resolved name=\"%s\" kind=\"%s\" file=\"%s\" line=\"%d\" qname=\"%s\"",
		escapeXML(resolved.Name), escapeXML(resolved.Kind), escapeXML(resolved.Filepath), resolved.Pos, escapeXML(resolved.QName)))
	if resolved.Signature != "" {
		xml.WriteString(fmt.Sprintf(" signature=\"%s\"", escapeXML(resolved.Signature)))
	}
	xml.WriteString(" />\n")

	// Results
	truncated := len(nodes) > maxCodeResults
	if truncated {
		nodes = nodes[:maxCodeResults]
	}

	xml.WriteString(fmt.Sprintf("  <results count=\"%d\"", len(nodes)))
	if truncated {
		xml.WriteString(" truncated=\"true\"")
	}
	xml.WriteString(">\n")

	for _, n := range nodes {
		xml.WriteString(fmt.Sprintf("    <symbol name=\"%s\" kind=\"%s\" file=\"%s\" line=\"%d\" qname=\"%s\"",
			escapeXML(n.Name), escapeXML(n.Kind), escapeXML(n.Filepath), n.Pos, escapeXML(n.QName)))
		if n.Signature != "" {
			xml.WriteString(fmt.Sprintf(" signature=\"%s\"", escapeXML(n.Signature)))
		}
		xml.WriteString(" />\n")
	}

	xml.WriteString("  </results>\n")

	// Hints
	if len(nodes) == 0 {
		xml.WriteString(fmt.Sprintf("  <hint>No %s found. This may be unused or called via reflection/external code.</hint>\n", params.Operation))
	} else if truncated {
		xml.WriteString(fmt.Sprintf("  <hint>Results truncated to %d. Add file filter to narrow.</hint>\n", maxCodeResults))
	}

	xml.WriteString("</code_result>")

	slog.DebugContext(ctx, "codegraph relationship executed",
		"operation", params.Operation,
		"symbol", params.Symbol,
		"resolved_qname", resolved.QName,
		"results", len(nodes))

	return withTokenEstimate(xml.String()), nil
}

// executeSymbols returns all symbols in a file.
func (t *ExploreTools) executeSymbols(ctx context.Context, params CodegraphParams) (string, error) {
	if params.File == "" {
		return t.xmlError("missing_param", "'file' parameter required for symbols.", "Example: codegraph(operation=\"symbols\", file=\"internal/brain/planner.go\")"), nil
	}

	// Check for unsupported language
	ext := strings.ToLower(filepath.Ext(params.File))
	if lang, ok := languageByExt[ext]; ok {
		supported := false
		for _, sl := range supportedLanguages {
			if sl == lang {
				supported = true
				break
			}
		}
		if !supported {
			return t.xmlUnsupportedLanguage(params.File, lang), nil
		}
	}

	symbols, err := t.arango.GetFileSymbols(ctx, arangodb.FileSymbolsOptions{
		Filepath: params.File,
		Kind:     params.Kind,
	})
	if err != nil {
		return t.xmlError("query_error", err.Error(), ""), nil
	}

	if len(symbols) == 0 {
		// Could be unsupported language or file not indexed
		return t.xmlEmptySymbols(params.File), nil
	}

	var xml strings.Builder
	if params.Kind != "" {
		xml.WriteString(fmt.Sprintf("<code_result operation=\"symbols\" file=\"%s\" kind=\"%s\">\n", escapeXML(params.File), escapeXML(params.Kind)))
	} else {
		xml.WriteString(fmt.Sprintf("<code_result operation=\"symbols\" file=\"%s\">\n", escapeXML(params.File)))
	}
	xml.WriteString(fmt.Sprintf("  <results count=\"%d\">\n", len(symbols)))

	for _, s := range symbols {
		xml.WriteString(fmt.Sprintf("    <symbol name=\"%s\" kind=\"%s\" line=\"%d\" qname=\"%s\"",
			escapeXML(s.Name), escapeXML(s.Kind), s.Pos, escapeXML(s.QName)))
		if s.Signature != "" {
			xml.WriteString(fmt.Sprintf(" signature=\"%s\"", escapeXML(s.Signature)))
		}
		xml.WriteString(" />\n")
	}

	xml.WriteString("  </results>\n")
	xml.WriteString("</code_result>")

	slog.DebugContext(ctx, "codegraph symbols executed",
		"file", params.File,
		"kind", params.Kind,
		"count", len(symbols))

	return withTokenEstimate(xml.String()), nil
}

// XML helper functions

func (t *ExploreTools) xmlError(errType, message, hint string) string {
	var xml strings.Builder
	xml.WriteString("<code_result>\n")
	xml.WriteString(fmt.Sprintf("  <error type=\"%s\">%s</error>\n", errType, escapeXML(message)))
	if hint != "" {
		xml.WriteString(fmt.Sprintf("  <hint>%s</hint>\n", escapeXML(hint)))
	}
	xml.WriteString("</code_result>")
	return xml.String()
}

func (t *ExploreTools) xmlSymbolNotFound(symbol, file string) string {
	var xml strings.Builder
	xml.WriteString("<code_result>\n")
	xml.WriteString(fmt.Sprintf("  <error type=\"symbol_not_found\">No symbol \"%s\" found.</error>\n", escapeXML(symbol)))
	xml.WriteString("  <hint>\n")
	xml.WriteString(fmt.Sprintf("    CodeGraph supports: %s. More languages coming soon.\n", strings.Join(supportedLanguages, ", ")))
	xml.WriteString(fmt.Sprintf("    Use bash for text search: rg -n \"%s\"\n", escapeXML(symbol)))
	xml.WriteString("  </hint>\n")
	xml.WriteString("</code_result>")
	return xml.String()
}

func (t *ExploreTools) xmlAmbiguousSymbol(symbol string, candidates []arangodb.SearchResult) string {
	var xml strings.Builder
	xml.WriteString(fmt.Sprintf("<code_result symbol=\"%s\">\n", escapeXML(symbol)))
	xml.WriteString(fmt.Sprintf("  <error type=\"ambiguous_symbol\">Multiple symbols match \"%s\".</error>\n", escapeXML(symbol)))
	xml.WriteString(fmt.Sprintf("  <candidates count=\"%d\">\n", len(candidates)))

	for _, c := range candidates {
		xml.WriteString(fmt.Sprintf("    <symbol name=\"%s\" kind=\"%s\" file=\"%s\" line=\"%d\" qname=\"%s\"",
			escapeXML(c.Name), escapeXML(c.Kind), escapeXML(c.Filepath), c.Pos, escapeXML(c.QName)))
		if c.Signature != "" {
			xml.WriteString(fmt.Sprintf(" signature=\"%s\"", escapeXML(c.Signature)))
		}
		xml.WriteString(" />\n")
	}

	xml.WriteString("  </candidates>\n")
	xml.WriteString("  <hint>Retry with file or kind filter to disambiguate.</hint>\n")
	xml.WriteString("</code_result>")
	return xml.String()
}

func (t *ExploreTools) xmlEmptySymbols(file string) string {
	ext := strings.ToLower(filepath.Ext(file))
	lang, known := languageByExt[ext]

	var xml strings.Builder
	xml.WriteString(fmt.Sprintf("<code_result operation=\"symbols\" file=\"%s\">\n", escapeXML(file)))
	xml.WriteString("  <results count=\"0\" />\n")
	xml.WriteString("  <hint>\n")

	if known {
		supported := false
		for _, sl := range supportedLanguages {
			if sl == lang {
				supported = true
				break
			}
		}
		if !supported {
			xml.WriteString(fmt.Sprintf("    CodeGraph does not support %s yet.\n", lang))
		} else {
			xml.WriteString("    No symbols found. File may not be indexed.\n")
		}
	} else {
		xml.WriteString("    No symbols found.\n")
	}

	xml.WriteString(fmt.Sprintf("    Supported: %s. More coming soon.\n", strings.Join(supportedLanguages, ", ")))
	xml.WriteString(fmt.Sprintf("    Use bash: rg -n \"func\\|type\\|class\" %s\n", escapeXML(file)))
	xml.WriteString("  </hint>\n")
	xml.WriteString("</code_result>")
	return xml.String()
}

func (t *ExploreTools) xmlUnsupportedLanguage(file, lang string) string {
	var xml strings.Builder
	xml.WriteString(fmt.Sprintf("<code_result operation=\"symbols\" file=\"%s\">\n", escapeXML(file)))
	xml.WriteString(fmt.Sprintf("  <error type=\"unsupported_language\">CodeGraph does not support %s yet.</error>\n", lang))
	xml.WriteString("  <hint>\n")
	xml.WriteString(fmt.Sprintf("    Supported: %s. More coming soon.\n", strings.Join(supportedLanguages, ", ")))
	xml.WriteString(fmt.Sprintf("    Use bash for %s:\n", lang))

	// Language-specific hints
	switch lang {
	case "TypeScript", "JavaScript":
		xml.WriteString(fmt.Sprintf("      rg -n \"function\\|class\\|interface\\|export\" %s\n", escapeXML(file)))
		xml.WriteString("      rg -n \"pattern\" --type ts\n")
	case "Rust":
		xml.WriteString(fmt.Sprintf("      rg -n \"fn\\|struct\\|impl\\|trait\" %s\n", escapeXML(file)))
		xml.WriteString("      rg -n \"pattern\" --type rust\n")
	case "Java":
		xml.WriteString(fmt.Sprintf("      rg -n \"class\\|interface\\|public.*void\\|public.*static\" %s\n", escapeXML(file)))
		xml.WriteString("      rg -n \"pattern\" --type java\n")
	default:
		xml.WriteString(fmt.Sprintf("      rg -n \"func\\|class\\|def\" %s\n", escapeXML(file)))
	}

	xml.WriteString("  </hint>\n")
	xml.WriteString("</code_result>")
	return xml.String()
}

// escapeXML escapes special characters for XML attributes and content.
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// withTokenEstimate appends a token cost estimate to codegraph output.
// Helps the model understand the context cost of each tool call.
func withTokenEstimate(xmlOutput string) string {
	tokenEstimate := len(xmlOutput) / 4 // ~4 chars per token
	return xmlOutput + fmt.Sprintf("\n\n[~%d tokens added to context]", tokenEstimate)
}
