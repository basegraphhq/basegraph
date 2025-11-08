package assistant

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	maxEntireFileBytes   = 200_000
	defaultReadLineLimit = 2000
	maxLineDisplayLength = 500
	defaultListOffset    = 1
	defaultListLimit     = 25
	defaultListDepth     = 2
	listIndentSpaces     = 2
)

type filesystemTools struct {
	root string
}

func newFilesystemTools(reg *ToolRegistry, root string) (*filesystemTools, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	info, statErr := os.Stat(absRoot)
	if statErr != nil {
		return nil, fmt.Errorf("inspect workspace root: %w", statErr)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace root must be a directory: %s", absRoot)
	}

	tools := &filesystemTools{root: absRoot}

	if err := reg.Add(tools.readEntireDefinition(), tools.handleReadEntireFile); err != nil {
		return nil, err
	}
	if err := reg.Add(tools.readPartialDefinition(), tools.handleReadPartialFile); err != nil {
		return nil, err
	}
	if err := reg.Add(tools.listDirectoryDefinition(), tools.handleListDirectory); err != nil {
		return nil, err
	}
	if err := reg.Add(tools.applyPatchDefinition(), tools.handleApplyPatch); err != nil {
		return nil, err
	}

	return tools, nil
}

func (f *filesystemTools) readEntireDefinition() ToolDefinition {
	return ToolDefinition{
		Name:        "read_entire_file",
		Description: "Read the full contents of a file (with safety limits) from the workspace.",
		Strict:      true,
		Parameters: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Absolute path or path relative to the workspace root.",
				},
				"max_bytes": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"description": "Optional override for maximum bytes to return (default 200k).",
				},
			},
			"required": []string{"file_path", "max_bytes"},
		},
	}
}

func (f *filesystemTools) readPartialDefinition() ToolDefinition {
	return ToolDefinition{
		Name:        "read_partial_file",
		Description: "Read a window of lines from a file with line numbers (1-indexed).",
		Strict:      true,
		Parameters: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Absolute path or path relative to the workspace root.",
				},
				"offset": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"description": "1-indexed line number to start reading from (default 1).",
				},
				"limit": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"maximum":     defaultReadLineLimit,
					"description": "Maximum number of lines to return (default 2000).",
				},
			},
			"required": []string{"file_path", "offset", "limit"},
		},
	}
}

func (f *filesystemTools) listDirectoryDefinition() ToolDefinition {
	return ToolDefinition{
		Name:        "list_directory",
		Description: "List directory entries breadth-first up to a specified depth.",
		Strict:      true,
		Parameters: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"dir_path": map[string]any{
					"type":        "string",
					"description": "Absolute path or path relative to the workspace root.",
				},
				"offset": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"description": "1-indexed entry to begin returning (default 1).",
				},
				"limit": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"description": "Maximum number of entries to include (default 25).",
				},
				"depth": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"description": "How many levels of subdirectories to traverse (default 2).",
				},
			},
			"required": []string{"dir_path", "offset", "limit", "depth"},
		},
	}
}

func (f *filesystemTools) applyPatchDefinition() ToolDefinition {
	return ToolDefinition{
		Name:        "apply_patch",
		Description: "Apply targeted edits to a file by replacing or creating content snippets.",
		Strict:      true,
		Parameters: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Absolute path or path relative to the workspace root.",
				},
				"operation": map[string]any{
					"type":        "string",
					"enum":        []string{"replace", "create", "delete"},
					"description": "Patch mode: replace (default), create (new file), or delete (remove file).",
				},
				"old_text": map[string]any{
					"type":        "string",
					"description": "Exact snippet to replace when operation is replace or delete.",
				},
				"new_text": map[string]any{
					"type":        "string",
					"description": "New content to insert (required for replace/create).",
				},
				"occurrence": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"description": "The nth occurrence of old_text to replace (default 1).",
				},
			},
			"required": []string{"file_path", "operation", "old_text", "new_text", "occurrence"},
		},
	}
}

type readEntireArgs struct {
	FilePath string `json:"file_path"`
	MaxBytes int    `json:"max_bytes"`
}

type readPartialArgs struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset"`
	Limit    int    `json:"limit"`
}

type listDirectoryArgs struct {
	DirPath string `json:"dir_path"`
	Offset  int    `json:"offset"`
	Limit   int    `json:"limit"`
	Depth   int    `json:"depth"`
}

type applyPatchArgs struct {
	FilePath   string `json:"file_path"`
	Operation  string `json:"operation"`
	OldText    string `json:"old_text"`
	NewText    string `json:"new_text"`
	Occurrence int    `json:"occurrence"`
}

