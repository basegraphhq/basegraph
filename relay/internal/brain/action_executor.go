package brain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"basegraph.co/relay/common/id"
	"basegraph.co/relay/internal/model"
	"basegraph.co/relay/internal/service/issue_tracker"
	"basegraph.co/relay/internal/store"
	"github.com/jackc/pgx/v5/pgconn"
)

const maxCodeFindings = 20

// Comment size limits per provider
const (
	gitlabCommentLimit  = 1000000 // ~1M chars
	githubCommentLimit  = 65536   // 64K chars
	linearCommentLimit  = 65536   // 64K chars
	defaultCommentLimit = 65536   // Conservative default
)

type actionExecutor struct {
	issueTracker  issue_tracker.IssueTrackerService
	txRunner      TxRunner
	issues        store.IssueStore
	gaps          store.GapStore
	integrations  store.IntegrationStore
	learnings     store.LearningStore
	specGenerator *SpecGenerator
}

func NewActionExecutor(
	issueTracker issue_tracker.IssueTrackerService,
	txRunner TxRunner,
	issues store.IssueStore,
	gaps store.GapStore,
	integrations store.IntegrationStore,
	learnings store.LearningStore,
	specGenerator *SpecGenerator,
) actionExecutor {
	return actionExecutor{
		issueTracker:  issueTracker,
		txRunner:      txRunner,
		issues:        issues,
		gaps:          gaps,
		integrations:  integrations,
		learnings:     learnings,
		specGenerator: specGenerator,
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
	case ActionTypeUpdateLearnings:
		return e.executeUpdateLearnings(ctx, issue, action)
	case ActionTypeReadyForSpecGeneration:
		return e.executeReadyForSpecGeneration(ctx, issue, action)
	case ActionTypeSetSpecStatus:
		return e.executeSetSpecStatus(ctx, issue, action)
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

	content, stripped := SanitizeComment(data.Content)
	if stripped > 0 {
		slog.WarnContext(ctx, "stripped gap IDs from comment", "count", stripped, "issue_id", issue.ID)
	}

	if data.ReplyToID == "" {
		slog.InfoContext(ctx, "creating discussion",
			"issue_id", issue.ID,
			"content_length", len(content))
		_, err = e.issueTracker.CreateDiscussion(ctx, issue_tracker.CreateDiscussionParams{
			Issue:   issue,
			Content: content,
		})
	} else {
		slog.InfoContext(ctx, "replying to thread",
			"issue_id", issue.ID,
			"reply_to_id", data.ReplyToID,
			"content_length", len(content))
		_, err = e.issueTracker.ReplyToThread(ctx, issue_tracker.ReplyToThreadParams{
			Issue:        issue,
			DiscussionID: data.ReplyToID,
			Content:      content,
		})
	}

	if err != nil {
		slog.ErrorContext(ctx, "failed to post comment",
			"issue_id", issue.ID,
			"reply_to_id", data.ReplyToID,
			"error", err)
	}

	return err
}

func (e *actionExecutor) executeUpdateFindings(ctx context.Context, issue model.Issue, action Action) error {
	data, err := ParseActionData[UpdateFindingsAction](action)
	if err != nil {
		return err
	}

	if e.txRunner == nil {
		return fmt.Errorf("txRunner not configured for findings update")
	}

	// Pre-compute findings to add (with generated IDs) outside the transaction
	// to minimize lock hold time
	findingsToAdd := make([]model.CodeFinding, len(data.Add))
	for i, input := range data.Add {
		sources := make([]model.CodeSource, len(input.Sources))
		for j, s := range input.Sources {
			sources[j] = model.CodeSource{
				Location: s.Location,
				Snippet:  s.Snippet,
				Kind:     s.Kind,
			}
		}
		findingsToAdd[i] = model.CodeFinding{
			ID:        fmt.Sprintf("%d", id.New()),
			Synthesis: input.Synthesis,
			Sources:   sources,
		}
	}

	// Use transaction with SELECT FOR UPDATE to prevent lost updates
	// when multiple explores write findings concurrently
	return e.txRunner.WithTx(ctx, func(stores StoreProvider) error {
		// Lock and fetch fresh issue data
		freshIssue, err := stores.Issues().GetByIDForUpdate(ctx, issue.ID)
		if err != nil {
			return fmt.Errorf("locking issue for findings update: %w", err)
		}

		findings := freshIssue.CodeFindings

		// Apply removals
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

		// Append new findings
		findings = append(findings, findingsToAdd...)

		// Enforce max limit (keep newest)
		if len(findings) > maxCodeFindings {
			findings = findings[len(findings)-maxCodeFindings:]
		}

		// Persist within the same transaction
		if err := stores.Issues().UpdateCodeFindings(ctx, issue.ID, findings); err != nil {
			return fmt.Errorf("updating issue findings: %w", err)
		}

		return nil
	})
}

func (e *actionExecutor) executeUpdateGaps(ctx context.Context, issue model.Issue, action Action) error {
	data, err := ParseActionData[UpdateGapsAction](action)
	if err != nil {
		return err
	}

	for _, input := range data.Add {
		status := model.GapStatusOpen
		if input.Pending {
			status = model.GapStatusPending
		}
		gap := model.Gap{
			IssueID:    issue.ID,
			Status:     status,
			Question:   input.Question,
			Evidence:   input.Evidence,
			Severity:   model.GapSeverity(input.Severity),
			Respondent: model.GapRespondent(input.Respondent),
		}
		if _, err := e.gaps.Create(ctx, gap); err != nil {
			return fmt.Errorf("creating gap: %w", err)
		}
	}

	for _, close := range data.Close {
		id, err := strconv.ParseInt(string(close.GapID), 10, 64)
		if err != nil {
			return fmt.Errorf("parsing gap id %s: %w", string(close.GapID), err)
		}
		if err := e.closeGapByAnyID(ctx, id, close.Reason, close.Note); err != nil {
			return err
		}
	}

	// Promote pending gaps to open (asked)
	for _, gapID := range data.Ask {
		id, err := strconv.ParseInt(string(gapID), 10, 64)
		if err != nil {
			return fmt.Errorf("parsing gap id %s: %w", string(gapID), err)
		}
		if err := e.openGapByAnyID(ctx, id); err != nil {
			return err
		}
	}

	return nil
}

func (e *actionExecutor) closeGapByAnyID(ctx context.Context, id int64, reason GapCloseReason, note string) error {
	var status model.GapStatus
	switch reason {
	case GapCloseAnswered, GapCloseInferred:
		status = model.GapStatusResolved
	case GapCloseNotRelevant:
		status = model.GapStatusSkipped
	default:
		return fmt.Errorf("unsupported close reason: %s", reason)
	}

	_, err := e.gaps.Close(ctx, id, status, string(reason), note)
	if err == nil {
		return nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("closing gap %d: %w", id, err)
	}

	// Try short ID
	gap, err := e.gaps.GetByShortID(ctx, id)
	if err != nil {
		return fmt.Errorf("closing gap %d: %w", id, err)
	}
	if _, err := e.gaps.Close(ctx, gap.ID, status, string(reason), note); err != nil {
		return fmt.Errorf("closing gap %d: %w", id, err)
	}

	return nil
}

func (e *actionExecutor) openGapByAnyID(ctx context.Context, id int64) error {
	_, err := e.gaps.Open(ctx, id)
	if err == nil {
		return nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("opening gap %d: %w", id, err)
	}

	// Try short ID
	gap, err := e.gaps.GetByShortID(ctx, id)
	if err != nil {
		return fmt.Errorf("opening gap %d: %w", id, err)
	}
	if _, err := e.gaps.Open(ctx, gap.ID); err != nil {
		return fmt.Errorf("opening gap %d: %w", id, err)
	}

	return nil
}

func (e *actionExecutor) executeUpdateLearnings(ctx context.Context, issue model.Issue, action Action) error {
	data, err := ParseActionData[UpdateLearningsAction](action)
	if err != nil {
		return err
	}
	if len(data.Propose) == 0 {
		return nil
	}

	integration, err := e.integrations.GetByID(ctx, issue.IntegrationID)
	if err != nil {
		return fmt.Errorf("loading integration for learnings: %w", err)
	}

	issueID := issue.ID
	for _, input := range data.Propose {
		learning := model.Learning{
			ID:                   id.New(),
			WorkspaceID:          integration.WorkspaceID,
			RuleUpdatedByIssueID: &issueID,
			Type:                 input.Type,
			Content:              input.Content,
		}

		if err := e.learnings.Create(ctx, &learning); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				continue
			}
			return fmt.Errorf("creating learning: %w", err)
		}
	}

	return nil
}

