package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"basegraph.app/relay/common/llm"
	"basegraph.app/relay/common/logger"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/store"
)

const (
	specGeneratorTimeout     = 15 * time.Minute // Must exceed exploreTimeout (12m) + LLM call buffer
	specGeneratorMaxAttempts = 3
)

// IssueSnapshot contains issue metadata for spec generation.
type IssueSnapshot struct {
	ID              int64
	ExternalIssueID string
	Provider        string
	Title           string
	Description     string
	Labels          []string
	Assignees       []string
	Reporter        string
	ExternalURL     string
}

// GapSnapshot contains gap data for spec generation.
type GapSnapshot struct {
	ID           int64
	Question     string
	ClosedReason string
	ClosedNote   string
}

// FindingSnapshot contains code finding data for spec generation.
type FindingSnapshot struct {
	ID        string
	Synthesis string
	Sources   []model.CodeSource
	IsCore    bool // Planner flagged this as directly relevant to the spec
}

// LearningSnapshot contains learning data for spec generation.
type LearningSnapshot struct {
	Type    string
	Content string
}

// SpecConstraints contains constraints for spec generation.
type SpecConstraints struct {
	MaxLength      int
	ComplexityHint model.SpecComplexity
}

// SpecGeneratorInput contains all context needed for spec generation.
type SpecGeneratorInput struct {
	Issue            IssueSnapshot
	ProceedSignal    string
	ContextSummary   string
	ClosedGaps       []GapSnapshot
	RelevantFindings []FindingSnapshot
	Learnings        []LearningSnapshot
	Discussions      []model.ConversationMessage // Full conversation thread
	ExistingSpec     *string
	ExistingSpecRef  *model.SpecRef
	Constraints      SpecConstraints
}

// SpecGeneratorOutput contains the generated spec and metadata.
type SpecGeneratorOutput struct {
	SpecMarkdown     string
	SpecSummary      string
	Changelog        string
	Metadata         model.SpecMetadataJSON
	ValidationErrors []model.ValidationError
}

// SubmitSpecParams defines the schema for the submit_spec tool.
type SubmitSpecParams struct {
	SpecMarkdown string `json:"spec_markdown" jsonschema:"required,description=Full spec in markdown format."`
	SpecSummary  string `json:"spec_summary" jsonschema:"required,description=5-10 bullet summary of the spec."`
	Changelog    string `json:"changelog" jsonschema:"required,description=What changed from previous version. Use empty string if this is a new spec."`
}

// SpecGenerator is a dedicated agent for generating specs from gathered context.
// It runs in a fresh context window to avoid polluting the Planner's context.
type SpecGenerator struct {
	llm       llm.AgentClient
	explore   *ExploreAgent
	specStore store.SpecStore
	debugDir  string
}

// NewSpecGenerator creates a SpecGenerator agent.
func NewSpecGenerator(llmClient llm.AgentClient, explore *ExploreAgent, specStore store.SpecStore, debugDir string) *SpecGenerator {
	return &SpecGenerator{
		llm:       llmClient,
		explore:   explore,
		specStore: specStore,
		debugDir:  debugDir,
	}
}

