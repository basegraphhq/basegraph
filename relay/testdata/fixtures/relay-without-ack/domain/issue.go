package domain

import "time"

// Issue represents the Relay view of an issue enriched with findings.
type Issue struct {
	ID              int64         `json:"id"`
	IntegrationID   int64         `json:"integration_id"`
	ExternalIssueID string        `json:"external_issue_id"`
	Title           *string       `json:"title,omitempty"`
	Description     *string       `json:"description,omitempty"`
	Labels          []string      `json:"labels,omitempty"`
	Members         []string      `json:"members,omitempty"`
	Assignees       []string      `json:"assignees,omitempty"`
	Reporter        *string       `json:"reporter,omitempty"`
	Keywords        []Keyword     `json:"keywords,omitempty"`
	CodeFindings    []CodeFinding `json:"code_findings,omitempty"`
	Learnings       []Learning    `json:"learnings,omitempty"`
	Discussions     []Discussion  `json:"discussions,omitempty"`
	Spec            *string       `json:"spec,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}
