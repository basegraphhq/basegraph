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
	ErrMissingGapCloseReason = errors.New("gap close missing reason")
	ErrMissingGapCloseNote   = errors.New("gap close missing note")
	ErrInvalidGapCloseReason = errors.New("invalid gap close reason")
	ErrMissingSynthesis      = errors.New("finding missing synthesis")
	ErrMissingSources        = errors.New("finding missing sources")
	ErrMissingSourceLocation = errors.New("source missing location")
	ErrMissingLearning       = errors.New("learning missing content")
	ErrInvalidLearningType   = errors.New("invalid learning type")
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

	// Track gap IDs that will be closed (resolved or skipped) anywhere in this batch.
	// This allows ready_for_plan to pass validation when blocking gaps are closed in the same batch,
	// regardless of action ordering.
	pendingClosures := make(map[string]struct{})
	for _, action := range input.Actions {
		if action.Type != ActionTypeUpdateGaps {
			continue
		}
		data, err := ParseActionData[UpdateGapsAction](action)
		if err != nil {
			continue
		}
		for _, id := range data.Resolve {
			pendingClosures[id] = struct{}{}
		}
		for _, id := range data.Skip {
			pendingClosures[id] = struct{}{}
		}
		for _, close := range data.Close {
			pendingClosures[close.GapID] = struct{}{}
		}
	}

	for i, action := range input.Actions {
		var err error
		switch action.Type {
		case ActionTypePostComment:
			err = v.validatePostComment(action)
		case ActionTypeUpdateFindings:
			err = v.validateUpdateFindings(action)
		case ActionTypeUpdateGaps:
			err = v.validateUpdateGaps(ctx, action)
		case ActionTypeUpdateLearnings:
			err = v.validateUpdateLearnings(action)
		case ActionTypeReadyForPlan:
			err = v.validateReadyForPlan(ctx, issue, action, pendingClosures)
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

func (v *actionValidator) validateUpdateGaps(ctx context.Context, action Action) error {
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
		_, err := v.lookupGapByAnyID(ctx, idStr)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("resolve[%d]: %w: %s", i, ErrGapNotFound, idStr)
			}
			return fmt.Errorf("resolve[%d]: %w", i, err)
		}
	}

	for i, idStr := range data.Skip {
		_, err := v.lookupGapByAnyID(ctx, idStr)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("skip[%d]: %w: %s", i, ErrGapNotFound, idStr)
			}
			return fmt.Errorf("skip[%d]: %w", i, err)
		}
	}

	for i, close := range data.Close {
		if close.GapID == "" {
			return fmt.Errorf("close[%d]: %w", i, ErrGapNotFound)
		}
		if !isValidGapCloseReason(close.Reason) {
			return fmt.Errorf("close[%d]: %w: %s", i, ErrInvalidGapCloseReason, close.Reason)
		}
		switch close.Reason {
		case GapCloseAnswered, GapCloseInferred:
			if close.Note == "" {
				return fmt.Errorf("close[%d]: %w", i, ErrMissingGapCloseNote)
			}
		}

		if _, err := v.lookupGapByAnyID(ctx, close.GapID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("close[%d]: %w: %s", i, ErrGapNotFound, close.GapID)
			}
			return fmt.Errorf("close[%d]: %w", i, err)
		}
	}

	return nil
}

func (v *actionValidator) validateUpdateLearnings(action Action) error {
	data, err := ParseActionData[UpdateLearningsAction](action)
	if err != nil {
		return err
	}

	for i, learning := range data.Propose {
		if learning.Content == "" {
			return fmt.Errorf("propose[%d]: %w", i, ErrMissingLearning)
		}
		if !isValidLearningType(learning.Type) {
			return fmt.Errorf("propose[%d]: %w: %s", i, ErrInvalidLearningType, learning.Type)
		}
	}

	return nil
}

func (v *actionValidator) validateReadyForPlan(ctx context.Context, issue model.Issue, action Action, pendingClosures map[string]struct{}) error {
	data, err := ParseActionData[ReadyForSpecAction](action)
	if err != nil {
		return err
	}

	// Get all open gaps and filter for blocking severity
	openGaps, err := v.gaps.ListOpenByIssue(ctx, issue.ID)
	if err != nil {
		return fmt.Errorf("checking blocking gaps: %w", err)
	}

	// Count blocking gaps that won't be closed by earlier actions in this batch
	remainingBlocking := 0
	for _, gap := range openGaps {
		if gap.Severity != model.GapSeverityBlocking {
			continue
		}
		primaryIDStr := strconv.FormatInt(gap.ID, 10)
		shortIDStr := strconv.FormatInt(gap.ShortID, 10)
		if _, willBeClosed := pendingClosures[primaryIDStr]; willBeClosed {
			continue
		}
		if _, willBeClosed := pendingClosures[shortIDStr]; !willBeClosed {
			remainingBlocking++
		}
	}

	if remainingBlocking > 0 {
		return fmt.Errorf("%w: %d blocking", ErrOpenBlockingGaps, remainingBlocking)
	}

	hasContext := len(data.ClosedGaps) > 0 || len(data.RelevantFindings) > 0
	if !hasContext {
		return ErrNoResolvedContext
	}

	return nil
}

func (v *actionValidator) lookupGapByAnyID(ctx context.Context, idStr string) (model.Gap, error) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return model.Gap{}, fmt.Errorf("invalid gap id: %s", idStr)
	}

	gap, err := v.gaps.GetByID(ctx, id)
	if err == nil {
		return gap, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return model.Gap{}, err
	}

	return v.gaps.GetByShortID(ctx, id)
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

func isValidLearningType(t string) bool {
	switch t {
	case model.LearningTypeProjectStandards, model.LearningTypeCodebaseStandards, model.LearningTypeDomainKnowledge:
		return true
	}
	return false
}

func isValidGapCloseReason(r GapCloseReason) bool {
	switch r {
	case GapCloseAnswered, GapCloseInferred, GapCloseNotRelevant:
		return true
	}
	return false
}