// Generate creates a spec from the provided input context.
func (g *SpecGenerator) Generate(ctx context.Context, input SpecGeneratorInput) (SpecGeneratorOutput, error) {
	start := time.Now()
	sessionID := time.Now().Format("20060102-150405")

	ctx = logger.WithLogFields(ctx, logger.LogFields{
		Component: "relay.brain.spec_generator",
	})

	ctx, cancel := context.WithTimeout(ctx, specGeneratorTimeout)
	defer cancel()

	var debugLog strings.Builder
	debugLog.WriteString(fmt.Sprintf("=== SPEC GENERATOR SESSION %s ===\n", sessionID))
	debugLog.WriteString(fmt.Sprintf("Issue: %s (ID: %d)\n", input.Issue.Title, input.Issue.ID))
	debugLog.WriteString(fmt.Sprintf("Complexity hint: %s\n", input.Constraints.ComplexityHint.String()))
	debugLog.WriteString(fmt.Sprintf("Existing spec: %v\n\n", input.ExistingSpec != nil))

	slog.InfoContext(ctx, "spec generator starting",
		"issue_id", input.Issue.ID,
		"issue_title", logger.Truncate(input.Issue.Title, 50),
		"closed_gaps", len(input.ClosedGaps),
		"findings", len(input.RelevantFindings),
		"has_existing_spec", input.ExistingSpec != nil)

	// Build system prompt
	systemPrompt := g.systemPrompt(input)

	// Build user message with context
	userMessage := g.buildUserMessage(input)

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	debugLog.WriteString("[SYSTEM PROMPT]\n")
	debugLog.WriteString(logger.Truncate(systemPrompt, 2000))
	debugLog.WriteString("\n\n[USER MESSAGE]\n")
	debugLog.WriteString(logger.Truncate(userMessage, 3000))
	debugLog.WriteString("\n\n")

	var output SpecGeneratorOutput
	exploreCount := 0
	const maxExploreAttempts = 2

	for attempt := 1; attempt <= specGeneratorMaxAttempts; attempt++ {
		slog.DebugContext(ctx, "spec generator iteration", "attempt", attempt, "explore_count", exploreCount)

		// Dynamically gate tools: once explore limit is reached, force submit_spec only
		tools := g.tools()
		if exploreCount >= maxExploreAttempts {
			tools = g.submitOnlyTools()
		}

		resp, err := g.llm.ChatWithTools(ctx, llm.AgentRequest{
			Messages: messages,
			Tools:    tools,
		})
		if err != nil {
			g.writeDebugLog(sessionID, debugLog.String())
			return SpecGeneratorOutput{}, fmt.Errorf("spec generator chat attempt %d: %w", attempt, err)
		}

		debugLog.WriteString(fmt.Sprintf("--- ATTEMPT %d ---\n", attempt))
		debugLog.WriteString(fmt.Sprintf("[ASSISTANT] (tokens: prompt=%d, completion=%d)\n%s\n\n",
			resp.PromptTokens, resp.CompletionTokens, logger.Truncate(resp.Content, 1000)))

		// Look for submit_spec tool call
		for _, tc := range resp.ToolCalls {
			if tc.Name == "submit_spec" {
				params, err := llm.ParseToolArguments[SubmitSpecParams](tc.Arguments)
				if err != nil {
					debugLog.WriteString(fmt.Sprintf("[ERROR] parsing submit_spec: %s\n", err))
					// Add error feedback and retry
					messages = append(messages, llm.Message{
						Role:      "assistant",
						Content:   resp.Content,
						ToolCalls: resp.ToolCalls,
					})
					messages = append(messages, llm.Message{
						Role:       "tool",
						Content:    fmt.Sprintf("Error parsing submit_spec: %s. Please try again.", err),
						ToolCallID: tc.ID,
					})
					continue
				}

				// Validate the spec
				complexity := inferComplexity(input)
				validationErrors := validateSpec(params.SpecMarkdown, complexity)

				output = SpecGeneratorOutput{
					SpecMarkdown:     params.SpecMarkdown,
					SpecSummary:      params.SpecSummary,
					Changelog:        params.Changelog,
					Metadata:         extractSpecMetadata(params.SpecMarkdown),
					ValidationErrors: validationErrors,
				}

				// Check for fatal validation errors
				hasErrors := false
				for _, ve := range validationErrors {
					if ve.Severity == "error" {
						hasErrors = true
						break
					}
				}

				if hasErrors && attempt < specGeneratorMaxAttempts {
					// Provide feedback to fix validation errors
					var feedback strings.Builder
					feedback.WriteString("Spec validation failed. Please fix these issues:\n\n")
					for _, ve := range validationErrors {
						feedback.WriteString(fmt.Sprintf("- [%s] %s: %s\n", ve.Severity, ve.Rule, ve.Detail))
					}
					feedback.WriteString("\nCall submit_spec again with the corrected spec.")

					messages = append(messages, llm.Message{
						Role:      "assistant",
						Content:   resp.Content,
						ToolCalls: resp.ToolCalls,
					})
					messages = append(messages, llm.Message{
						Role:       "tool",
						Content:    feedback.String(),
						ToolCallID: tc.ID,
					})

					debugLog.WriteString(fmt.Sprintf("[VALIDATION FAILED]\n%s\n", feedback.String()))
					continue
				}

				// Success (or max attempts reached)
				debugLog.WriteString(fmt.Sprintf("=== SPEC GENERATOR COMPLETED ===\n"))
				debugLog.WriteString(fmt.Sprintf("Duration: %dms\n", time.Since(start).Milliseconds()))
				debugLog.WriteString(fmt.Sprintf("Validation errors: %d\n", len(validationErrors)))
				g.writeDebugLog(sessionID, debugLog.String())

				slog.InfoContext(ctx, "spec generator completed",
					"issue_id", input.Issue.ID,
					"duration_ms", time.Since(start).Milliseconds(),
					"spec_length", len(params.SpecMarkdown),
					"validation_errors", len(validationErrors))

				return output, nil
			}
		}

		// Handle explore tool calls (optional, for verification)
		if len(resp.ToolCalls) > 0 && resp.ToolCalls[0].Name == "explore" {
			messages = append(messages, llm.Message{
				Role:      "assistant",
				Content:   resp.Content,
				ToolCalls: resp.ToolCalls,
			})

			for _, tc := range resp.ToolCalls {
				if tc.Name != "explore" {
					continue
				}

				exploreCount++

				// Enforce explore limit to prevent burning all attempts
				if exploreCount > maxExploreAttempts {
					debugLog.WriteString(fmt.Sprintf("[EXPLORE LIMIT] Attempt %d exceeded max %d\n", exploreCount, maxExploreAttempts))
					messages = append(messages, llm.Message{
						Role:       "tool",
						Content:    "Explore limit reached (max 2 calls). Please call submit_spec with your best effort based on the context provided.",
						ToolCallID: tc.ID,
					})
					continue
				}

				params, err := llm.ParseToolArguments[ExploreParams](tc.Arguments)
				if err != nil {
					messages = append(messages, llm.Message{
						Role:       "tool",
						Content:    fmt.Sprintf("Error: %s", err),
						ToolCallID: tc.ID,
					})
					continue
				}

				// Quick exploration only
				report, err := g.explore.Explore(ctx, params.Query, ThoroughnessQuick)
				if err != nil {
					report = fmt.Sprintf("Explore error: %s", err)
				}

				debugLog.WriteString(fmt.Sprintf("[EXPLORE %d/%d] %s\n", exploreCount, maxExploreAttempts, logger.Truncate(params.Query, 100)))
				debugLog.WriteString(fmt.Sprintf("[RESULT] %s\n\n", logger.Truncate(report, 500)))

				messages = append(messages, llm.Message{
					Role:       "tool",
					Content:    report,
					ToolCallID: tc.ID,
				})
			}
			continue
		}

		// No tool calls - unexpected
		if len(resp.ToolCalls) == 0 {
			messages = append(messages, llm.Message{
				Role:    "assistant",
				Content: resp.Content,
			})
			messages = append(messages, llm.Message{
				Role:    "user",
				Content: "Please call submit_spec with the generated spec.",
			})
		}
	}

	g.writeDebugLog(sessionID, debugLog.String())
	return output, fmt.Errorf("spec generator failed after %d attempts", specGeneratorMaxAttempts)
}