func (e *actionExecutor) executeReadyForSpecGeneration(ctx context.Context, issue model.Issue, action Action) error {
	data, err := ParseActionData[ReadyForSpecGenerationAction](action)
	if err != nil {
		return err
	}

	slog.InfoContext(ctx, "ready_for_spec_generation received",
		"issue_id", issue.ID,
		"closed_gaps", len(data.ClosedGaps),
		"relevant_findings", len(data.RelevantFindings),
		"context_summary_length", len(data.ContextSummary),
		"proceed_signal", data.ProceedSignal)

	// Refresh issue to avoid overwriting concurrent updates (e.g., findings)
	freshIssue, err := e.issues.GetByID(ctx, issue.ID)
	if err != nil {
		slog.WarnContext(ctx, "failed to refresh issue before spec generation",
			"issue_id", issue.ID,
			"error", err)
		return fmt.Errorf("refreshing issue: %w", err)
	}
	issue = *freshIssue

	// 1. Post acknowledgment
	ackPosted := false
	_, ackErr := e.issueTracker.CreateDiscussion(ctx, issue_tracker.CreateDiscussionParams{
		Issue:   issue,
		Content: "Got it — drafting the implementation approach now.",
	})
	if ackErr != nil {
		slog.WarnContext(ctx, "failed to post spec generation ack",
			"issue_id", issue.ID,
			"error", ackErr)
		// Continue anyway - ack is nice to have, not critical
	} else {
		ackPosted = true
	}

	// 2. Fetch closed gaps for this issue
	closedGaps, err := e.gaps.ListClosedByIssue(ctx, issue.ID, 100)
	if err != nil {
		e.postSpecError(ctx, issue, ackPosted, "Failed to fetch gap context")
		return fmt.Errorf("fetching closed gaps: %w", err)
	}

	// 3. Fetch learnings for the workspace
	integration, err := e.integrations.GetByID(ctx, issue.IntegrationID)
	if err != nil {
		e.postSpecError(ctx, issue, ackPosted, "Failed to fetch workspace context")
		return fmt.Errorf("fetching integration: %w", err)
	}

	learnings, err := e.learnings.ListByWorkspace(ctx, integration.WorkspaceID)
	if err != nil {
		e.postSpecError(ctx, issue, ackPosted, "Failed to fetch learnings")
		return fmt.Errorf("fetching learnings: %w", err)
	}

	// 4. Build input for spec generator
	input := SpecGeneratorInput{
		Issue:          issue,
		ContextSummary: data.ContextSummary,
		Gaps:           closedGaps,
		Findings:       issue.CodeFindings,
		Learnings:      learnings,
		ProceedSignal:  data.ProceedSignal,
	}

	// 5. Generate spec
	slog.InfoContext(ctx, "generating spec",
		"issue_id", issue.ID,
		"gaps", len(closedGaps),
		"findings", len(issue.CodeFindings),
		"learnings", len(learnings))

	output, err := e.specGenerator.Generate(ctx, input)
	if err != nil {
		e.postSpecError(ctx, issue, ackPosted, "Spec generation encountered an error")
		return fmt.Errorf("generating spec: %w", err)
	}

	slog.InfoContext(ctx, "spec generated",
		"issue_id", issue.ID,
		"spec_length", len(output.Spec))

	// 6. Post spec to issue tracker (with splitting if needed)
	if err := e.postSpec(ctx, issue, output.Spec); err != nil {
		e.postSpecError(ctx, issue, ackPosted, "Failed to post spec")
		return fmt.Errorf("posting spec: %w", err)
	}

	// 7. Store spec without overwriting other fields (mark completed)
	if err := e.issues.UpdateSpec(ctx, issue.ID, &output.Spec); err != nil {
		e.postSpecError(ctx, issue, ackPosted, "Failed to save spec")
		return fmt.Errorf("storing spec: %w", err)
	}

	slog.InfoContext(ctx, "spec generation completed",
		"issue_id", issue.ID,
		"spec_length", len(output.Spec))

	return nil
}

