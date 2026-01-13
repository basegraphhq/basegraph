package model

import "time"

type Severity string

type DiscussionStatus string

type DiscussionType string

type ProcessingStatus string

const (
	SeverityLow    Severity = "low"
	SeverityMedium Severity = "medium"
	SeverityHigh   Severity = "high"
)

const (
	ProcessingStatusIdle       ProcessingStatus = "idle"
	ProcessingStatusQueued     ProcessingStatus = "queued"
	ProcessingStatusProcessing ProcessingStatus = "processing"
)

const (
	DiscussionStatusOpen      DiscussionStatus = "open"
	DiscussionStatusResolved  DiscussionStatus = "resolved"
	DiscussionStatusDismissed DiscussionStatus = "dismissed"
)

const (
	DiscussionTypeQuestion DiscussionType = "question"
	DiscussionTypeAnswer   DiscussionType = "answer"
	DiscussionTypeNote     DiscussionType = "note"
)

// CodeSource provides evidence grounding for a code finding.
// These are the actual code locations that support the synthesis.
type CodeSource struct {
	Location string `json:"location"`       // e.g., "internal/billing/service.go:42"
	Snippet  string `json:"snippet"`        // Actual code snippet
	Kind     string `json:"kind,omitempty"` // function, struct, interface, etc.
}

// CodeFinding represents the ExploreAgent's understanding of code context.
// Auto-persisted by ExploreAgent after each exploration.
// The Query field enables deduplication across planner runs.
type CodeFinding struct {
	// ID is a Snowflake ID for referencing this finding in actions (e.g., removal).
	ID string `json:"id"`

	// Query is the original exploration query that produced this finding.
	// Used for deduplication: similar queries return cached findings.
	Query string `json:"query"`

	// Synthesis is free-form prose describing what was found and understood.
	// Written like a senior engineer briefing the team - patterns, relationships,
	// constraints, gotchas, unknowns - all in natural language.
	Synthesis string `json:"synthesis"`

	// Sources provide evidence/grounding for the synthesis.
	// These are the actual code locations referenced.
	Sources []CodeSource `json:"sources"`

	// TokensUsed tracks the cost of this exploration (prompt + completion).
	TokensUsed int `json:"tokens_used,omitempty"`

	// CreatedAt enables staleness detection.
	// Findings older than a threshold may be re-explored.
	CreatedAt time.Time `json:"created_at"`
}

type Keyword struct {
	Value       string    `json:"value"`
	Weight      float64   `json:"weight"`
	Category    string    `json:"category"` // entity, concept, library
	Source      string    `json:"source"`
	ExtractedAt time.Time `json:"extracted_at"`
}

type Discussion struct {
	ExternalID string    `json:"external_id"`
	ThreadID   *string   `json:"thread_id,omitempty"`
	Author     string    `json:"author"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`

	Type           DiscussionType `json:"type,omitempty"`
	Category       string         `json:"category,omitempty"`
	Severity       Severity       `json:"severity,omitempty"`
	FollowUpNeeded bool           `json:"follow_up_needed,omitempty"`
}

type Issue struct {
	ID                int64         `json:"id"`
	IntegrationID     int64         `json:"integration_id"`
	ExternalProjectID *string       `json:"external_project_id,omitempty"`
	ExternalIssueID   string        `json:"external_issue_id"`
	Provider          Provider      `json:"provider"`
	Title             *string       `json:"title,omitempty"`
	Description       *string       `json:"description,omitempty"`
	Labels            []string      `json:"labels,omitempty"`
	Members           []string      `json:"members,omitempty"`
	Assignees         []string      `json:"assignees,omitempty"`
	Reporter          *string       `json:"reporter,omitempty"`
	ExternalIssueURL  *string       `json:"external_issue_url,omitempty"`
	Keywords          []Keyword     `json:"keywords,omitempty"`
	CodeFindings      []CodeFinding `json:"code_findings,omitempty"`
	Learnings         []Learning    `json:"learnings,omitempty"`
	Discussions       []Discussion  `json:"discussions,omitempty"`
	Spec              *string       `json:"spec,omitempty"`

	// Processing state for issue-centric queue handling
	ProcessingStatus    ProcessingStatus `json:"processing_status"`
	ProcessingStartedAt *time.Time       `json:"processing_started_at,omitempty"`
	LastProcessedAt     *time.Time       `json:"last_processed_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