func (g *SpecGenerator) tools() []llm.Tool {
	tools := []llm.Tool{
		{
			Name:        "submit_spec",
			Description: "Submit the final spec. Call this exactly once when the spec is complete.",
			Parameters:  llm.GenerateSchemaFrom(SubmitSpecParams{}),
			Strict:      true,
		},
	}

	// Add explore for optional verification
	if g.explore != nil {
		tools = append(tools, llm.Tool{
			Name:        "explore",
			Description: "Quick codebase lookup to verify exact symbol names, file paths, or config keys. Use sparingly (1-2 calls max).",
			Parameters:  llm.GenerateSchemaFrom(ExploreParams{}),
		})
	}

	return tools
}

// submitOnlyTools returns the minimal toolset that forces the model to finalize the spec.
func (g *SpecGenerator) submitOnlyTools() []llm.Tool {
	return []llm.Tool{
		{
			Name:        "submit_spec",
			Description: "Submit the final spec. Call this exactly once when the spec is complete.",
			Parameters:  llm.GenerateSchemaFrom(SubmitSpecParams{}),
			Strict:      true,
		},
	}
}

func (g *SpecGenerator) buildUserMessage(input SpecGeneratorInput) string {
	var sb strings.Builder

	sb.WriteString("# Issue Context\n\n")
	sb.WriteString(fmt.Sprintf("**Title:** %s\n", input.Issue.Title))
	if input.Issue.Description != "" {
		sb.WriteString(fmt.Sprintf("**Description:**\n%s\n\n", input.Issue.Description))
	}
	if input.Issue.ExternalURL != "" {
		sb.WriteString(fmt.Sprintf("**URL:** %s\n", input.Issue.ExternalURL))
	}
	if len(input.Issue.Labels) > 0 {
		sb.WriteString(fmt.Sprintf("**Labels:** %s\n", strings.Join(input.Issue.Labels, ", ")))
	}

	// Ownership information for spec metadata
	sb.WriteString("\n## Ownership\n")
	if input.Issue.Reporter != "" {
		sb.WriteString(fmt.Sprintf("**Reporter:** @%s\n", input.Issue.Reporter))
	}
	if len(input.Issue.Assignees) > 0 {
		sb.WriteString(fmt.Sprintf("**Reviewers:** %s\n", formatAsHandles(input.Issue.Assignees)))
	} else {
		sb.WriteString("**Reviewers:** TBD\n")
	}
	sb.WriteString("**Author:** Relay (automated)\n")
	sb.WriteString("\n")

	sb.WriteString("# Proceed Signal\n\n")
	sb.WriteString(fmt.Sprintf("> %s\n\n", input.ProceedSignal))

	sb.WriteString("# Context Summary (from Planner)\n\n")
	sb.WriteString(input.ContextSummary)
	sb.WriteString("\n\n")

	if len(input.ClosedGaps) > 0 {
		sb.WriteString("# Resolved Gaps\n\n")
		sb.WriteString("*IMPORTANT: When referencing these gaps in the spec, INLINE the question and answer. Do not just write 'Gap #1'.*\n\n")
		for i, gap := range input.ClosedGaps {
			sb.WriteString(fmt.Sprintf("## Gap %d (ID: %d)\n", i+1, gap.ID))
			sb.WriteString(fmt.Sprintf("**Question:** %s\n", gap.Question))
			sb.WriteString(fmt.Sprintf("**Resolution (%s):** %s\n\n", gap.ClosedReason, gap.ClosedNote))
		}
	}

	if len(input.RelevantFindings) > 0 {
		sb.WriteString("# Code Findings\n\n")
		sb.WriteString("*IMPORTANT: When referencing these findings in the spec, include the file path and key details inline. Use these code snippets to inform gotchas and best practices.*\n\n")

		// Separate core (Planner-flagged) and supporting findings
		var coreFindings, supportingFindings []FindingSnapshot
		for _, f := range input.RelevantFindings {
			if f.IsCore {
				coreFindings = append(coreFindings, f)
			} else {
				supportingFindings = append(supportingFindings, f)
			}
		}

		// Core findings with full detail including code snippets
		if len(coreFindings) > 0 {
			sb.WriteString("## Core Findings (directly relevant)\n\n")
			for i, f := range coreFindings {
				sb.WriteString(fmt.Sprintf("### Finding F%d (ID: %s)\n", i+1, f.ID))
				sb.WriteString(fmt.Sprintf("**Synthesis:** %s\n\n", f.Synthesis))
				for _, s := range f.Sources {
					sb.WriteString(fmt.Sprintf("**%s**", s.Location))
					if s.Kind != "" {
						sb.WriteString(fmt.Sprintf(" (%s)", s.Kind))
					}
					sb.WriteString("\n")
					if s.Snippet != "" {
						sb.WriteString("```go\n")
						sb.WriteString(s.Snippet)
						sb.WriteString("\n```\n\n")
					}
				}
			}
		}

		// Supporting findings (context, less detail)
		if len(supportingFindings) > 0 {
			sb.WriteString("## Supporting Findings (additional context)\n\n")
			for i, f := range supportingFindings {
				sb.WriteString(fmt.Sprintf("### Finding F%d (ID: %s)\n", len(coreFindings)+i+1, f.ID))
				if len(f.Sources) > 0 {
					locations := make([]string, 0, len(f.Sources))
					for _, s := range f.Sources {
						locations = append(locations, s.Location)
					}
					sb.WriteString(fmt.Sprintf("**Location(s):** %s\n", strings.Join(locations, ", ")))
				}
				sb.WriteString(fmt.Sprintf("**Synthesis:** %s\n\n", f.Synthesis))
			}
		}
	}

	if len(input.Learnings) > 0 {
		sb.WriteString("# Workspace Learnings\n\n")
		for _, l := range input.Learnings {
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", l.Type, l.Content))
		}
		sb.WriteString("\n")
	}

	// Add conversation thread in provider-agnostic XML format
	if len(input.Discussions) > 0 {
		sb.WriteString("# Conversation Thread\n\n")
		sb.WriteString("<conversation>\n")
		for _, msg := range input.Discussions {
			sb.WriteString(fmt.Sprintf(`  <msg n="%d" author="%s" role="%s" ts="%s"`,
				msg.Seq, msg.Author, msg.Role, msg.Timestamp.Format(time.RFC3339)))
			if msg.ReplyToSeq != nil {
				sb.WriteString(fmt.Sprintf(` reply_to="%d"`, *msg.ReplyToSeq))
			}
			if msg.AnswersGapID != nil {
				sb.WriteString(fmt.Sprintf(` answers_gap="%d"`, *msg.AnswersGapID))
			}
			if msg.IsProceed {
				sb.WriteString(` is_proceed="true"`)
			}
			sb.WriteString(">\n")
			sb.WriteString(msg.Content)
			sb.WriteString("\n  </msg>\n")
		}
		sb.WriteString("</conversation>\n\n")
	}

	if input.ExistingSpec != nil {
		sb.WriteString("# Existing Spec (for revision)\n\n")
		sb.WriteString("```markdown\n")
		sb.WriteString(*input.ExistingSpec)
		sb.WriteString("\n```\n\n")
		sb.WriteString("**Task:** Revise the spec above based on the new context. Include a Changelog section at the end.\n")
	} else {
		sb.WriteString("**Task:** Generate a new spec based on the context above.\n")
	}

	return sb.String()
}

