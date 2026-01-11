package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"basegraph.app/relay/internal/model"
)

func TestLocalSpecStore_WriteAndRead(t *testing.T) {
	tempDir := t.TempDir()

	store, err := NewLocalSpecStore(tempDir)
	if err != nil {
		t.Fatalf("NewLocalSpecStore failed: %v", err)
	}

	ctx := context.Background()
	content := "# Test Spec\n\nThis is a test spec."

	// Write spec
	ref, err := store.Write(ctx, 123, "gitlab", "456", "add-dark-mode", content)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify ref fields
	if ref.Version != 1 {
		t.Errorf("Version = %d, want 1", ref.Version)
	}
	if ref.Backend != "local" {
		t.Errorf("Backend = %s, want local", ref.Backend)
	}
	if ref.Format != "markdown" {
		t.Errorf("Format = %s, want markdown", ref.Format)
	}
	if ref.SHA256 == "" {
		t.Error("SHA256 should not be empty")
	}
	if !strings.Contains(ref.Path, "issue_123_gitlab_456_add-dark-mode") {
		t.Errorf("Path = %s, should contain issue identifier", ref.Path)
	}

	// Read back
	readContent, meta, err := store.Read(ctx, ref)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if readContent != content {
		t.Errorf("Read content = %q, want %q", readContent, content)
	}
	if meta.SHA256 != ref.SHA256 {
		t.Errorf("Meta SHA256 = %s, want %s", meta.SHA256, ref.SHA256)
	}
}

func TestLocalSpecStore_Exists(t *testing.T) {
	tempDir := t.TempDir()

	store, err := NewLocalSpecStore(tempDir)
	if err != nil {
		t.Fatalf("NewLocalSpecStore failed: %v", err)
	}

	ctx := context.Background()

	// Check non-existent
	exists, err := store.Exists(ctx, model.SpecRef{Path: "nonexistent/spec.md"})
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("Exists = true for nonexistent spec")
	}

	// Write and check exists
	ref, err := store.Write(ctx, 1, "gitlab", "1", "test", "content")
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	exists, err = store.Exists(ctx, ref)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Exists = false for existing spec")
	}
}

func TestLocalSpecStore_PathTraversal(t *testing.T) {
	tempDir := t.TempDir()

	store, err := NewLocalSpecStore(tempDir)
	if err != nil {
		t.Fatalf("NewLocalSpecStore failed: %v", err)
	}

	ctx := context.Background()

	// Try to read with path traversal
	_, _, err = store.Read(ctx, model.SpecRef{Path: "../../../etc/passwd"})
	if err != ErrSpecPathTraversal {
		t.Errorf("Read with traversal = %v, want ErrSpecPathTraversal", err)
	}

	// Try exists with path traversal
	_, err = store.Exists(ctx, model.SpecRef{Path: "foo/../../../bar"})
	if err != ErrSpecPathTraversal {
		t.Errorf("Exists with traversal = %v, want ErrSpecPathTraversal", err)
	}
}

func TestLocalSpecStore_EmptyContent(t *testing.T) {
	tempDir := t.TempDir()

	store, err := NewLocalSpecStore(tempDir)
	if err != nil {
		t.Fatalf("NewLocalSpecStore failed: %v", err)
	}

	ctx := context.Background()

	_, err = store.Write(ctx, 1, "gitlab", "1", "test", "")
	if err == nil {
		t.Error("Write with empty content should fail")
	}
}

func TestLocalSpecStore_TooLargeContent(t *testing.T) {
	tempDir := t.TempDir()

	store, err := NewLocalSpecStore(tempDir)
	if err != nil {
		t.Fatalf("NewLocalSpecStore failed: %v", err)
	}

	ctx := context.Background()

	// Create content larger than MaxSpecSize
	largeContent := strings.Repeat("x", MaxSpecSize+1)

	_, err = store.Write(ctx, 1, "gitlab", "1", "test", largeContent)
	if err != ErrSpecTooLarge {
		t.Errorf("Write with large content = %v, want ErrSpecTooLarge", err)
	}
}

func TestLocalSpecStore_AtomicWrite(t *testing.T) {
	tempDir := t.TempDir()

	store, err := NewLocalSpecStore(tempDir)
	if err != nil {
		t.Fatalf("NewLocalSpecStore failed: %v", err)
	}

	ctx := context.Background()
	content := "# Test Spec"

	ref, err := store.Write(ctx, 1, "gitlab", "1", "test", content)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify no .tmp file exists
	tmpPath := filepath.Join(tempDir, ref.Path+".tmp")
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("Temp file should not exist after successful write")
	}
}

func TestLocalSpecStore_ReadNotFound(t *testing.T) {
	tempDir := t.TempDir()

	store, err := NewLocalSpecStore(tempDir)
	if err != nil {
		t.Fatalf("NewLocalSpecStore failed: %v", err)
	}

	ctx := context.Background()

	_, _, err = store.Read(ctx, model.SpecRef{Path: "nonexistent/spec.md"})
	if err != ErrSpecNotFound {
		t.Errorf("Read nonexistent = %v, want ErrSpecNotFound", err)
	}
}

func TestLocalSpecStore_HashMismatch(t *testing.T) {
	tempDir := t.TempDir()

	store, err := NewLocalSpecStore(tempDir)
	if err != nil {
		t.Fatalf("NewLocalSpecStore failed: %v", err)
	}

	ctx := context.Background()

	ref, err := store.Write(ctx, 1, "gitlab", "1", "test", "content")
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Modify the hash
	ref.SHA256 = "invalid_hash"

	_, _, err = store.Read(ctx, ref)
	if err == nil {
		t.Error("Read with wrong hash should fail")
	}
	if !strings.Contains(err.Error(), "hash mismatch") {
		t.Errorf("Error = %v, should mention hash mismatch", err)
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Add Dark Mode Toggle", "add-dark-mode-toggle"},
		{"Fix: Bug in Login", "fix-bug-in-login"},
		{"Feature/User Authentication", "feature-user-authentication"},
		{"Hello   World", "hello-world"},
		{"---dashes---", "dashes"},
		{"", ""},
		{"1234", "1234"},
		{strings.Repeat("a", 100), strings.Repeat("a", 50)},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := slugify(tt.input)
			if got != tt.want {
				t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractSpecSummary(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		maxChars int
		want     string
	}{
		{
			name: "with TL;DR section",
			content: `# Spec

## TL;DR
- Point 1
- Point 2

## Problem Statement
Details here`,
			maxChars: 500,
			want:     "- Point 1\n- Point 2",
		},
		{
			name:     "without TL;DR",
			content:  "Some content that is long enough",
			maxChars: 10,
			want:     "Some conte...",
		},
		{
			name:     "short content",
			content:  "Short",
			maxChars: 100,
			want:     "Short",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractSpecSummary(tt.content, tt.maxChars)
			if got != tt.want {
				t.Errorf("ExtractSpecSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}
