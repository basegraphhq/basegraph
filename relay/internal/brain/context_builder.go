package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"basegraph.app/relay/common/llm"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/store"
)

const maxDiscussions = 100

// specStubInfo holds parsed spec metadata for context dump.
// This is a lightweight representation to avoid injecting full spec into Planner context.
type specStubInfo struct {
	Path      string
	UpdatedAt time.Time
	SHA256    string
	Summary   string // TL;DR or first N chars
}

// contextBuilder constructs the LLM message thread for Planner.
// It fetches workspace-level learnings and formats discussions as a proper conversation.
type contextBuilder struct {
	integrations store.IntegrationStore
	configs      store.IntegrationConfigStore
	learnings    store.LearningStore
	gaps         store.GapStore
	specStore    store.SpecStore
}

// NewContextBuilder creates a ContextBuilder with required store dependencies.
func NewContextBuilder(
	integrations store.IntegrationStore,
	configs store.IntegrationConfigStore,
	learnings store.LearningStore,
	gaps store.GapStore,
	specStore store.SpecStore,
) *contextBuilder {
	return &contextBuilder{
		integrations: integrations,
		configs:      configs,
		learnings:    learnings,
		gaps:         gaps,
		specStore:    specStore,
	}
}

// BuildPlannerMessages constructs the full message thread for Planner.
// Returns messages in order: system prompt, context dump, discussion history.
// triggerThreadID is the thread that triggered this engagement (for reply context).
func (b *contextBuilder) BuildPlannerMessages(ctx context.Context, issue model.Issue, triggerThreadID string) ([]llm.Message, error) {
	// Fetch Relay's identity for self-recognition
	relayUsername, err := b.getRelayUsername(ctx, issue.IntegrationID)
	if err != nil {
		return nil, fmt.Errorf("getting relay username: %w", err)
	}

	// Fetch workspace-level learnings
	learnings, err := b.fetchLearnings(ctx, issue.IntegrationID)
	if err != nil {
		return nil, fmt.Errorf("fetching learnings: %w", err)
	}

	// Fetch open gaps for this issue
	gaps, err := b.fetchOpenGaps(ctx, issue.ID)
	if err != nil {
		return nil, fmt.Errorf("fetching open gaps: %w", err)
	}

	// Fetch recent closed gaps (last 10) for context
	recentClosed, err := b.fetchRecentClosedGaps(ctx, issue.ID, 10)
	if err != nil {
		return nil, err
	}

	// Fetch spec stub if spec exists (non-blocking: log error but don't fail)
	var specStub *specStubInfo
	if issue.Spec != nil && *issue.Spec != "" {
		specStub = b.fetchSpecStub(ctx, *issue.Spec)
	}

	messages := make([]llm.Message, 0, 3+len(issue.Discussions))

	// Message 1: System prompt with self-identity
	messages = append(messages, llm.Message{
		Role:    "system",
		Content: b.buildSystemPrompt(relayUsername),
	})

	// Message 2: Context dump (issue metadata, participants, learnings, gaps, findings, spec stub)
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: b.buildContextDump(issue, learnings, gaps, recentClosed, triggerThreadID, specStub),
	})

	// Messages 3+: Discussion history as conversation
	discussionMessages := b.buildDiscussionMessages(issue.Discussions, relayUsername)
	messages = append(messages, discussionMessages...)

	return messages, nil
}

// getRelayUsername fetches Relay's service account username for the integration.
func (b *contextBuilder) getRelayUsername(ctx context.Context, integrationID int64) (string, error) {
	config, err := b.configs.GetByIntegrationAndKey(ctx, integrationID, model.ConfigKeyServiceAccount)
	if err != nil {
		return "", fmt.Errorf("fetching service account config: %w", err)
	}

	var sa model.ServiceAccountConfig
	if err := json.Unmarshal(config.Value, &sa); err != nil {
		return "", fmt.Errorf("parsing service account config: %w", err)
	}

	return sa.Username, nil
}