func (e *actionExecutor) executeSetSpecStatus(ctx context.Context, issue model.Issue, action Action) error {
	data, err := ParseActionData[SetSpecStatusAction](action)
	if err != nil {
		return err
	}

	status := model.SpecStatus(data.Status)
	if status != model.SpecStatusApproved && status != model.SpecStatusRejected {
		return fmt.Errorf("invalid spec status: %s", data.Status)
	}

	slog.InfoContext(ctx, "setting spec status",
		"issue_id", issue.ID,
		"status", status)

	return e.issues.UpdateSpecStatus(ctx, issue.ID, status)
}

// postSpecError posts an error message to the issue if ack was already posted.
func (e *actionExecutor) postSpecError(ctx context.Context, issue model.Issue, ackPosted bool, message string) {
	if !ackPosted {
		return // No ack posted, user doesn't know we started - fail silently
	}

	content := fmt.Sprintf("%s 😕. Should I retry?", message)
	_, err := e.issueTracker.CreateDiscussion(ctx, issue_tracker.CreateDiscussionParams{
		Issue:   issue,
		Content: content,
	})
	if err != nil {
		slog.WarnContext(ctx, "failed to post spec error message",
			"issue_id", issue.ID,
			"error", err)
	}
}

// postSpec posts the spec to the issue tracker, splitting if necessary for platform limits.
func (e *actionExecutor) postSpec(ctx context.Context, issue model.Issue, spec string) error {
	limit := e.commentLimitForProvider(issue.Provider)

	// If spec fits in one comment, post directly
	if len(spec) <= limit {
		_, err := e.issueTracker.CreateDiscussion(ctx, issue_tracker.CreateDiscussionParams{
			Issue:   issue,
			Content: spec,
		})
		return err
	}

	// Split into parts
	parts := e.splitSpec(spec, limit-500) // Leave room for part header
	for i, part := range parts {
		header := fmt.Sprintf("## Implementation Spec (Part %d of %d)\n\n", i+1, len(parts))
		content := header + part

		_, err := e.issueTracker.CreateDiscussion(ctx, issue_tracker.CreateDiscussionParams{
			Issue:   issue,
			Content: content,
		})
		if err != nil {
			return fmt.Errorf("posting spec part %d/%d: %w", i+1, len(parts), err)
		}

		slog.InfoContext(ctx, "posted spec part",
			"issue_id", issue.ID,
			"part", i+1,
			"total_parts", len(parts),
			"part_length", len(content))
	}

	return nil
}