// formatAsHandles converts a list of usernames to @-prefixed handles.
func formatAsHandles(names []string) string {
	handles := make([]string, len(names))
	for i, n := range names {
		handles[i] = "@" + n
	}
	return strings.Join(handles, ", ")
}

func (g *SpecGenerator) systemPrompt(input SpecGeneratorInput) string {
	complexity := inferComplexity(input)

	return fmt.Sprintf(`You are a staff engineer writing an implementation guide that will wow devs and PMs.
Not a spec — a document so complete that an AI agent can one-shot the implementation with zero clarification.

# Quality Bar (S-tier)

Your spec must:
- Cover 100%% of user requirements — every answer they gave is addressed
- Be copy-paste executable — code snippets work, file paths are real
- Anticipate questions — gotchas, edge cases, "what about X?" all answered
- Show deep understanding — reference actual code patterns from findings
- Be self-contained — reader never needs to look elsewhere

A dev reading this should think: "This person really understood the codebase and the requirements."

# Principles

1. **Show the code**: Include actual snippets from findings. Current state → proposed changes. Copy-paste ready.
2. **Inline everything**: Never "Gap #2" alone. Write "Gap #2: 'question' → 'answer'". Self-contained.
3. **Concrete over abstract**: GIVEN/WHEN/THEN with real data. Tests with actual fixtures.
4. **Mine the findings**: Extract gotchas, edge cases, patterns from code snippets. Prevent bugs before they're written.
5. **Production-ready**: Answer: How know it works? How know it's broken? How rollback?
6. **Complete scope**: Every user requirement addressed. Nothing falls through the cracks.

# Complexity: %s

%s

# Template

%s

# Context from Planner

The Context Summary and Code Findings below are the result of the Planner's exploration.
DO NOT re-explore topics already covered in findings (auth patterns, data models, existing APIs).
Explore ONLY for: exact symbol names, method signatures, or config keys not in findings.
If findings cover what you need, use them directly and call submit_spec.

# Requirements Extraction (critical)

Your spec MUST address every requirement. Missing even one = incomplete spec.

Sources of requirements (in priority order):
1. USER REQUIREMENTS section in Context Summary — these are non-negotiable, address each explicitly
2. Resolved Gaps — each gap's question + answer is a requirement
3. Conversation Thread (fallback) — if gaps are sparse, scan for:
   - Numbered answers from users (e.g., "1. yes, 2. option B, 3. both")
   - Direct answers to Relay's explicit questions
   - User decisions stated in replies

Before calling submit_spec, verify coverage:
- List all user requirements you found
- Check each one appears in your spec
- If something is deferred, add "Out of Scope" section explaining why

If you cannot find a key requirement (auth method, API shape, error handling), DO NOT guess.
Note it as "TBD — requires clarification" in the spec.

# Output

Call submit_spec(spec_markdown, spec_summary, changelog).
Optional: explore(query) for 1-2 quick lookups ONLY if findings are insufficient.
`, complexity.String(), complexityGuidance(complexity), specTemplate(complexity))
}

