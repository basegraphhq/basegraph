package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"basegraph.app/relay/internal/model"
)

type ActionType string

const (
	ActionTypePostComment            ActionType = "post_comment"
	ActionTypeUpdateFindings         ActionType = "update_findings"
	ActionTypeUpdateGaps             ActionType = "update_gaps"
	ActionTypeUpdateLearnings        ActionType = "update_learnings"
	ActionTypeReadyForSpecGeneration ActionType = "ready_for_spec_generation"
	ActionTypeSetSpecStatus          ActionType = "set_spec_status"
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
	Kind     string `json:"kind,omitempty"`
}

type UpdateGapsAction struct {
	Add   []GapInput `json:"add,omitempty"`
	Close []GapClose `json:"close,omitempty"`
	Ask   []GapID    `json:"ask,omitempty"` // promote pending gaps to open (asked)
}

type UpdateLearningsAction struct {
	Propose []LearningInput `json:"propose,omitempty"`
}

type LearningInput struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

type GapInput struct {
	Question   string      `json:"question"`
	Evidence   string      `json:"evidence,omitempty"`
	Severity   GapSeverity `json:"severity"`
	Respondent Respondent  `json:"respondent"`
	Pending    bool        `json:"pending,omitempty"` // true = store for later, false = mark as asked
}

type GapCloseReason string

const (
	GapCloseAnswered    GapCloseReason = "answered"
	GapCloseInferred    GapCloseReason = "inferred"
	GapCloseNotRelevant GapCloseReason = "not_relevant"
)

type GapID string

func (g *GapID) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*g = ""
		return nil
	}
	if len(b) == 0 {
		return fmt.Errorf("gap_id empty")
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*g = GapID(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*g = GapID(n.String())
	return nil
}

type GapClose struct {
	GapID  GapID          `json:"gap_id"`
	Reason GapCloseReason `json:"reason"`
	Note   string         `json:"note,omitempty"`
}

type GapSeverity string

const (
	GapSeverityBlocking GapSeverity = "blocking"
	GapSeverityHigh     GapSeverity = "high"
	GapSeverityMedium   GapSeverity = "medium"
	GapSeverityLow      GapSeverity = "low"
)

func (g *GapSeverity) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*g = ""
		return nil
	}

	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("severity must be a string: %w", err)
	}

	normalized := strings.ToLower(strings.TrimSpace(s))
	*g = GapSeverity(normalized)
	return nil
}

type Respondent string

const (
	RespondentReporter Respondent = "reporter"
	RespondentAssignee Respondent = "assignee"
)

type ReadyForSpecGenerationAction struct {
	ContextSummary   string   `json:"context_summary"`
	RelevantFindings []string `json:"relevant_finding_ids"`
	ClosedGaps       []GapID  `json:"closed_gap_ids"`
	LearningsApplied []string `json:"learning_ids"`
	ProceedSignal    string   `json:"proceed_signal"`
}

type SetSpecStatusAction struct {
	Status string `json:"status"`
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
