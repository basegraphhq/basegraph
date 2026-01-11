package brain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"basegraph.app/relay/common/id"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/service/issue_tracker"
	"basegraph.app/relay/internal/store"
	"github.com/jackc/pgx/v5/pgconn"
)

const maxCodeFindings = 20

type actionExecutor struct {
	issueTracker  issue_tracker.IssueTrackerService
	issues        store.IssueStore
	gaps          store.GapStore
	integrations  store.IntegrationStore
	configs       store.IntegrationConfigStore
	learnings     store.LearningStore
	specStore     store.SpecStore
	specGenerator *SpecGenerator
}

func NewActionExecutor(
	issueTracker issue_tracker.IssueTrackerService,
	issues store.IssueStore,
	gaps store.GapStore,
	integrations store.IntegrationStore,
	configs store.IntegrationConfigStore,
	learnings store.LearningStore,
	specStore store.SpecStore,
	specGenerator *SpecGenerator,
) actionExecutor {
	return actionExecutor{
		issueTracker:  issueTracker,
		issues:        issues,
		gaps:          gaps,
		integrations:  integrations,
		configs:       configs,
		learnings:     learnings,
		specStore:     specStore,
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
	case ActionTypeUpdateSpec:
		return e.executeUpdateSpec(ctx, issue, action)
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
		_, err = e.issueTracker.CreateDiscussion(ctx, issue_tracker.CreateDiscussionParams{
			Issue:   issue,
			Content: content,
		})
	} else {
		_, err = e.issueTracker.ReplyToThread(ctx, issue_tracker.ReplyToThreadParams{
			Issue:        issue,
			DiscussionID: data.ReplyToID,
			Content:      content,
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

	for _, close := range data.Close {
		id, err := strconv.ParseInt(string(close.GapID), 10, 64)
		if err != nil {
			return fmt.Errorf("parsing gap id %s: %w", string(close.GapID), err)
		}
		if err := e.closeGapByAnyID(ctx, id, close.Reason, close.Note); err != nil {
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
		"closed_gaps", data.ClosedGaps,
		"relevant_findings", data.RelevantFindings,
		"learning_ids", data.LearningsApplied,
		"context_summary", data.ContextSummary,
		"proceed_signal", data.ProceedSignal)

	// Fetch closed gaps for this issue
	closedGaps, err := e.gaps.ListClosedByIssue(ctx, issue.ID, 50)
	if err != nil {
		return fmt.Errorf("fetching closed gaps: %w", err)
	}

	// Fetch learnings for the workspace
	integration, err := e.integrations.GetByID(ctx, issue.IntegrationID)
	if err != nil {
		return fmt.Errorf("fetching integration: %w", err)
	}
	learnings, err := e.learnings.ListByWorkspace(ctx, integration.WorkspaceID)
	if err != nil {
		return fmt.Errorf("fetching learnings: %w", err)
	}

	// Fetch relay username for conversation mapping
	relayUsername, err := e.getRelayUsername(ctx, issue.IntegrationID)
	if err != nil {
		slog.WarnContext(ctx, "failed to get relay username, using empty",
			"error", err,
			"issue_id", issue.ID)
		relayUsername = ""
	}

	// Check for existing spec
	var existingSpec *string
	var existingSpecRef *model.SpecRef
	if issue.Spec != nil && *issue.Spec != "" {
		var ref model.SpecRef
		if err := json.Unmarshal([]byte(*issue.Spec), &ref); err == nil {
			content, _, err := e.specStore.Read(ctx, ref)
			if err == nil {
				existingSpec = &content
				existingSpecRef = &ref
			}
		}
	}

	// Build input for SpecGenerator
	input := BuildSpecGeneratorInput(
		issue,
		data,
		closedGaps,
		learnings,
		existingSpec,
		existingSpecRef,
		0, // Let SpecGenerator infer complexity
		relayUsername,
	)

	// Generate spec
	output, err := e.specGenerator.Generate(ctx, input)
	if err != nil {
		return fmt.Errorf("generating spec: %w", err)
	}

	// Write spec to store
	ref, err := e.specGenerator.WriteSpec(ctx, issue, output)
	if err != nil {
		return fmt.Errorf("writing spec: %w", err)
	}

	// Update issue with spec reference
	refJSON, err := SerializeSpecRef(ref)
	if err != nil {
		return fmt.Errorf("serializing spec ref: %w", err)
	}
	issue.Spec = &refJSON

	if _, err := e.issues.Upsert(ctx, &issue); err != nil {
		return fmt.Errorf("updating issue with spec ref: %w", err)
	}

	slog.InfoContext(ctx, "spec generated and stored",
		"issue_id", issue.ID,
		"spec_path", ref.Path,
		"spec_sha256", ref.SHA256,
		"char_count", output.Metadata.CharCount,
		"validation_errors", len(output.ValidationErrors))

	return nil
}

func (e *actionExecutor) executeUpdateSpec(ctx context.Context, issue model.Issue, action Action) error {
	data, err := ParseActionData[UpdateSpecAction](action)
	if err != nil {
		return err
	}

	// Generate slug from issue title
	slug := "spec"
	if issue.Title != nil && *issue.Title != "" {
		slug = *issue.Title
	}

	// Write spec to store
	ref, err := e.specStore.Write(ctx, issue.ID, string(issue.Provider), issue.ExternalIssueID, slug, data.ContentMarkdown)
	if err != nil {
		return fmt.Errorf("writing spec: %w", err)
	}

	// Serialize SpecRef to JSON for storage in issues.spec
	refJSON, err := json.Marshal(ref)
	if err != nil {
		return fmt.Errorf("serializing spec ref: %w", err)
	}

	specStr := string(refJSON)
	issue.Spec = &specStr

	// Update issue with new spec reference
	if _, err := e.issues.Upsert(ctx, &issue); err != nil {
		return fmt.Errorf("updating issue spec: %w", err)
	}

	slog.InfoContext(ctx, "spec updated",
		"issue_id", issue.ID,
		"spec_path", ref.Path,
		"spec_sha256", ref.SHA256,
		"reason", data.Reason)

	return nil
}

// getRelayUsername fetches Relay's service account username for the integration.
func (e *actionExecutor) getRelayUsername(ctx context.Context, integrationID int64) (string, error) {
	config, err := e.configs.GetByIntegrationAndKey(ctx, integrationID, model.ConfigKeyServiceAccount)
	if err != nil {
		return "", fmt.Errorf("fetching service account config: %w", err)
	}

	var sa model.ServiceAccountConfig
	if err := json.Unmarshal(config.Value, &sa); err != nil {
		return "", fmt.Errorf("parsing service account config: %w", err)
	}

	return sa.Username, nil
}
