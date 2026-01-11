package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"basegraph.app/relay/internal/model"
)

const (
	// MaxSpecSize is the maximum allowed spec size in bytes.
	MaxSpecSize = 200 * 1024 // 200KB

	// specFilename is the canonical filename for specs.
	specFilename = "spec.md"
)

var (
	ErrSpecNotFound      = errors.New("spec not found")
	ErrSpecTooLarge      = errors.New("spec exceeds maximum size")
	ErrInvalidSpecPath   = errors.New("invalid spec path")
	ErrSpecPathTraversal = errors.New("path traversal not allowed")
)

// SpecStore provides read/write operations for spec artifacts.
type SpecStore interface {
	// Read retrieves a spec by its reference.
	Read(ctx context.Context, ref model.SpecRef) (content string, meta model.SpecMeta, err error)

	// Write stores a spec and returns its reference.
	Write(ctx context.Context, issueID int64, provider, externalIssueID, slug, content string) (ref model.SpecRef, err error)

	// Exists checks if a spec exists at the given reference.
	Exists(ctx context.Context, ref model.SpecRef) (bool, error)
}

// LocalSpecStore implements SpecStore using the local filesystem.
type LocalSpecStore struct {
	rootDir string
}

// NewLocalSpecStore creates a LocalSpecStore with the given root directory.
func NewLocalSpecStore(rootDir string) (*LocalSpecStore, error) {
	if rootDir == "" {
		return nil, fmt.Errorf("spec root directory is required")
	}

	// Ensure root directory exists
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating spec root directory: %w", err)
	}

	return &LocalSpecStore{rootDir: rootDir}, nil
}

// Read retrieves a spec by its reference.
func (s *LocalSpecStore) Read(ctx context.Context, ref model.SpecRef) (string, model.SpecMeta, error) {
	if err := s.validatePath(ref.Path); err != nil {
		return "", model.SpecMeta{}, err
	}

	fullPath := filepath.Join(s.rootDir, ref.Path)

	content, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", model.SpecMeta{}, ErrSpecNotFound
		}
		return "", model.SpecMeta{}, fmt.Errorf("reading spec: %w", err)
	}

	// Verify SHA256 if provided
	if ref.SHA256 != "" {
		actualHash := sha256Hash(content)
		if actualHash != ref.SHA256 {
			return "", model.SpecMeta{}, fmt.Errorf("spec hash mismatch: expected %s, got %s", ref.SHA256, actualHash)
		}
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return "", model.SpecMeta{}, fmt.Errorf("stat spec: %w", err)
	}

	meta := model.SpecMeta{
		UpdatedAt: info.ModTime(),
		SHA256:    sha256Hash(content),
	}

	return string(content), meta, nil
}

// Write stores a spec and returns its reference.
func (s *LocalSpecStore) Write(ctx context.Context, issueID int64, provider, externalIssueID, slug, content string) (model.SpecRef, error) {
	if len(content) > MaxSpecSize {
		return model.SpecRef{}, ErrSpecTooLarge
	}

	if len(content) == 0 {
		return model.SpecRef{}, fmt.Errorf("spec content cannot be empty")
	}

	// Build directory path: issue_{id}_{provider}_{extID}_{slug}/
	sanitizedSlug := slugify(slug)
	if sanitizedSlug == "" {
		sanitizedSlug = "spec"
	}

	dirName := fmt.Sprintf("issue_%d_%s_%s_%s", issueID, provider, externalIssueID, sanitizedSlug)
	relPath := filepath.Join(dirName, specFilename)

	if err := s.validatePath(relPath); err != nil {
		return model.SpecRef{}, err
	}

	fullDir := filepath.Join(s.rootDir, dirName)
	fullPath := filepath.Join(s.rootDir, relPath)

	// Create directory
	if err := os.MkdirAll(fullDir, 0o755); err != nil {
		return model.SpecRef{}, fmt.Errorf("creating spec directory: %w", err)
	}

	// Atomic write: write to temp file, then rename
	tmpPath := fullPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0o644); err != nil {
		return model.SpecRef{}, fmt.Errorf("writing temp spec: %w", err)
	}

	if err := os.Rename(tmpPath, fullPath); err != nil {
		// Clean up temp file on rename failure
		os.Remove(tmpPath)
		return model.SpecRef{}, fmt.Errorf("renaming spec: %w", err)
	}

	hash := sha256Hash([]byte(content))
	now := time.Now().UTC()

	return model.SpecRef{
		Version:   1,
		Backend:   "local",
		Path:      relPath,
		UpdatedAt: now,
		SHA256:    hash,
		Format:    "markdown",
	}, nil
}

// Exists checks if a spec exists at the given reference.
func (s *LocalSpecStore) Exists(ctx context.Context, ref model.SpecRef) (bool, error) {
	if err := s.validatePath(ref.Path); err != nil {
		return false, err
	}

	fullPath := filepath.Join(s.rootDir, ref.Path)
	_, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking spec existence: %w", err)
	}
	return true, nil
}

// validatePath ensures the path is safe (no traversal, stays under root).
func (s *LocalSpecStore) validatePath(path string) error {
	if path == "" {
		return ErrInvalidSpecPath
	}

	// Check for path traversal attempts
	if strings.Contains(path, "..") {
		return ErrSpecPathTraversal
	}

	// Ensure it's not an absolute path
	if filepath.IsAbs(path) {
		return ErrSpecPathTraversal
	}

	// Clean and verify the path stays under root
	cleaned := filepath.Clean(path)
	if strings.HasPrefix(cleaned, "..") {
		return ErrSpecPathTraversal
	}

	return nil
}

// sha256Hash computes SHA256 hash of content.
func sha256Hash(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}

// slugify converts a title to a URL-safe slug.
var slugRegex = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	// Convert to lowercase
	s = strings.ToLower(s)

	// Replace non-alphanumeric with hyphens
	s = slugRegex.ReplaceAllString(s, "-")

	// Trim leading/trailing hyphens
	s = strings.Trim(s, "-")

	// Truncate to reasonable length
	if len(s) > 50 {
		s = s[:50]
		// Don't end with hyphen after truncation
		s = strings.TrimRight(s, "-")
	}

	// Remove any non-ASCII characters that slipped through
	result := strings.Builder{}
	for _, r := range s {
		if r <= unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-') {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// ExtractSpecSummary extracts a summary from spec markdown for context builder.
// Returns the TL;DR section if present, otherwise first N characters.
func ExtractSpecSummary(content string, maxChars int) string {
	// Try to find TL;DR section
	tldrMarker := "## TL;DR"
	if idx := strings.Index(content, tldrMarker); idx != -1 {
		start := idx + len(tldrMarker)
		// Find the next section (##)
		rest := content[start:]
		if endIdx := strings.Index(rest[1:], "\n##"); endIdx != -1 {
			return strings.TrimSpace(rest[:endIdx+1])
		}
		// No next section, take up to maxChars
		if len(rest) > maxChars {
			rest = rest[:maxChars] + "..."
		}
		return strings.TrimSpace(rest)
	}

	// Fall back to first N chars
	if len(content) > maxChars {
		return content[:maxChars] + "..."
	}
	return content
}
