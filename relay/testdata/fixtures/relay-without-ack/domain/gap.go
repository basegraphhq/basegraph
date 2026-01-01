package domain

import "time"

type GapStatus string

const (
	GapStatusOpen     GapStatus = "open"
	GapStatusResolved GapStatus = "resolved"
	GapStatusSkipped  GapStatus = "skipped"
)

type GapSeverity string

const (
	GapSeverityBlocking GapSeverity = "blocking"
	GapSeverityHigh     GapSeverity = "high"
	GapSeverityMedium   GapSeverity = "medium"
	GapSeverityLow      GapSeverity = "low"
)

type GapRespondent string

const (
	GapRespondentReporter GapRespondent = "reporter"
	GapRespondentAssignee GapRespondent = "assignee"
)

type Gap struct {
	ID         string        `json:"id"`
	IssueID    int64         `json:"issue_id"`
	Status     GapStatus     `json:"status"`
	Question   string        `json:"question"`
	Evidence   string        `json:"evidence,omitempty"`
	Severity   GapSeverity   `json:"severity"`
	Respondent GapRespondent `json:"respondent"`
	LearningID *string       `json:"learning_id,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
	ResolvedAt *time.Time    `json:"resolved_at,omitempty"`
}