func complexityGuidance(c model.SpecComplexity) string {
	// Single guidance for all complexity levels (v1)
	// Complexity is informational, not a section selector
	return `Complexity: ` + c.String() + `

Include ALL sections. Every spec needs:
- Code snippets (current state + proposed changes) — copy-paste ready
- Implementation steps with file paths and done-when criteria
- Tests with GIVEN/WHEN/THEN fixtures
- Gotchas mined from code findings
- Operations (verify, monitor, rollback)
- Decisions with inlined gap context (Gap #N: "question" → "answer")
- Alternatives considered (even if brief — "considered X, rejected because Y")
- Assumptions with fallback actions

The goal: an AI agent can one-shot this implementation with no clarification needed.`
}

func specTemplate(complexity model.SpecComplexity) string {
	// Single template for all complexity levels (v1)
	// Complexity is metadata, not a template selector
	return `# {Issue Title}

**Issue:** {url} | **Complexity:** ` + complexity.String() + ` | **Author:** Relay | **Reviewers:** {assignees}

## TL;DR
{5 bullets: outcome, approach, key constraint, risk, validation}

## What We're Building
{Problem context — reference findings, link to why this matters}

## Code Changes

### Current State
{Existing code from findings — actual snippets showing what exists today}

### Proposed Changes
{New code to write — copy-paste ready, with file paths}

### Key Types/Interfaces
{Definitions the implementation must satisfy — if applicable}

## Implementation
| # | Task | File | Done When | Blocked By |
|---|------|------|-----------|------------|
| 1 | {task} | {file:line} | {criteria} | - |

## Tests
- [ ] Unit: GIVEN {fixture} WHEN {action} THEN {expected}
- [ ] Integration: {e2e scenario}
- [ ] Edge case: GIVEN {error condition} THEN {graceful handling}

## Gotchas
{Patterns, edge cases, constraints from code — prevent bugs before they're written}

## Operations
- **Verify:** {how to confirm it works}
- **Monitor:** {what to watch in prod}
- **Rollback:** {how to revert if needed}

## Decisions
| Decision | Why | Trade-offs |
|----------|-----|------------|
| {choice} | Gap #N: "{question}" → "{answer}" | {impact} |

## Alternatives Considered
| Option | Pros | Cons | Why Not |
|--------|------|------|---------|
| {alternative approach} | {benefits} | {drawbacks} | {reason rejected} |

## Assumptions
| Assumption | If Wrong |
|------------|----------|
| {assumption} | {fallback action} |`
}

