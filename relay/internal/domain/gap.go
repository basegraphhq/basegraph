package domain

import "time"

// GapCategory captures the type of gap detected by the planner or gap detector.
type GapCategory string

const (
	GapCategoryRequirement GapCategory = "requirement"
	GapCategoryCode        GapCategory = "code"
	GapCategoryDomain      GapCategory = "domain"
	GapCategoryAssumption  GapCategory = "assumption"
	GapCategoryRisk        GapCategory = "risk"
)

// GapSeverity represents the severity of a gap.
type GapSeverity string

const (
	GapSeverityLow    GapSeverity = "low"
	GapSeverityMedium GapSeverity = "medium"
	GapSeverityHigh   GapSeverity = "high"
)

// Gap represents an identified concern or missing piece of information.
type Gap struct {
	ID               string      `json:"id"`
	Category         GapCategory `json:"category"`
	Severity         GapSeverity `json:"severity"`
	Summary          string      `json:"summary"`
	Details          string      `json:"details,omitempty"`
	SuggestedActions []string    `json:"suggested_actions,omitempty"`
	Evidence         []string    `json:"evidence,omitempty"`
	RelatedEntities  []string    `json:"related_entities,omitempty"`
	DetectedAt       time.Time   `json:"detected_at"`
	ResolvedAt       *time.Time  `json:"resolved_at,omitempty"`
	ResolvedBy       *string     `json:"resolved_by,omitempty"`
	ResolutionNote   *string     `json:"resolution_note,omitempty"`
}

// GapAnalysis encapsulates the result of running the gap detector.
type GapAnalysis struct {
	Gaps         []Gap        `json:"gaps"`
	Questions    []Discussion `json:"questions"`
	ReadyForSpec bool         `json:"ready_for_spec"`
	Confidence   float64      `json:"confidence"`
	Notes        []string     `json:"notes,omitempty"`
}
