package brain

import (
	"context"
	"fmt"
	"strconv"

	"basegraph.co/relay/common/id"
	"basegraph.co/relay/internal/model"
	"basegraph.co/relay/internal/service/issue_tracker"
	"basegraph.co/relay/internal/store"
)

const maxCodeFindings = 20

type actionExecutor struct {
	issueTracker issue_tracker.IssueTrackerService
	issues       store.IssueStore
	gaps         store.GapStore
}

func NewActionExecutor(
	issueTracker issue_tracker.IssueTrackerService,
	issues store.IssueStore,
	gaps store.GapStore,
) actionExecutor {
	return actionExecutor{
		issueTracker: issueTracker,
		issues:       issues,
		gaps:         gaps,
	}
}

func (e *actionExecutor) Execute(ctx context.Context, issue model.Issue, action Action) error {
	switch action.Type {
	case ActionTypePostComment:
		return e.executePostComment(ctx, issue, action)
	case ActionTypeUpdateFindings:
		return e.executeUpdateFindings(ctx, issue, action)
	case ActionTypeUpdateGaps:
		return e.executeUpdateGaps(ctx, issue, action)
	}
	return nil
}

func (e *actionExecutor) ExecuteBatch(ctx context.Context, issue model.Issue, actions []Action) []ActionError {
	var errors []ActionError
	for _, action := range actions {
		if err := e.Execute(ctx, issue, action); err != nil {
			errors = append(errors, ActionError{Action: action, Error: err.Error(), Recoverable: true})
		}
	}
	return errors
}

func (e *actionExecutor) executePostComment(ctx context.Context, issue model.Issue, action Action) error {
	data, err := ParseActionData[PostCommentAction](action)
	if err != nil {
		return err
	}

	if data.ReplyToID == "" {
		_, err = e.issueTracker.CreateDiscussion(ctx, issue_tracker.CreateDiscussionParams{
			Issue:   issue,
			Content: data.Content,
		})
	} else {
		_, err = e.issueTracker.ReplyToThread(ctx, issue_tracker.ReplyToThreadParams{
			Issue:        issue,
			DiscussionID: data.ReplyToID,
			Content:      data.Content,
		})
	}

	return err
}

func (e *actionExecutor) executeUpdateFindings(ctx context.Context, issue model.Issue, action Action) error {
	data, err := ParseActionData[UpdateFindingsAction](action)
	if err != nil {
		return err
	}

	findings := issue.CodeFindings

	if len(data.Remove) > 0 {
		removeSet := make(map[string]bool, len(data.Remove))
		for _, id := range data.Remove {
			removeSet[id] = true
		}
		filtered := make([]model.CodeFinding, 0, len(findings))
		for _, f := range findings {
			if !removeSet[f.ID] {
				filtered = append(filtered, f)
			}
		}
		findings = filtered
	}

	for _, input := range data.Add {
		sources := make([]model.CodeSource, len(input.Sources))
		for i, s := range input.Sources {
			sources[i] = model.CodeSource{
				Location: s.Location,
				Snippet:  s.Snippet,
				QName:    s.QName,
				Kind:     s.Kind,
			}
		}
		finding := model.CodeFinding{
			ID:        fmt.Sprintf("%d", id.New()),
			Synthesis: input.Synthesis,
			Sources:   sources,
		}
		findings = append(findings, finding)
	}

	if len(findings) > maxCodeFindings {
		findings = findings[len(findings)-maxCodeFindings:]
	}

	issue.CodeFindings = findings
	if _, err := e.issues.Upsert(ctx, &issue); err != nil {
		return fmt.Errorf("updating issue findings: %w", err)
	}

	return nil
}

func (e *actionExecutor) executeUpdateGaps(ctx context.Context, issue model.Issue, action Action) error {
	data, err := ParseActionData[UpdateGapsAction](action)
	if err != nil {
		return err
	}

	for _, input := range data.Add {
		gap := model.Gap{
			IssueID:    issue.ID,
			Status:     model.GapStatusOpen,
			Question:   input.Question,
			Evidence:   input.Evidence,
			Severity:   model.GapSeverity(input.Severity),
			Respondent: model.GapRespondent(input.Respondent),
		}
		if _, err := e.gaps.Create(ctx, gap); err != nil {
			return fmt.Errorf("creating gap: %w", err)
		}
	}

	for _, idStr := range data.Resolve {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return fmt.Errorf("parsing gap id %s: %w", idStr, err)
		}
		if _, err := e.gaps.Resolve(ctx, id); err != nil {
			return fmt.Errorf("resolving gap %d: %w", id, err)
		}
	}

	for _, idStr := range data.Skip {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return fmt.Errorf("parsing gap id %s: %w", idStr, err)
		}
		if _, err := e.gaps.Skip(ctx, id); err != nil {
			return fmt.Errorf("skipping gap %d: %w", id, err)
		}
	}

	return nil
}
