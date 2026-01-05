package model

import "time"

type Learning struct {
	ID                   int64     `json:"id"`
	ShortID              int64     `json:"short_id"`
	WorkspaceID          int64     `json:"workspace_id"`
	RuleUpdatedByIssueID *int64    `json:"rule_updated_by_issue_id,omitempty"`
	Type                 string    `json:"type"`
	Content              string    `json:"content"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

const (
	LearningTypeProjectStandards  = "project_standards"
	LearningTypeCodebaseStandards = "codebase_standards"
	LearningTypeDomainKnowledge   = "domain_knowledge"
)