// commentLimitForProvider returns the comment character limit for a provider.
func (e *actionExecutor) commentLimitForProvider(provider model.Provider) int {
	switch provider {
	case model.ProviderGitLab:
		return gitlabCommentLimit
	case model.ProviderGitHub:
		return githubCommentLimit
	case model.ProviderLinear:
		return linearCommentLimit
	default:
		return defaultCommentLimit
	}
}

// splitSpec splits a spec into parts that fit within the limit.
// Tries to split at section boundaries (---) when possible.
func (e *actionExecutor) splitSpec(spec string, limit int) []string {
	if len(spec) <= limit {
		return []string{spec}
	}

	var parts []string
	remaining := spec

	for len(remaining) > 0 {
		if len(remaining) <= limit {
			parts = append(parts, remaining)
			break
		}

		// Find a good split point - prefer section boundaries
		splitPoint := limit

		// Look for "---" section divider within the limit
		searchArea := remaining[:limit]
		if idx := lastIndex(searchArea, "\n---\n"); idx > limit/2 {
			splitPoint = idx + 5 // Include the divider in current part
		} else if idx := lastIndex(searchArea, "\n## "); idx > limit/2 {
			// Split before a heading
			splitPoint = idx + 1
		} else if idx := lastIndex(searchArea, "\n\n"); idx > limit/2 {
			// Split at paragraph boundary
			splitPoint = idx + 2
		} else if idx := lastIndex(searchArea, "\n"); idx > limit/2 {
			// Split at line boundary
			splitPoint = idx + 1
		}

		parts = append(parts, remaining[:splitPoint])
		remaining = remaining[splitPoint:]
	}

	return parts
}

// lastIndex returns the last index of substr in s, or -1 if not found.
func lastIndex(s, substr string) int {
	for i := len(s) - len(substr); i >= 0; i-- {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
