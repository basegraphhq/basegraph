package model

import "time"

type Severity string

type DiscussionStatus string

type DiscussionType string

const (
	SeverityLow    Severity = "low"
	SeverityMedium Severity = "medium"
	SeverityHigh   Severity = "high"
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
	ID             string           `json:"id"`
	ExternalID     *string          `json:"external_id,omitempty"`
	Type           DiscussionType   `json:"type"`
	Author         string           `json:"author"`
	Body           string           `json:"body"`
	Category       string           `json:"category,omitempty"`
	Severity       Severity         `json:"severity,omitempty"`
	Status         DiscussionStatus `json:"status"`
	AskedAt        time.Time        `json:"asked_at"`
	ResolvedAt     *time.Time       `json:"resolved_at,omitempty"`
	ParentID       *string          `json:"parent_id,omitempty"`
	RelatedNoteID  *string          `json:"related_note_id,omitempty"`
	FollowUpNeeded bool             `json:"follow_up_needed,omitempty"`
}

type Issue struct {
	ID               int64         `json:"id"`
	IntegrationID    int64         `json:"integration_id"`
	ExternalIssueID  string        `json:"external_issue_id"`
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
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
}
