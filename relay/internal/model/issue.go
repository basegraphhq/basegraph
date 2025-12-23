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

type CodeFinding struct {
	Finding          string   `json:"finding"`
	Severity         Severity `json:"severity"`
	Sources          []string `json:"sources"`
	SuggestedActions []string `json:"suggested_actions"`
}

type Discussion struct {
	ExternalID string    `json:"external_id"`
	ThreadID   *string   `json:"thread_id,omitempty"`
	ParentID   *string   `json:"parent_id,omitempty"`
	Author     string    `json:"author"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`

	Type           DiscussionType `json:"type,omitempty"`
	Category       string         `json:"category,omitempty"`
	Severity       Severity       `json:"severity,omitempty"`
	FollowUpNeeded bool           `json:"follow_up_needed,omitempty"`
}

type Issue struct {
	ID               int64         `json:"id"`
	IntegrationID    int64         `json:"integration_id"`
	ExternalIssueID  string        `json:"external_issue_id"`
	Provider         Provider      `json:"provider"`
	Title            *string       `json:"title,omitempty"`
	Description      *string       `json:"description,omitempty"`
	Labels           []string      `json:"labels,omitempty"`
	Members          []string      `json:"members,omitempty"`
	Assignees        []string      `json:"assignees,omitempty"`
	Reporter         *string       `json:"reporter,omitempty"`
	ExternalIssueURL *string       `json:"external_issue_url,omitempty"`
	Keywords         []string      `json:"keywords,omitempty"`
	CodeFindings     []CodeFinding `json:"code_findings,omitempty"`
	Learnings        []Learning    `json:"learnings,omitempty"`
	Discussions      []Discussion  `json:"discussions,omitempty"`
	Spec             *string       `json:"spec,omitempty"`

	// Processing state for issue-centric queue handling
	ProcessingStatus    ProcessingStatus `json:"processing_status"`
	ProcessingStartedAt *time.Time       `json:"processing_started_at,omitempty"`
	LastProcessedAt     *time.Time       `json:"last_processed_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