func (b *contextBuilder) GetRelayServiceAccount(ctx context.Context, integrationID int64) (model.ServiceAccountConfig, error) {
	config, err := b.configs.GetByIntegrationAndKey(ctx, integrationID, model.ConfigKeyServiceAccount)
	if err != nil {
		return model.ServiceAccountConfig{}, fmt.Errorf("fetching service account config: %w", err)
	}

	var sa model.ServiceAccountConfig
	if err := json.Unmarshal(config.Value, &sa); err != nil {
		return model.ServiceAccountConfig{}, fmt.Errorf("parsing service account config: %w", err)
	}

	return sa, nil
}

// fetchLearnings retrieves workspace-level learnings for the integration.
func (b *contextBuilder) fetchLearnings(ctx context.Context, integrationID int64) ([]model.Learning, error) {
	integration, err := b.integrations.GetByID(ctx, integrationID)
	if err != nil {
		return nil, fmt.Errorf("fetching integration: %w", err)
	}

	learnings, err := b.learnings.ListByWorkspace(ctx, integration.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("fetching learnings: %w", err)
	}

	return learnings, nil
}

// fetchOpenGaps retrieves open gaps for the issue.
func (b *contextBuilder) fetchOpenGaps(ctx context.Context, issueID int64) ([]model.Gap, error) {
	gaps, err := b.gaps.ListOpenByIssue(ctx, issueID)
	if err != nil {
		return nil, fmt.Errorf("fetching open gaps: %w", err)
	}
	return gaps, nil
}

// fetchRecentClosedGaps returns the most recent closed gaps (resolved or skipped), up to limit.
func (b *contextBuilder) fetchRecentClosedGaps(ctx context.Context, issueID int64, limit int) ([]model.Gap, error) {
	all, err := b.gaps.ListClosedByIssue(ctx, issueID, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("fetching closed gaps: %w", err)
	}
	return all, nil
}

// fetchSpecStub parses the SpecRef JSON and retrieves a summary for context dump.
// Returns nil if spec doesn't exist or can't be read (non-fatal: Planner can use read_spec tool).
func (b *contextBuilder) fetchSpecStub(ctx context.Context, specRefJSON string) *specStubInfo {
	if b.specStore == nil {
		return nil
	}

	var ref model.SpecRef
	if err := json.Unmarshal([]byte(specRefJSON), &ref); err != nil {
		return nil
	}

	content, meta, err := b.specStore.Read(ctx, ref)
	if err != nil {
		return nil
	}

	// Extract summary (TL;DR or first 500 chars)
	summary := store.ExtractSpecSummary(content, 500)

	return &specStubInfo{
		Path:      ref.Path,
		UpdatedAt: meta.UpdatedAt,
		SHA256:    meta.SHA256,
		Summary:   summary,
	}
}

// buildSystemPrompt creates the system message with Relay's identity.
func (b *contextBuilder) buildSystemPrompt(relayUsername string) string {
	return fmt.Sprintf(`%s

# Self-Identity

Your comments appear as @%s. When you see messages from @%s in the discussion history, those are YOUR previous messages.`, plannerSystemPrompt, relayUsername, relayUsername)
}

