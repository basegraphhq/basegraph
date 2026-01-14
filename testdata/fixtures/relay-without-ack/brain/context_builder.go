package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"basegraph.app/relay/common/llm"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/store"
)

const maxDiscussions = 100

// contextBuilder constructs the LLM message thread for Planner.
// It fetches workspace-level learnings and formats discussions as a proper conversation.
type contextBuilder struct {
	integrations store.IntegrationStore
	configs      store.IntegrationConfigStore
	learnings    store.LearningStore
}

// NewContextBuilder creates a ContextBuilder with required store dependencies.
func NewContextBuilder(
	integrations store.IntegrationStore,
	configs store.IntegrationConfigStore,
	learnings store.LearningStore,
) *contextBuilder {
	return &contextBuilder{
		integrations: integrations,
		configs:      configs,
		learnings:    learnings,
	}
}

// BuildPlannerMessages constructs the full message thread for Planner.
// Returns messages in order: system prompt, context dump, discussion history.
func (b *contextBuilder) BuildPlannerMessages(ctx context.Context, issue model.Issue) ([]llm.Message, error) {
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

	messages := make([]llm.Message, 0, 3+len(issue.Discussions))

	// Message 1: System prompt with self-identity
	messages = append(messages, llm.Message{
		Role:    "system",
		Content: b.buildSystemPrompt(relayUsername),
	})

	// Message 2: Context dump (issue metadata, participants, learnings, findings)
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: b.buildContextDump(issue, learnings),
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

// buildSystemPrompt creates the system message with Relay's identity.
func (b *contextBuilder) buildSystemPrompt(relayUsername string) string {
	return fmt.Sprintf(`%s

# Self-Identity

Your comments appear as @%s. When you see messages from @%s in the discussion history, those are YOUR previous messages.`, plannerSystemPrompt, relayUsername, relayUsername)
}

// buildContextDump creates the context message with issue metadata, learnings, and findings.
func (b *contextBuilder) buildContextDump(issue model.Issue, learnings []model.Learning) string {
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

	return sb.String()
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