func (f *filesystemTools) handleReadEntireFile(_ context.Context, raw json.RawMessage) (string, error) {
	var args readEntireArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	path, err := f.resolvePath(args.FilePath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}
	if info.IsDir() {
		return "", errors.New("file_path points to a directory")
	}
	limit := args.MaxBytes
	if limit <= 0 || limit > maxEntireFileBytes {
		limit = maxEntireFileBytes
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	reader := io.LimitReader(file, int64(limit))
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	truncated := int64(len(data)) < info.Size()

	response := map[string]any{
		"file_path": path,
		"bytes":     len(data),
		"truncated": truncated,
		"content":   string(data),
	}
	encoded, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode response: %w", err)
	}
	return string(encoded), nil
}

func (f *filesystemTools) handleReadPartialFile(_ context.Context, raw json.RawMessage) (string, error) {
	var args readPartialArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	path, err := f.resolvePath(args.FilePath)
	if err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	offset := args.Offset
	if offset <= 0 {
		offset = 1
	}
	limit := args.Limit
	if limit <= 0 || limit > defaultReadLineLimit {
		limit = defaultReadLineLimit
	}

	reader := bufio.NewReader(file)
	var (
		lines     []string
		lineNum   = 0
		truncated = false
	)
	for {
		line, err := reader.ReadString('\n')
		if errors.Is(err, io.EOF) && len(line) == 0 {
			break
		}
		lineNum++
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if lineNum < offset {
			if err != nil && !errors.Is(err, io.EOF) {
				return "", fmt.Errorf("read file: %w", err)
			}
			if errors.Is(err, io.EOF) {
				break
			}
			continue
		}
		if len(lines) < limit {
			lines = append(lines, fmt.Sprintf("L%d: %s", lineNum, shortenLine(line)))
		} else {
			truncated = true
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", fmt.Errorf("read file: %w", err)
		}
	}
	if lineNum < offset {
		return "", fmt.Errorf("offset %d exceeds file length", offset)
	}

	response := map[string]any{
		"file_path": path,
		"offset":    offset,
		"limit":     limit,
		"lines":     lines,
		"truncated": truncated,
	}
	encoded, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode response: %w", err)
	}
	return string(encoded), nil
}

func (f *filesystemTools) handleListDirectory(ctx context.Context, raw json.RawMessage) (string, error) {
	var args listDirectoryArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	path, err := f.resolvePath(args.DirPath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat directory: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("dir_path is not a directory")
	}

	offset := args.Offset
	if offset <= 0 {
		offset = defaultListOffset
	}
	limit := args.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	depth := args.Depth
	if depth <= 0 {
		depth = defaultListDepth
	}

	entries, err := f.collectEntries(ctx, path, depth)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		response := map[string]any{
			"dir_path":      path,
			"entries":       []any{},
			"offset":        offset,
			"limit":         limit,
			"depth":         depth,
			"has_more":      false,
			"total_entries": 0,
		}
		encoded, encErr := json.MarshalIndent(response, "", "  ")
		if encErr != nil {
			return "", fmt.Errorf("encode response: %w", encErr)
		}
		return string(encoded), nil
	}

	if offset > len(entries) {
		return "", fmt.Errorf("offset %d exceeds directory entry count (%d)", offset, len(entries))
	}

	start := offset - 1
	end := start + limit
	if end > len(entries) {
		end = len(entries)
	}
	visible := entries[start:end]
	resultEntries := make([]map[string]any, 0, len(visible))
	for _, entry := range visible {
		indent := strings.Repeat(" ", entry.depth*listIndentSpaces)
		display := indent + entry.display
		resultEntries = append(resultEntries, map[string]any{
			"relative_path": entry.relativePath,
			"display":       display,
			"kind":          entry.kind,
		})
	}

	response := map[string]any{
		"dir_path":      path,
		"entries":       resultEntries,
		"offset":        offset,
		"limit":         limit,
		"depth":         depth,
		"has_more":      end < len(entries),
		"total_entries": len(entries),
	}
	encoded, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode response: %w", err)
	}
	return string(encoded), nil
}

func (f *filesystemTools) collectEntries(ctx context.Context, root string, depth int) ([]dirEntry, error) {
	type queueItem struct {
		absPath        string
		relativePrefix string
		remainingDepth int
	}
	queue := []queueItem{{absPath: root, relativePrefix: "", remainingDepth: depth}}
	var out []dirEntry

	for len(queue) > 0 {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		item := queue[0]
		queue = queue[1:]

		entries, err := os.ReadDir(item.absPath)
		if err != nil {
			return nil, fmt.Errorf("read directory %s: %w", item.absPath, err)
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Name() < entries[j].Name()
		})

		for _, entry := range entries {
			if entry.Name() == "." || entry.Name() == ".." {
				continue
			}
			childRel := entry.Name()
			if item.relativePrefix != "" {
				childRel = filepath.Join(item.relativePrefix, entry.Name())
			}
			childAbs := filepath.Join(item.absPath, entry.Name())
			display := formatDirEntryDisplay(entry)
			kind := classifyDirEntry(entry)
			depthValue := strings.Count(childRel, string(os.PathSeparator))
			if entry.IsDir() && item.remainingDepth > 1 {
				queue = append(queue, queueItem{
					absPath:        childAbs,
					relativePrefix: childRel,
					remainingDepth: item.remainingDepth - 1,
				})
			}
			out = append(out, dirEntry{
				relativePath: filepath.ToSlash(childRel),
				display:      display,
				depth:        depthValue,
				kind:         kind,
			})
		}
	}

	return out, nil
}