func inferComplexity(input SpecGeneratorInput) model.SpecComplexity {
	if input.Constraints.ComplexityHint != 0 {
		return input.Constraints.ComplexityHint
	}

	// Infer from context
	gapCount := len(input.ClosedGaps)
	findingCount := len(input.RelevantFindings)
	total := gapCount + findingCount

	switch {
	case total <= 2:
		return model.SpecComplexityBugFix
	case total <= 4:
		return model.SpecComplexitySmallFeature
	case total <= 8:
		return model.SpecComplexityMediumFeature
	case total <= 15:
		return model.SpecComplexityLargeFeature
	default:
		return model.SpecComplexityArchitectural
	}
}

// validateSpec performs structural validation on the spec.
// Same rules for all complexity levels (v1).
func validateSpec(markdown string, _ model.SpecComplexity) []model.ValidationError {
	var errors []model.ValidationError

	// Required sections
	required := []struct {
		section, rule, detail string
	}{
		{"## TL;DR", "has_tldr", "TL;DR section required"},
		{"## What We're Building", "has_context", "What We're Building section required"},
		{"## Implementation", "has_implementation", "Implementation section required"},
	}

	for _, r := range required {
		if !strings.Contains(markdown, r.section) {
			errors = append(errors, model.ValidationError{
				Rule:     r.rule,
				Severity: "error",
				Detail:   r.detail,
			})
		}
	}

	// Code blocks required for all specs (this is a handoff document!)
	codeBlockCount := strings.Count(markdown, "```")
	if codeBlockCount < 2 { // At least one opening + closing
		errors = append(errors, model.ValidationError{
			Rule:     "has_code_blocks",
			Severity: "error",
			Detail:   "Spec must include code snippets",
		})
	}

	// GIVEN/THEN scenarios required for all
	hasScenario := strings.Contains(markdown, "GIVEN") && strings.Contains(markdown, "THEN")
	if !hasScenario {
		errors = append(errors, model.ValidationError{
			Rule:     "has_scenarios",
			Severity: "error",
			Detail:   "Tests must have GIVEN/WHEN/THEN scenarios",
		})
	}

	// Decisions section recommended
	if !strings.Contains(markdown, "## Decisions") {
		errors = append(errors, model.ValidationError{
			Rule:     "has_decisions",
			Severity: "warning",
			Detail:   "Decisions section recommended",
		})
	}

	// Alternatives section recommended
	if !strings.Contains(markdown, "## Alternatives") {
		errors = append(errors, model.ValidationError{
			Rule:     "has_alternatives",
			Severity: "warning",
			Detail:   "Alternatives Considered section recommended",
		})
	}

	// Gap inlining check
	gapRefPattern := regexp.MustCompile(`Gap #\d+[^:\n"'→]`)
	if gapRefPattern.MatchString(markdown) {
		errors = append(errors, model.ValidationError{
			Rule:     "gaps_inlined",
			Severity: "warning",
			Detail:   "Gap references should inline context",
		})
	}

	return errors
}

