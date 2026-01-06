package brain

import (
	"context"
	"encoding/json"
	"fmt"

	"basegraph.app/relay/internal/model"
)

type ActionType string

const (
	ActionTypePostComment            ActionType = "post_comment"
	ActionTypeUpdateFindings         ActionType = "update_findings"
	ActionTypeUpdateGaps             ActionType = "update_gaps"
	ActionTypeReadyForSpecGeneration ActionType = "ready_for_spec_generation"
)

type Action struct {
	Type ActionType      `json:"type"`
	Data json.RawMessage `json:"data"`
}

// ParseData unmarshals the action's Data field into the specified type.
func ParseActionData[T any](action Action) (T, error) {
	var data T
	if err := json.Unmarshal(action.Data, &data); err != nil {
		return data, fmt.Errorf("parsing %s data: %w", action.Type, err)
	}
	return data, nil
}

type SubmitActionsInput struct {
	Actions   []Action `json:"actions"`
	Reasoning string   `json:"reasoning"`
}

type PostCommentAction struct {
	Content   string `json:"content"`
	ReplyToID string `json:"reply_to_id,omitempty"`
}

type UpdateFindingsAction struct {
	Add    []CodeFindingInput `json:"add,omitempty"`
	Remove []string           `json:"remove,omitempty"`
}

type CodeFindingInput struct {
	Synthesis string            `json:"synthesis"`
	Sources   []CodeSourceInput `json:"sources"`
}

type CodeSourceInput struct {
	Location string `json:"location"`
	Snippet  string `json:"snippet"`
	QName    string `json:"qname,omitempty"`
	Kind     string `json:"kind,omitempty"`
}

type UpdateGapsAction struct {
	Add   []GapInput `json:"add,omitempty"`
	Close []GapClose `json:"close,omitempty"`
}

type GapInput struct {
	Question   string      `json:"question"`
	Evidence   string      `json:"evidence,omitempty"`
	Severity   GapSeverity `json:"severity"`
	Respondent Respondent  `json:"respondent"`
}

type GapSeverity string

const (
	GapSeverityBlocking GapSeverity = "blocking"
	GapSeverityHigh     GapSeverity = "high"
	GapSeverityMedium   GapSeverity = "medium"
	GapSeverityLow      GapSeverity = "low"
)

type GapCloseReason string

const (
	GapCloseAnswered    GapCloseReason = "answered"
	GapCloseInferred    GapCloseReason = "inferred"
	GapCloseNotRelevant GapCloseReason = "not_relevant"
)

type GapClose struct {
	GapID  string         `json:"gap_id"`
	Reason GapCloseReason `json:"reason"`
	Note   string         `json:"note,omitempty"`
}

type Respondent string

const (
	RespondentReporter Respondent = "reporter"
	RespondentAssignee Respondent = "assignee"
)

type ReadyForSpecGenerationAction struct {
	ContextSummary   string   `json:"context_summary"`
	RelevantFindings []string `json:"relevant_finding_ids"`
	ResolvedGaps     []string `json:"resolved_gap_ids"`
	LearningsApplied []string `json:"learning_ids"`
}

// ActionExecutor executes actions returned by the Planner.
type ActionExecutor interface {
	Execute(ctx context.Context, issue model.Issue, action Action) error
	ExecuteBatch(ctx context.Context, issue model.Issue, actions []Action) []ActionError
}

// ActionError represents a failed action execution.
type ActionError struct {
	Action      Action
	Error       string
	Recoverable bool
}

// ActionValidator validates actions before execution.
type ActionValidator interface {
	Validate(ctx context.Context, issue model.Issue, input SubmitActionsInput) error
}
