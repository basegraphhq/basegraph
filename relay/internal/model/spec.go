package model

import "time"

// SpecRef identifies a spec artifact stored in SpecStore.
// Serialized as JSON in issues.spec column for forward-compatibility.
type SpecRef struct {
	Version   int       `json:"version"`
	Backend   string    `json:"backend"` // "local", "s3", "gcs"
	Path      string    `json:"path"`    // relative path under configured root
	UpdatedAt time.Time `json:"updated_at"`
	SHA256    string    `json:"sha256"`
	Format    string    `json:"format"` // "markdown"
}

// SpecMeta contains metadata about a spec.
type SpecMeta struct {
	IssueID         int64
	ExternalIssueID string
	Title           string
	UpdatedAt       time.Time
	SHA256          string
}

// SpecComplexity represents the scale-adaptive depth level for specs.
// Based on BMAD-METHOD's L0-L4 complexity levels.
type SpecComplexity int

const (
	// SpecComplexityBugFix is for bug fixes - minimal spec (L0).
	SpecComplexityBugFix SpecComplexity = iota
	// SpecComplexitySmallFeature is for small features - light spec (L1).
	SpecComplexitySmallFeature
	// SpecComplexityMediumFeature is for medium features - standard spec (L2).
	SpecComplexityMediumFeature
	// SpecComplexityLargeFeature is for large features - full spec (L3).
	SpecComplexityLargeFeature
	// SpecComplexityArchitectural is for architectural changes - comprehensive spec (L4).
	SpecComplexityArchitectural
)

func (c SpecComplexity) String() string {
	switch c {
	case SpecComplexityBugFix:
		return "L0"
	case SpecComplexitySmallFeature:
		return "L1"
	case SpecComplexityMediumFeature:
		return "L2"
	case SpecComplexityLargeFeature:
		return "L3"
	case SpecComplexityArchitectural:
		return "L4"
	default:
		return "L2"
	}
}

// SpecStatus represents the current state of a spec.
type SpecStatus string

const (
	SpecStatusDraft    SpecStatus = "draft"
	SpecStatusInReview SpecStatus = "in_review"
	SpecStatusApproved SpecStatus = "approved"
	SpecStatusArchived SpecStatus = "archived"
)

// SpecMetadataJSON contains structured metadata for spec evaluation.
type SpecMetadataJSON struct {
	Sections        []string `json:"sections"`
	DecisionCount   int      `json:"decision_count"`
	AssumptionCount int      `json:"assumption_count"`
	TaskCount       int      `json:"task_count"`
	TestCaseCount   int      `json:"test_case_count"`
	CharCount       int      `json:"char_count"`
	SHA256          string   `json:"sha256"`
}

// ValidationError represents a spec validation failure.
type ValidationError struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"` // "error" or "warning"
	Detail   string `json:"detail,omitempty"`
}