func extractSpecMetadata(markdown string) model.SpecMetadataJSON {
	meta := model.SpecMetadataJSON{
		CharCount: len(markdown),
	}

	// Extract section headers
	headerRe := regexp.MustCompile(`(?m)^##\s+(.+)$`)
	matches := headerRe.FindAllStringSubmatch(markdown, -1)
	for _, m := range matches {
		meta.Sections = append(meta.Sections, m[1])
	}

	// Count decisions (table rows in Decisions section)
	if idx := strings.Index(markdown, "## Decisions"); idx != -1 {
		section := markdown[idx:]
		if endIdx := strings.Index(section[1:], "\n##"); endIdx != -1 {
			section = section[:endIdx+1]
		}
		meta.DecisionCount = strings.Count(section, "\n|") - 1 // subtract header row
		meta.DecisionCount = max(meta.DecisionCount, 0)
	}

	// Count assumptions
	if idx := strings.Index(markdown, "## Assumptions"); idx != -1 {
		section := markdown[idx:]
		if endIdx := strings.Index(section[1:], "\n##"); endIdx != -1 {
			section = section[:endIdx+1]
		}
		meta.AssumptionCount = strings.Count(section, "\n|") - 1
		meta.AssumptionCount = max(meta.AssumptionCount, 0)
	}

	// Count tasks (Implementation section)
	if idx := strings.Index(markdown, "## Implementation"); idx != -1 {
		section := markdown[idx:]
		if endIdx := strings.Index(section[1:], "\n##"); endIdx != -1 {
			section = section[:endIdx+1]
		}
		meta.TaskCount = strings.Count(section, "\n|") - 1
		meta.TaskCount = max(meta.TaskCount, 0)
	}

	// Count test cases
	meta.TestCaseCount = strings.Count(markdown, "- [ ]")

	return meta
}

func (g *SpecGenerator) writeDebugLog(sessionID, content string) {
	if g.debugDir == "" {
		return
	}

	if err := os.MkdirAll(g.debugDir, 0o755); err != nil {
		slog.Warn("failed to create debug dir", "dir", g.debugDir, "error", err)
		return
	}

	filename := filepath.Join(g.debugDir, fmt.Sprintf("spec_generator_%s.txt", sessionID))
	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		slog.Warn("failed to write debug log", "file", filename, "error", err)
	}
}

// BuildSpecGeneratorInput constructs the input from action data and issue context.
func BuildSpecGeneratorInput(
	issue model.Issue,
	action ReadyForSpecGenerationAction,
	closedGaps []model.Gap,
	learnings []model.Learning,
	existingSpec *string,
	existingSpecRef *model.SpecRef,
	complexityHint model.SpecComplexity,
	relayUsername string,
) SpecGeneratorInput {
	input := SpecGeneratorInput{
		Issue: IssueSnapshot{
			ID:              issue.ID,
			ExternalIssueID: issue.ExternalIssueID,
			Provider:        string(issue.Provider),
			Labels:          issue.Labels,
			Assignees:       issue.Assignees,
		},
		ProceedSignal:   action.ProceedSignal,
		ContextSummary:  action.ContextSummary,
		ExistingSpec:    existingSpec,
		ExistingSpecRef: existingSpecRef,
		Constraints: SpecConstraints{
			MaxLength:      store.MaxSpecSize,
			ComplexityHint: complexityHint,
		},
	}

	if issue.Title != nil {
		input.Issue.Title = *issue.Title
	}
	if issue.Description != nil {
		input.Issue.Description = *issue.Description
	}
	if issue.ExternalIssueURL != nil {
		input.Issue.ExternalURL = *issue.ExternalIssueURL
	}
	if issue.Reporter != nil {
		input.Issue.Reporter = *issue.Reporter
	}

	// Map closed gaps
	for _, gap := range closedGaps {
		input.ClosedGaps = append(input.ClosedGaps, GapSnapshot{
			ID:           gap.ID,
			Question:     gap.Question,
			ClosedReason: gap.ClosedReason,
			ClosedNote:   gap.ClosedNote,
		})
	}

	// Map ALL findings from issue — all findings are core (v1)
	// Planner can't provide real IDs at decision time, so we treat all as relevant
	for _, f := range issue.CodeFindings {
		input.RelevantFindings = append(input.RelevantFindings, FindingSnapshot{
			ID:        f.ID,
			Synthesis: f.Synthesis,
			Sources:   f.Sources,
			IsCore:    true, // All findings are core (v1)
		})
	}

	// Map learnings
	for _, l := range learnings {
		input.Learnings = append(input.Learnings, LearningSnapshot{
			Type:    l.Type,
			Content: l.Content,
		})
	}

	// Map discussions to provider-agnostic conversation messages
	input.Discussions = mapDiscussionsToConversation(issue, closedGaps, action.ProceedSignal, relayUsername)

	return input
}

