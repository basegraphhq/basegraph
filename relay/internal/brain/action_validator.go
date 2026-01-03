package brain

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/store"
)

const (
	maxCommentLength = 65000
	minCommentLength = 1
)

var (
	ErrEmptyActions          = errors.New("no actions provided")
	ErrUnknownActionType     = errors.New("unknown action type")
	ErrContentTooShort       = errors.New("content too short")
	ErrContentTooLong        = errors.New("content too long")
	ErrMissingQuestion       = errors.New("gap missing question")
	ErrInvalidSeverity       = errors.New("invalid gap severity")
	ErrInvalidRespondent     = errors.New("invalid gap respondent")
	ErrGapNotFound           = errors.New("gap not found")
	ErrMissingSynthesis      = errors.New("finding missing synthesis")
	ErrMissingSources        = errors.New("finding missing sources")
	ErrMissingSourceLocation = errors.New("source missing location")
	ErrOpenBlockingGaps      = errors.New("open blocking gaps exist")
	ErrNoResolvedContext     = errors.New("no resolved gaps or findings")
)

type actionValidator struct {
	gaps store.GapStore
}

func NewActionValidator(gaps store.GapStore) ActionValidator {
	return &actionValidator{gaps: gaps}
}

func (v *actionValidator) Validate(ctx context.Context, issue model.Issue, input SubmitActionsInput) error {
	if len(input.Actions) == 0 {
		return ErrEmptyActions
	}

	// Track gap IDs that will be resolved by earlier actions in this batch.
	// This allows ready_for_plan to pass validation when blocking gaps
	// are resolved in the same batch.
	pendingResolutions := make(map[string]struct{})

	for i, action := range input.Actions {
		var err error
		switch action.Type {
		case ActionTypePostComment:
			err = v.validatePostComment(action)
		case ActionTypeUpdateFindings:
			err = v.validateUpdateFindings(action)
		case ActionTypeUpdateGaps:
			err = v.validateUpdateGaps(ctx, issue, action)
			if err == nil {
				// Track resolved gaps for later ready_for_plan validation
				data, _ := ParseActionData[UpdateGapsAction](action)
				for _, id := range data.Resolve {
					pendingResolutions[id] = struct{}{}
				}
			}
		case ActionTypeReadyForPlan:
			err = v.validateReadyForPlan(ctx, issue, action, pendingResolutions)
		default:
			err = fmt.Errorf("%w: %s", ErrUnknownActionType, action.Type)
		}
		if err != nil {
			return fmt.Errorf("action[%d] %s: %w", i, action.Type, err)
		}
	}

	return nil
}

func (v *actionValidator) validatePostComment(action Action) error {
	data, err := ParseActionData[PostCommentAction](action)
	if err != nil {
		return err
	}

	if len(data.Content) < minCommentLength {
		return ErrContentTooShort
	}
	if len(data.Content) > maxCommentLength {
		return ErrContentTooLong
	}

	return nil
}

func (v *actionValidator) validateUpdateFindings(action Action) error {
	data, err := ParseActionData[UpdateFindingsAction](action)
	if err != nil {
		return err
	}

	for i, finding := range data.Add {
		if finding.Synthesis == "" {
			return fmt.Errorf("add[%d]: %w", i, ErrMissingSynthesis)
		}
		if len(finding.Sources) == 0 {
			return fmt.Errorf("add[%d]: %w", i, ErrMissingSources)
		}
		for j, source := range finding.Sources {
			if source.Location == "" {
				return fmt.Errorf("add[%d].sources[%d]: %w", i, j, ErrMissingSourceLocation)
			}
		}
	}

	return nil
}

func (v *actionValidator) validateUpdateGaps(ctx context.Context, issue model.Issue, action Action) error {
	data, err := ParseActionData[UpdateGapsAction](action)
	if err != nil {
		return err
	}

	for i, gap := range data.Add {
		if gap.Question == "" {
			return fmt.Errorf("add[%d]: %w", i, ErrMissingQuestion)
		}
		if !isValidSeverity(gap.Severity) {
			return fmt.Errorf("add[%d]: %w: %s", i, ErrInvalidSeverity, gap.Severity)
		}
		if !isValidRespondent(gap.Respondent) {
			return fmt.Errorf("add[%d]: %w: %s", i, ErrInvalidRespondent, gap.Respondent)
		}
	}

	for i, idStr := range data.Resolve {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return fmt.Errorf("resolve[%d]: invalid gap id: %s", i, idStr)
		}
		if _, err := v.gaps.GetByID(ctx, id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("resolve[%d]: %w: %s", i, ErrGapNotFound, idStr)
			}
			return fmt.Errorf("resolve[%d]: %w", i, err)
		}
	}

	for i, idStr := range data.Skip {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return fmt.Errorf("skip[%d]: invalid gap id: %s", i, idStr)
		}
		if _, err := v.gaps.GetByID(ctx, id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("skip[%d]: %w: %s", i, ErrGapNotFound, idStr)
			}
			return fmt.Errorf("skip[%d]: %w", i, err)
		}
	}

	return nil
}

func (v *actionValidator) validateReadyForPlan(ctx context.Context, issue model.Issue, action Action, pendingResolutions map[string]struct{}) error {
	data, err := ParseActionData[ReadyForSpecAction](action)
	if err != nil {
		return err
	}

	// Get all open gaps and filter for blocking severity
	openGaps, err := v.gaps.ListOpenByIssue(ctx, issue.ID)
	if err != nil {
		return fmt.Errorf("checking blocking gaps: %w", err)
	}

	// Count blocking gaps that won't be resolved by earlier actions in this batch
	remainingBlocking := 0
	for _, gap := range openGaps {
		if gap.Severity != model.GapSeverityBlocking {
			continue
		}
		idStr := strconv.FormatInt(gap.ID, 10)
		if _, willBeResolved := pendingResolutions[idStr]; !willBeResolved {
			remainingBlocking++
		}
	}

	if remainingBlocking > 0 {
		return fmt.Errorf("%w: %d blocking", ErrOpenBlockingGaps, remainingBlocking)
	}

	hasContext := len(data.ResolvedGaps) > 0 || len(data.RelevantFindings) > 0
	if !hasContext {
		return ErrNoResolvedContext
	}

	return nil
}

func isValidSeverity(s GapSeverity) bool {
	switch s {
	case GapSeverityBlocking, GapSeverityHigh, GapSeverityMedium, GapSeverityLow:
		return true
	}
	return false
}

func isValidRespondent(r Respondent) bool {
	switch r {
	case RespondentReporter, RespondentAssignee:
		return true
	}
	return false
}