// buildContextDump creates the context message with issue metadata, learnings, gaps, findings, and spec stub.
func (b *contextBuilder) buildContextDump(issue model.Issue, learnings []model.Learning, openGaps []model.Gap, recentClosed []model.Gap, triggerThreadID string, specStub *specStubInfo) string {
	var sb strings.Builder

	// Issue section
	sb.WriteString("# Issue\n\n")
	if issue.Title != nil && *issue.Title != "" {
		sb.WriteString(fmt.Sprintf("**Title**: %s\n\n", *issue.Title))
	}
	if issue.Description != nil && *issue.Description != "" {
		sb.WriteString(fmt.Sprintf("**Description**:\n%s\n\n", *issue.Description))
	}

	// Participants section
	sb.WriteString("# Participants\n\n")
	if issue.Reporter != nil && *issue.Reporter != "" {
		sb.WriteString(fmt.Sprintf("**Reporter**: @%s — created this issue\n", *issue.Reporter))
	}
	if len(issue.Assignees) > 0 {
		assigneeList := make([]string, len(issue.Assignees))
		for i, a := range issue.Assignees {
			assigneeList[i] = "@" + a
		}
		sb.WriteString(fmt.Sprintf("**Assignee(s)**: %s — assigned to implement\n", strings.Join(assigneeList, ", ")))
	}
	if len(issue.Members) > 0 {
		memberList := make([]string, len(issue.Members))
		for i, m := range issue.Members {
			memberList[i] = "@" + m
		}
		sb.WriteString(fmt.Sprintf("**Other participants**: %s\n", strings.Join(memberList, ", ")))
	}
	sb.WriteString("\n")

	// Learnings section
	if len(learnings) > 0 {
		sb.WriteString("# Learnings\n\n")
		for i, l := range learnings {
			sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, l.Type, l.Content))
		}
		sb.WriteString("\n")
	}

	// Open gaps section
	if gapsSection := formatGapsSection(issue, openGaps); gapsSection != "" {
		sb.WriteString(gapsSection)
	}

	// Recently closed gaps section (last N)
	if closedSection := formatClosedGapsSection(issue, recentClosed); closedSection != "" {
		sb.WriteString(closedSection)
	}

	// Code findings section
	if len(issue.CodeFindings) > 0 {
		sb.WriteString("# Code Findings\n\n")
		for _, f := range issue.CodeFindings {
			// Format sources as header
			if len(f.Sources) > 0 {
				locations := make([]string, 0, len(f.Sources))
				for _, s := range f.Sources {
					locations = append(locations, fmt.Sprintf("`%s`", s.Location))
				}
				sb.WriteString(fmt.Sprintf("## %s\n\n", strings.Join(locations, ", ")))
			}
			sb.WriteString(f.Synthesis)
			sb.WriteString("\n\n")
		}
	}

	// Current spec section (stub only - full content via read_spec tool)
	if specStub != nil {
		sb.WriteString("# Current Spec\n\n")
		sb.WriteString(fmt.Sprintf("- **Path:** `%s`\n", specStub.Path))
		sb.WriteString(fmt.Sprintf("- **Last updated:** %s\n", specStub.UpdatedAt.UTC().Format(time.RFC3339)))
		sb.WriteString(fmt.Sprintf("- **SHA256:** %s\n\n", specStub.SHA256[:16]+"..."))
		sb.WriteString("### Summary (excerpt)\n\n")
		sb.WriteString("> ")
		// Indent summary lines for blockquote
		summaryLines := strings.Split(specStub.Summary, "\n")
		sb.WriteString(strings.Join(summaryLines, "\n> "))
		sb.WriteString("\n\n")
		sb.WriteString("Use `read_spec` tool for full content.\n\n")
	}

	// Reply context - tells planner which thread to reply to
	if triggerThreadID != "" {
		sb.WriteString("# Reply Context\n\n")
		sb.WriteString(fmt.Sprintf("This engagement was triggered by a message in thread `%s`. ", triggerThreadID))
		sb.WriteString("Always use `reply_to_id: \"" + triggerThreadID + "\"` to keep the conversation in the same thread.\n\n")
	}

	return sb.String()
}