// mapDiscussionsToConversation converts provider-specific discussions to a provider-agnostic format.
func mapDiscussionsToConversation(
	issue model.Issue,
	closedGaps []model.Gap,
	proceedSignal string,
	relayUsername string,
) []model.ConversationMessage {
	if len(issue.Discussions) == 0 {
		return nil
	}

	// Sort by timestamp (should already be sorted, but ensure)
	discussions := make([]model.Discussion, len(issue.Discussions))
	copy(discussions, issue.Discussions)
	sort.Slice(discussions, func(i, j int) bool {
		return discussions[i].CreatedAt.Before(discussions[j].CreatedAt)
	})

	// Build thread root map for reply_to tracking
	threadRoots := make(map[string]int) // threadID -> seq of first message in thread

	messages := make([]model.ConversationMessage, 0, len(discussions))

	for i, d := range discussions {
		seq := i + 1 // 1-indexed

		msg := model.ConversationMessage{
			Seq:       seq,
			Author:    d.Author,
			Role:      determineRole(d.Author, issue, relayUsername),
			Timestamp: d.CreatedAt,
			Content:   d.Body,
		}

		// Handle reply threading
		if d.ThreadID != nil && *d.ThreadID != "" {
			if rootSeq, exists := threadRoots[*d.ThreadID]; exists {
				msg.ReplyToSeq = &rootSeq
			} else {
				// First message in this thread
				threadRoots[*d.ThreadID] = seq
			}
		}

		messages = append(messages, msg)
	}

	// Apply heuristic annotations
	annotateGapAnswers(messages, closedGaps, issue)
	annotateProceedSignal(messages, proceedSignal)

	return messages
}

// determineRole determines the role of a message author.
func determineRole(author string, issue model.Issue, relayUsername string) string {
	if strings.EqualFold(author, relayUsername) {
		return model.RoleSelf
	}
	if issue.Reporter != nil && strings.EqualFold(author, *issue.Reporter) {
		return model.RoleReporter
	}
	for _, assignee := range issue.Assignees {
		if strings.EqualFold(author, assignee) {
			return model.RoleAssignee
		}
	}
	return model.RoleOther
}

// annotateGapAnswers marks messages that likely answered gaps using heuristics.
// Heuristic: Message author matches gap respondent AND timestamp in [gap.CreatedAt, gap.ResolvedAt].
func annotateGapAnswers(messages []model.ConversationMessage, closedGaps []model.Gap, issue model.Issue) {
	for _, gap := range closedGaps {
		if gap.ResolvedAt == nil {
			continue
		}

		for i := range messages {
			msg := &messages[i]
			if msg.AnswersGapID != nil {
				continue // Already annotated
			}

			// Check if message is in the gap's resolution window
			if msg.Timestamp.Before(gap.CreatedAt) || msg.Timestamp.After(*gap.ResolvedAt) {
				continue
			}

			// Check if author matches expected respondent
			if matchesRespondent(msg.Author, gap.Respondent, issue) {
				gapID := gap.ID
				msg.AnswersGapID = &gapID
				break // First match wins for this gap
			}
		}
	}
}

// matchesRespondent checks if an author matches the expected gap respondent.
func matchesRespondent(author string, respondent model.GapRespondent, issue model.Issue) bool {
	switch respondent {
	case model.GapRespondentReporter:
		return issue.Reporter != nil && strings.EqualFold(author, *issue.Reporter)
	case model.GapRespondentAssignee:
		for _, assignee := range issue.Assignees {
			if strings.EqualFold(author, assignee) {
				return true
			}
		}
	}
	return false
}

// annotateProceedSignal marks the message containing the proceed signal.
// Heuristic: Find message containing the proceed signal text (case-insensitive substring).
func annotateProceedSignal(messages []model.ConversationMessage, proceedSignal string) {
	if proceedSignal == "" {
		return
	}

	// Use first 50 chars of proceed signal for matching (avoid matching entire essays)
	matchText := proceedSignal
	if len(matchText) > 50 {
		matchText = matchText[:50]
	}
	matchText = strings.ToLower(matchText)

	// Find last message containing the signal (most recent is most relevant)
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.Contains(strings.ToLower(messages[i].Content), matchText) {
			messages[i].IsProceed = true
			return // Only mark one message
		}
	}
}

// WriteSpec persists the generated spec and updates the issue.
func (g *SpecGenerator) WriteSpec(ctx context.Context, issue model.Issue, output SpecGeneratorOutput) (model.SpecRef, error) {
	slug := "spec"
	if issue.Title != nil && *issue.Title != "" {
		slug = *issue.Title
	}

	ref, err := g.specStore.Write(ctx, issue.ID, string(issue.Provider), issue.ExternalIssueID, slug, output.SpecMarkdown)
	if err != nil {
		return model.SpecRef{}, fmt.Errorf("writing spec to store: %w", err)
	}

	// Compute and add SHA256 to metadata
	output.Metadata.SHA256 = ref.SHA256

	slog.InfoContext(ctx, "spec written",
		"issue_id", issue.ID,
		"path", ref.Path,
		"sha256", ref.SHA256,
		"char_count", output.Metadata.CharCount)

	return ref, nil
}

// SerializeSpecRef converts a SpecRef to JSON string for storage in issues.spec.
func SerializeSpecRef(ref model.SpecRef) (string, error) {
	data, err := json.Marshal(ref)
	if err != nil {
		return "", fmt.Errorf("serializing spec ref: %w", err)
	}
	return string(data), nil
}
