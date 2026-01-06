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

	for i, action := range input.Actions {
		var err error
		switch action.Type {
		case ActionTypePostComment:
			err = v.validatePostComment(action)
		case ActionTypeUpdateFindings:
			err = v.validateUpdateFindings(action)
		case ActionTypeUpdateGaps:
			err = v.validateUpdateGaps(ctx, issue, action)
		case ActionTypeReadyForSpecGeneration:
			err = v.validateReadyForSpecGeneration(ctx, issue, action)
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

	for i, close := range data.Close {
		if close.GapID == "" {
			return fmt.Errorf("close[%d]: %w", i, ErrGapNotFound)
		}
		id, err := strconv.ParseInt(close.GapID, 10, 64)
		if err != nil {
			return fmt.Errorf("close[%d]: invalid gap id: %s", i, close.GapID)
		}
		if _, err := v.gaps.GetByID(ctx, id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("close[%d]: %w: %s", i, ErrGapNotFound, close.GapID)
			}
			return fmt.Errorf("close[%d]: %w", i, err)
		}
	}

	return nil
}

func (v *actionValidator) validateReadyForSpecGeneration(ctx context.Context, issue model.Issue, action Action) error {
	data, err := ParseActionData[ReadyForSpecGenerationAction](action)
	if err != nil {
		return err
	}

	blockingCount, err := v.gaps.CountOpenBlocking(ctx, issue.ID)
	if err != nil {
		return fmt.Errorf("checking blocking gaps: %w", err)
	}
	if blockingCount > 0 {
		return fmt.Errorf("%w: %d blocking", ErrOpenBlockingGaps, blockingCount)
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