// formatGapsSection creates markdown for open gaps grouped by severity.
func formatGapsSection(issue model.Issue, gaps []model.Gap) string {
	if len(gaps) == 0 {
		return ""
	}

	// Group gaps by severity (already ordered by severity from store)
	bySeverity := make(map[model.GapSeverity][]model.Gap)
	for _, g := range gaps {
		bySeverity[g.Severity] = append(bySeverity[g.Severity], g)
	}

	var sb strings.Builder
	sb.WriteString("# Open Gaps\n\n")

	// Process in severity order: blocking > high > medium > low
	severityOrder := []model.GapSeverity{
		model.GapSeverityBlocking,
		model.GapSeverityHigh,
		model.GapSeverityMedium,
		model.GapSeverityLow,
	}

	for _, sev := range severityOrder {
		gapsForSev := bySeverity[sev]
		if len(gapsForSev) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("## %s\n\n", strings.ToUpper(string(sev))))

		for i, g := range gapsForSev {
			gapID := g.ShortID
			if gapID == 0 {
				gapID = g.ID
			}
			sb.WriteString(fmt.Sprintf("%d. [gap %s] [for %s] %s\n", i+1, strconv.FormatInt(gapID, 10), formatGapRespondent(issue, g.Respondent), g.Question))
			if g.Evidence != "" {
				sb.WriteString(fmt.Sprintf("   Evidence: %s\n", g.Evidence))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func formatClosedGapsSection(issue model.Issue, gaps []model.Gap) string {
	if len(gaps) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Recently Closed Gaps (latest 10)\n\n")

	for i, g := range gaps {
		gapID := g.ShortID
		if gapID == 0 {
			gapID = g.ID
		}
		sb.WriteString(fmt.Sprintf("%d. [gap %s] [%s] %s", i+1, strconv.FormatInt(gapID, 10), strings.ToUpper(string(g.Status)), g.Question))
		if g.Respondent != "" {
			sb.WriteString(fmt.Sprintf(" (for %s)", formatGapRespondent(issue, g.Respondent)))
		}
		sb.WriteString("\n")
		if g.Evidence != "" {
			sb.WriteString(fmt.Sprintf("   Evidence: %s\n", g.Evidence))
		}
		if g.ClosedReason != "" {
			sb.WriteString(fmt.Sprintf("   Closed reason: %s", g.ClosedReason))
			if g.ClosedNote != "" {
				sb.WriteString(fmt.Sprintf(" — %s", g.ClosedNote))
			}
			sb.WriteString("\n")
		}
		if g.ResolvedAt != nil {
			sb.WriteString(fmt.Sprintf("   Closed at: %s\n", g.ResolvedAt.UTC().Format(time.RFC3339)))
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

func formatGapRespondent(issue model.Issue, respondent model.GapRespondent) string {
	switch respondent {
	case model.GapRespondentReporter:
		if issue.Reporter != nil && *issue.Reporter != "" {
			return fmt.Sprintf("reporter (@%s)", *issue.Reporter)
		}
		return "reporter"
	case model.GapRespondentAssignee:
		if len(issue.Assignees) == 1 {
			return fmt.Sprintf("assignee (@%s)", issue.Assignees[0])
		}
		if len(issue.Assignees) > 1 {
			tags := make([]string, len(issue.Assignees))
			for i, a := range issue.Assignees {
				tags[i] = "@" + a
			}
			return fmt.Sprintf("assignees (%s)", strings.Join(tags, ", "))
		}
		return "assignee"
	default:
		return string(respondent)
	}
}

// buildDiscussionMessages converts discussions to LLM messages.
// Relay's comments become assistant messages, others become user messages with name.
func (b *contextBuilder) buildDiscussionMessages(discussions []model.Discussion, relayUsername string) []llm.Message {
	if len(discussions) == 0 {
		return nil
	}

	// Sort by creation time (oldest first)
	sorted := make([]model.Discussion, len(discussions))
	copy(sorted, discussions)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt.Before(sorted[j].CreatedAt)
	})

	// Truncate to max discussions (keep most recent)
	if len(sorted) > maxDiscussions {
		sorted = sorted[len(sorted)-maxDiscussions:]
	}

	// Track thread roots for reply context
	threadRoots := make(map[string]string) // threadID -> first author

	messages := make([]llm.Message, 0, len(sorted))

	for _, d := range sorted {
		content := d.Body

		// Handle reply context
		if d.ThreadID != nil && *d.ThreadID != "" {
			if rootAuthor, exists := threadRoots[*d.ThreadID]; exists {
				// This is a reply - prepend context
				content = fmt.Sprintf("(replying to @%s) %s", rootAuthor, d.Body)
			} else {
				// This is a thread root - record it
				threadRoots[*d.ThreadID] = d.Author
			}
		}

		if b.isRelayAuthor(d.Author, relayUsername) {
			messages = append(messages, llm.Message{
				Role:    "assistant",
				Content: content,
			})
		} else {
			messages = append(messages, llm.Message{
				Role:    "user",
				Name:    llm.SanitizeName(d.Author),
				Content: content,
			})
		}
	}

	return messages
}

// isRelayAuthor checks if the author matches Relay's identity.
// Handles both username match and "id:123" format used by some providers.
func (b *contextBuilder) isRelayAuthor(author, relayUsername string) bool {
	return strings.EqualFold(author, relayUsername)
}