func (f *filesystemTools) handleApplyPatch(_ context.Context, raw json.RawMessage) (string, error) {
	var args applyPatchArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	path, err := f.resolvePath(args.FilePath)
	if err != nil {
		return "", err
	}
	operation := strings.ToLower(strings.TrimSpace(args.Operation))
	if operation == "" {
		operation = "replace"
	}
	occurrence := args.Occurrence
	if occurrence <= 0 {
		occurrence = 1
	}

	var result map[string]any
	switch operation {
	case "replace":
		if args.OldText == "" {
			return "", errors.New("old_text is required for replace operations")
		}
		if args.NewText == "" {
			return "", errors.New("new_text is required for replace operations")
		}
		result, err = replaceInFile(path, args.OldText, args.NewText, occurrence)
	case "create":
		if args.NewText == "" {
			return "", errors.New("new_text is required for create operations")
		}
		result, err = createFile(path, args.NewText)
	case "delete":
		if args.OldText == "" {
			return "", errors.New("old_text is required for delete operations")
		}
		result, err = deleteSnippet(path, args.OldText, occurrence)
	default:
		return "", fmt.Errorf("unsupported operation %q", operation)
	}
	if err != nil {
		return "", err
	}
	result["file_path"] = path
	result["operation"] = operation
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode response: %w", err)
	}
	return string(encoded), nil
}

func replaceInFile(path, oldText, newText string, occurrence int) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	content := string(data)
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	index := -1
	start := 0
	for i := 0; i < occurrence; i++ {
		pos := strings.Index(content[start:], oldText)
		if pos < 0 {
			return nil, fmt.Errorf("old_text occurrence %d not found", occurrence)
		}
		index = start + pos
		start = index + len(oldText)
	}
	if index < 0 {
		return nil, fmt.Errorf("old_text occurrence %d not found", occurrence)
	}
	updated := content[:index] + newText + content[index+len(oldText):]
	perm := info.Mode().Perm()
	if err := os.WriteFile(path, []byte(updated), perm); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}
	return map[string]any{
		"replacements": 1,
	}, nil
}

func createFile(path, content string) (map[string]any, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create parent directories: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}
	return map[string]any{
		"created": true,
		"bytes":   len(content),
	}, nil
}

func deleteSnippet(path, snippet string, occurrence int) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	content := string(data)
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	index := -1
	start := 0
	for i := 0; i < occurrence; i++ {
		pos := strings.Index(content[start:], snippet)
		if pos < 0 {
			return nil, fmt.Errorf("snippet occurrence %d not found", occurrence)
		}
		index = start + pos
		start = index + len(snippet)
	}
	if index < 0 {
		return nil, fmt.Errorf("snippet occurrence %d not found", occurrence)
	}
	updated := content[:index] + content[index+len(snippet):]
	perm := info.Mode().Perm()
	if err := os.WriteFile(path, []byte(updated), perm); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}
	return map[string]any{
		"removed": 1,
	}, nil
}

type dirEntry struct {
	relativePath string
	display      string
	depth        int
	kind         string
}

func formatDirEntryDisplay(entry fs.DirEntry) string {
	name := entry.Name()
	if len([]rune(name)) > maxLineDisplayLength {
		name = string([]rune(name)[:maxLineDisplayLength]) + "..."
	}
	switch {
	case entry.Type()&os.ModeSymlink != 0:
		return name + "@"
	case entry.IsDir():
		return name + "/"
	case entry.Type().IsRegular():
		return name
	default:
		return name + "?"
	}
}

func classifyDirEntry(entry fs.DirEntry) string {
	switch {
	case entry.Type()&os.ModeSymlink != 0:
		return "symlink"
	case entry.IsDir():
		return "directory"
	case entry.Type().IsRegular():
		return "file"
	default:
		return "other"
	}
}

func shortenLine(line string) string {
	line = strings.ReplaceAll(line, "\t", strings.Repeat(" ", 4))
	runes := []rune(line)
	if len(runes) <= maxLineDisplayLength {
		return line
	}
	return string(runes[:maxLineDisplayLength]) + "..."
}

func (f *filesystemTools) resolvePath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("path must not be empty")
	}
	var abs string
	if filepath.IsAbs(trimmed) {
		abs = filepath.Clean(trimmed)
	} else {
		abs = filepath.Join(f.root, trimmed)
	}
	abs = filepath.Clean(abs)
	return abs, nil
}
