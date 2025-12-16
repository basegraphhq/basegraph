package domain

import "time"

// Keyword represents a salient term extracted from the issue or payload.
type Keyword struct {
	Value     string    `json:"value"`
	Weight    float64   `json:"weight,omitempty"`
	Source    string    `json:"source,omitempty"`
	Extracted time.Time `json:"extracted_at"`
}

// CodeFinding represents a relevant code insight discovered via code graph traversal.
type CodeFinding struct {
	Summary          string      `json:"summary"`
	Severity         GapSeverity `json:"severity"`
	Sources          []string    `json:"sources,omitempty"`
	SuggestedActions []string    `json:"suggested_actions,omitempty"`
	DetectedAt       time.Time   `json:"detected_at"`
}

// Learning captures domain/project learnings leveraged when planning or detecting gaps.
type Learning struct {
	Text      string    `json:"text"`
	UpdatedBy string    `json:"updated_by"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ContextSnapshot aggregates the different finding sets stored on an issue.
type ContextSnapshot struct {
	Keywords     []Keyword     `json:"keywords,omitempty"`
	CodeFindings []CodeFinding `json:"code_findings,omitempty"`
	Learnings    []Learning    `json:"learnings,omitempty"`
	Discussions  []Discussion  `json:"discussions,omitempty"`
}
