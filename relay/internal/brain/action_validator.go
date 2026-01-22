package brain

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"basegraph.co/relay/internal/model"
	"basegraph.co/relay/internal/store"
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
	ErrGapNotPending         = errors.New("gap is not pending")
	ErrMissingGapCloseReason = errors.New("gap close missing reason")
	ErrMissingGapCloseNote   = errors.New("gap close missing note")
	ErrInvalidGapCloseReason = errors.New("invalid gap close reason")
	ErrMissingSynthesis      = errors.New("finding missing synthesis")
	ErrMissingSources        = errors.New("finding missing sources")
	ErrMissingSourceLocation = errors.New("source missing location")
	ErrMissingLearning       = errors.New("learning missing content")
	ErrInvalidLearningType   = errors.New("invalid learning type")
	ErrOpenBlockingGaps      = errors.New("open blocking gaps exist")
	ErrOpenGaps              = errors.New("open gaps exist")
	ErrNoResolvedContext     = errors.New("no resolved gaps or findings")
	ErrMissingProceedSignal  = errors.New("proceed signal not provided")
	ErrInvalidSpecStatus     = errors.New("invalid spec status: use \"approved\" or \"rejected\"")
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

	// Track gap IDs that will be closed anywhere in this batch.
	// This allows ready_for_spec_generation to pass validation when blocking gaps are closed in the same batch,
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
		for _, close := range data.Close {
			pendingClosures[string(close.GapID)] = struct{}{}
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
		case ActionTypeReadyForSpecGeneration:
			err = v.validateReadyForSpecGeneration(ctx, issue, action, pendingClosures)
		case ActionTypeSetSpecStatus:
			err = v.validateSetSpecStatus(action)
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

	for i, close := range data.Close {
		if string(close.GapID) == "" {
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

		if _, err := v.lookupGapByAnyID(ctx, string(close.GapID)); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("close[%d]: %w: %s", i, ErrGapNotFound, string(close.GapID))
			}
			return fmt.Errorf("close[%d]: %w", i, err)
		}
	}

	// Validate Ask gap IDs - must exist and be in pending status
	for i, gapID := range data.Ask {
		if string(gapID) == "" {
			return fmt.Errorf("ask[%d]: %w", i, ErrGapNotFound)
		}

		gap, err := v.lookupGapByAnyID(ctx, string(gapID))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("ask[%d]: %w: %s", i, ErrGapNotFound, string(gapID))
			}
			return fmt.Errorf("ask[%d]: %w", i, err)
		}
		if gap.Status != model.GapStatusPending {
			return fmt.Errorf("ask[%d]: %w: %s (status: %s)", i, ErrGapNotPending, string(gapID), gap.Status)
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

func (v *actionValidator) validateSetSpecStatus(action Action) error {
	data, err := ParseActionData[SetSpecStatusAction](action)
	if err != nil {
		return err
	}

	switch data.Status {
	case string(model.SpecStatusApproved), string(model.SpecStatusRejected):
		return nil
	default:
		return ErrInvalidSpecStatus
	}
}

func (v *actionValidator) validateReadyForSpecGeneration(ctx context.Context, issue model.Issue, action Action, pendingClosures map[string]struct{}) error {
	data, err := ParseActionData[ReadyForSpecGenerationAction](action)
	if err != nil {
		return err
	}
	if strings.TrimSpace(data.ProceedSignal) == "" {
		return ErrMissingProceedSignal
	}
	// Trust model's judgment - proceed_signal is for audit trail only

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

	remainingOpen := 0
	for _, gap := range openGaps {
		primaryIDStr := strconv.FormatInt(gap.ID, 10)
		shortIDStr := strconv.FormatInt(gap.ShortID, 10)
		if _, willBeClosed := pendingClosures[primaryIDStr]; willBeClosed {
			continue
		}
		if _, willBeClosed := pendingClosures[shortIDStr]; willBeClosed {
			continue
		}
		remainingOpen++
	}

	if remainingOpen > 0 {
		return fmt.Errorf("%w: %d open", ErrOpenGaps, remainingOpen)
	}

	hasContext := len(data.ClosedGaps) > 0 || len(data.RelevantFindings) > 0
	if !hasContext {
		return ErrNoResolvedContext
	}

	return nil
}

func (v *actionValidator) lookupGapByAnyID(ctx context.Context, idStr string) (model.Gap, error) {
	// Normalize: strip common LLM mistakes like "gap " prefix
	normalized := strings.TrimSpace(idStr)
	normalized = strings.TrimPrefix(normalized, "gap ")
	normalized = strings.TrimPrefix(normalized, "Gap ")

	id, err := strconv.ParseInt(normalized, 10, 64)
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
	case model.LearningTypeDomainLearnings, model.LearningTypeCodeLearnings:
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

// FormatValidationErrorForLLM converts a validation error into an actionable
// message that helps the LLM understand and fix the issue.
func FormatValidationErrorForLLM(err error) string {
	var sb strings.Builder
	sb.WriteString("Action validation failed. Fix the issues and call submit_actions again:\n\n")
	sb.WriteString(err.Error())
	sb.WriteString("\n\nHints:\n")

	errStr := err.Error()

	if strings.Contains(errStr, "invalid gap id") {
		sb.WriteString("- Gap ID must be numeric only (e.g., \"220\", not \"gap 220\")\n")
		sb.WriteString("- Use the number from [gap N] format, not the full bracket text\n")
	}
	if strings.Contains(errStr, "gap not found") {
		sb.WriteString("- Gap IDs must match existing gaps from 'Open Gaps' or 'Pending Gaps' section\n")
		sb.WriteString("- Use short numeric IDs shown in [gap N] format\n")
	}
	if strings.Contains(errStr, "gap is not pending") {
		sb.WriteString("- The 'ask' action can only promote gaps with 'pending' status\n")
		sb.WriteString("- Check the 'Pending Gaps' section for valid gap IDs to ask\n")
	}
	if strings.Contains(errStr, "content too short") || strings.Contains(errStr, "content too long") {
		sb.WriteString("- Comment content must be 1-65000 characters\n")
	}
	if strings.Contains(errStr, "missing question") {
		sb.WriteString("- Each gap must have a non-empty question field\n")
	}
	if strings.Contains(errStr, "invalid gap severity") {
		sb.WriteString("- Valid severity values: blocking, high, medium, low\n")
	}
	if strings.Contains(errStr, "invalid gap respondent") {
		sb.WriteString("- Valid respondent values: reporter, assignee\n")
	}
	if strings.Contains(errStr, "invalid gap close reason") {
		sb.WriteString("- Valid close reasons: answered, inferred, not_relevant\n")
	}
	if strings.Contains(errStr, "gap close missing note") {
		sb.WriteString("- Closing with 'answered' or 'inferred' requires a note\n")
	}
	if strings.Contains(errStr, "proceed signal") {
		sb.WriteString("- ready_for_spec_generation requires a proceed_signal from the human\n")
	}
	if strings.Contains(errStr, "open gaps") || strings.Contains(errStr, "blocking gaps") {
		sb.WriteString("- Close all open gaps before signaling ready_for_spec_generation\n")
	}
	if strings.Contains(errStr, "finding missing") {
		sb.WriteString("- Each finding needs synthesis and at least one source with location\n")
	}
	if strings.Contains(errStr, "learning missing") || strings.Contains(errStr, "invalid learning type") {
		sb.WriteString("- Learnings need content and valid type (domain_learnings or code_learnings)\n")
	}

	return sb.String()
}
