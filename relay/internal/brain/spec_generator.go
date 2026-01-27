package brain

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"basegraph.co/relay/common/llm"
	"basegraph.co/relay/common/logger"
	"basegraph.co/relay/internal/model"
)

const (
	maxSpecIterations        = 30 // Safety limit for spec generation loop
	maxParallelSpecExplorers = 5  // Parallel exploration during spec generation
	maxLocateCalls           = 8  // Hard cap on locate calls for spec generation
)

// SubmitSpecParams defines the schema for the submit_spec tool.
// Simplified to just the spec - confidence info lives in the spec's Confidence Assessment section.
type SubmitSpecParams struct {
	Spec string `json:"spec" jsonschema:"required,description=The complete implementation spec. MUST be valid markdown with proper newlines and formatting."`
}

// SpecGeneratorInput contains all context needed to generate a spec.
type SpecGeneratorInput struct {
	Issue          model.Issue
	ContextSummary string
	Gaps           []model.Gap
	Findings       []model.CodeFinding
	Learnings      []model.Learning
	ProceedSignal  string
}

// SpecGeneratorOutput contains the generated spec.
type SpecGeneratorOutput struct {
	Spec string
}

// SpecGenerator generates implementation specs from gathered context.
// It uses ExploreAgent to verify code references and ensure accuracy.
type SpecGenerator struct {
	llm      llm.AgentClient
	explore  *ExploreAgent
	debugDir string
}

// NewSpecGenerator creates a SpecGenerator with an ExploreAgent for code verification.
func NewSpecGenerator(llmClient llm.AgentClient, explore *ExploreAgent, debugDir string) *SpecGenerator {
	return &SpecGenerator{
		llm:      llmClient,
		explore:  explore,
		debugDir: debugDir,
	}
}

// Generate creates an implementation spec from the gathered context.
// Returns the spec markdown and confidence assessment.
func (s *SpecGenerator) Generate(ctx context.Context, input SpecGeneratorInput) (SpecGeneratorOutput, error) {
	start := time.Now()

	ctx = logger.WithLogFields(ctx, logger.LogFields{
		Component: "relay.brain.spec_generator",
	})

	sessionID := time.Now().Format("20060102-150405")
	var debugLog strings.Builder
	debugLog.WriteString(fmt.Sprintf("=== SPEC GENERATOR SESSION %s ===\n", sessionID))
	debugLog.WriteString(fmt.Sprintf("Issue ID: %d\n", input.Issue.ID))
	debugLog.WriteString(fmt.Sprintf("Gaps: %d, Findings: %d, Learnings: %d\n",
		len(input.Gaps), len(input.Findings), len(input.Learnings)))
	debugLog.WriteString(fmt.Sprintf("Context Summary: %s\n\n", logger.Truncate(input.ContextSummary, 500)))

	slog.InfoContext(ctx, "spec generator starting",
		"issue_id", input.Issue.ID,
		"gaps", len(input.Gaps),
		"findings", len(input.Findings))

	messages := s.buildMessages(input)

	iterations := 0
	totalPromptTokens := 0
	totalCompletionTokens := 0
	locateCallCount := 0

	defer func() {
		slog.InfoContext(ctx, "spec generator completed",
			"duration_ms", time.Since(start).Milliseconds(),
			"iterations", iterations,
			"total_prompt_tokens", totalPromptTokens,
			"total_completion_tokens", totalCompletionTokens,
			"locate_calls", locateCallCount)
		s.writeDebugLog(sessionID, debugLog.String())
	}()

	for {
		iterations++

		if iterations > maxSpecIterations {
			debugLog.WriteString(fmt.Sprintf("\n=== ITERATION LIMIT REACHED (%d) ===\n", iterations))
			s.writeDebugLog(sessionID, debugLog.String())
			return SpecGeneratorOutput{}, fmt.Errorf("spec generator exceeded max iterations (%d)", maxSpecIterations)
		}

		slog.DebugContext(ctx, "spec generator iteration", "iteration", iterations)

		resp, err := s.llm.ChatWithTools(ctx, llm.AgentRequest{
			Messages: messages,
			Tools:    s.tools(),
		})
		if err != nil {
			s.writeDebugLog(sessionID, debugLog.String())
			return SpecGeneratorOutput{}, fmt.Errorf("spec generator chat iteration %d: %w", iterations, err)
		}

		totalPromptTokens += resp.PromptTokens
		totalCompletionTokens += resp.CompletionTokens

		debugLog.WriteString(fmt.Sprintf("--- ITERATION %d ---\n", iterations))
		debugLog.WriteString(fmt.Sprintf("[ASSISTANT] (prompt=%d, completion=%d)\n%s\n\n",
			resp.PromptTokens, resp.CompletionTokens, logger.Truncate(resp.Content, 2000)))

		// Check for submit_spec - terminates the loop
		for _, tc := range resp.ToolCalls {
			if tc.Name == "submit_spec" {
				params, err := llm.ParseToolArguments[SubmitSpecParams](tc.Arguments)
				if err != nil {
					s.writeDebugLog(sessionID, debugLog.String())
					return SpecGeneratorOutput{}, fmt.Errorf("parsing submit_spec: %w", err)
				}

				debugLog.WriteString("=== SPEC GENERATOR COMPLETED (submit_spec) ===\n")
				debugLog.WriteString(fmt.Sprintf("Spec length: %d chars\n", len(params.Spec)))
				s.writeDebugLog(sessionID, debugLog.String())

				slog.InfoContext(ctx, "spec generator submitted spec",
					"iterations", iterations,
					"spec_length", len(params.Spec),
					"duration_ms", time.Since(start).Milliseconds())

				return SpecGeneratorOutput{
					Spec: params.Spec,
				}, nil
			}
		}

		// No tool calls = unexpected termination
		if len(resp.ToolCalls) == 0 {
			debugLog.WriteString("=== SPEC GENERATOR COMPLETED (no submit_spec) ===\n")
			debugLog.WriteString(fmt.Sprintf("Final content:\n%s\n", resp.Content))
			s.writeDebugLog(sessionID, debugLog.String())

			slog.WarnContext(ctx, "spec generator completed without submit_spec",
				"iterations", iterations)

			// Treat the content as the spec
			return SpecGeneratorOutput{
				Spec: resp.Content,
			}, nil
		}

		// Log tool calls
		for _, tc := range resp.ToolCalls {
			debugLog.WriteString(fmt.Sprintf("[TOOL CALL] %s\n", tc.Name))
			debugLog.WriteString(fmt.Sprintf("Arguments: %s\n\n", logger.Truncate(tc.Arguments, 500)))
		}

		// Add assistant message to conversation
		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Count locate calls for this batch
		batchLocateCalls := 0
		for _, tc := range resp.ToolCalls {
			if tc.Name == "locate" {
				batchLocateCalls++
			}
		}

		// Check hard limit BEFORE executing
		if locateCallCount+batchLocateCalls > maxLocateCalls {
			slog.WarnContext(ctx, "spec generator hit locate limit",
				"locate_calls", locateCallCount,
				"batch_calls", batchLocateCalls,
				"limit", maxLocateCalls)

			debugLog.WriteString(fmt.Sprintf("\n=== LOCATE LIMIT REACHED (%d/%d) ===\n",
				locateCallCount+batchLocateCalls, maxLocateCalls))

			messages = append(messages, llm.Message{
				Role: "user",
				Content: fmt.Sprintf(`⚠️ LOCATE LIMIT REACHED (%d/%d)

You must submit your spec now using submit_spec. No more location calls allowed.
Trust the findings from the planner and write your spec based on available context.`,
					locateCallCount+batchLocateCalls, maxLocateCalls),
			})
			continue
		}

		locateCallCount += batchLocateCalls

		// Execute locate calls in parallel
		results := s.executeExploresParallel(ctx, resp.ToolCalls)

		for _, r := range results {
			debugLog.WriteString(fmt.Sprintf("[TOOL RESULT] (length: %d)\n", len(r.report)))
			if len(r.report) > 1000 {
				debugLog.WriteString(r.report[:1000])
				debugLog.WriteString("\n... (truncated)\n\n")
			} else {
				debugLog.WriteString(r.report)
				debugLog.WriteString("\n\n")
			}

			messages = append(messages, llm.Message{
				Role:       "tool",
				Content:    r.report,
				ToolCallID: r.callID,
			})
		}
	}
}

// buildMessages constructs the initial message thread for spec generation.
func (s *SpecGenerator) buildMessages(input SpecGeneratorInput) []llm.Message {
	messages := []llm.Message{
		{Role: "system", Content: specGeneratorSystemPrompt},
	}

	// Build context message
	var ctx strings.Builder

	// Issue context
	ctx.WriteString("# Issue Context\n\n")
	if input.Issue.Title != nil {
		ctx.WriteString(fmt.Sprintf("**Title**: %s\n\n", *input.Issue.Title))
	}
	if input.Issue.Description != nil {
		ctx.WriteString(fmt.Sprintf("**Description**:\n%s\n\n", *input.Issue.Description))
	}

	// Context summary from planner
	if input.ContextSummary != "" {
		ctx.WriteString("## Context Summary (from planning)\n\n")
		ctx.WriteString(input.ContextSummary)
		ctx.WriteString("\n\n")
	}

	// Proceed signal
	if input.ProceedSignal != "" {
		ctx.WriteString("## Proceed Signal\n\n")
		ctx.WriteString(fmt.Sprintf("Human approval: %s\n\n", input.ProceedSignal))
	}

	// Closed gaps with answers
	if len(input.Gaps) > 0 {
		ctx.WriteString("# Resolved Questions (Gaps)\n\n")
		ctx.WriteString("These questions were asked and answered during planning:\n\n")
		for i, g := range input.Gaps {
			ctx.WriteString(fmt.Sprintf("## Q%d: %s\n", i+1, g.Question))
			if g.Respondent != "" {
				ctx.WriteString(fmt.Sprintf("**Asked to**: %s\n", g.Respondent))
			}
			if g.ClosedReason != "" {
				ctx.WriteString(fmt.Sprintf("**Resolution**: %s\n", g.ClosedReason))
			}
			if g.ClosedNote != "" {
				ctx.WriteString(fmt.Sprintf("**Answer**: %s\n", g.ClosedNote))
			}
			ctx.WriteString("\n")
		}
	}

	// Code findings
	if len(input.Findings) > 0 {
		ctx.WriteString("# Code Findings\n\n")
		ctx.WriteString("These code insights were discovered during exploration:\n\n")
		for _, f := range input.Findings {
			if len(f.Sources) > 0 {
				locations := make([]string, len(f.Sources))
				for i, src := range f.Sources {
					locations[i] = fmt.Sprintf("`%s`", src.Location)
				}
				ctx.WriteString(fmt.Sprintf("## %s\n\n", strings.Join(locations, ", ")))
			}
			ctx.WriteString(f.Synthesis)
			ctx.WriteString("\n\n")
		}
	}

	// Learnings
	if len(input.Learnings) > 0 {
		ctx.WriteString("# Workspace Learnings\n\n")
		ctx.WriteString("Tribal knowledge relevant to this implementation:\n\n")
		for i, l := range input.Learnings {
			ctx.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, l.Type, l.Content))
		}
		ctx.WriteString("\n")
	}

	messages = append(messages, llm.Message{
		Role:    "user",
		Content: ctx.String(),
	})

	return messages
}

type specExploreResult struct {
	callID string
	report string
}

// executeExploresParallel runs multiple locate calls concurrently.
// Spec generator only uses ModeLocate for quick file verification.
func (s *SpecGenerator) executeExploresParallel(ctx context.Context, toolCalls []llm.ToolCall) []specExploreResult {
	results := make([]specExploreResult, len(toolCalls))
	var wg sync.WaitGroup

	sem := make(chan struct{}, maxParallelSpecExplorers)

	for i, tc := range toolCalls {
		if tc.Name != "locate" {
			results[i] = specExploreResult{
				callID: tc.ID,
				report: fmt.Sprintf("Unknown tool: %s. Only 'locate' is available for spec generation.", tc.Name),
			}
			continue
		}

		wg.Add(1)
		go func(idx int, call llm.ToolCall) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			params, err := llm.ParseToolArguments[ExploreParams](call.Arguments)
			if err != nil {
				results[idx] = specExploreResult{
					callID: call.ID,
					report: fmt.Sprintf("Error parsing arguments: %s", err),
				}
				return
			}

			slog.DebugContext(ctx, "spec generator spawning locate agent",
				"query", logger.Truncate(params.Query, 100))

			// Use ModeLocate for fast file finding
			report, err := s.explore.ExploreWithMode(ctx, params.Query, ModeLocate)
			if err != nil {
				slog.WarnContext(ctx, "spec generator locate failed",
					"error", err,
					"query", logger.Truncate(params.Query, 100))
				report = fmt.Sprintf("Locate error: %s", err)
			}

			results[idx] = specExploreResult{
				callID: call.ID,
				report: report,
			}
		}(i, tc)
	}

	wg.Wait()
	return results
}

func (s *SpecGenerator) tools() []llm.Tool {
	return []llm.Tool{
		{
			Name: "locate",
			Description: `Verify file locations before including them in the spec.

⚠️ LIMITED to 8 calls. Most specs need 0-3.
The planner already explored and provided findings. Trust them first.

Only use locate if:
- A file path in findings seems incorrect
- You need to verify a location exists

You're verifying locations, not exploring from scratch.`,
			Parameters: llm.GenerateSchemaFrom(ExploreParams{}),
		},
		{
			Name:        "submit_spec",
			Description: "Submit the final implementation spec. Call this when you've completed the spec with all sections. Include the Confidence Assessment section at the end of your spec.",
			Parameters:  llm.GenerateSchemaFrom(SubmitSpecParams{}),
		},
	}
}

func (s *SpecGenerator) writeDebugLog(sessionID, content string) {
	if s.debugDir == "" {
		return
	}

	if err := os.MkdirAll(s.debugDir, 0o755); err != nil {
		slog.Warn("failed to create debug dir", "dir", s.debugDir, "error", err)
		return
	}

	filename := filepath.Join(s.debugDir, fmt.Sprintf("spec_generator_%s.txt", sessionID))
	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		slog.Warn("failed to write debug log", "file", filename, "error", err)
	} else {
		slog.Debug("debug log written", "file", filename)
	}
}

const specGeneratorSystemPrompt = `You are a senior architect generating an implementation spec.

You receive comprehensive context from the planner:
- A detailed handoff report with decisions, rationale, and code patterns discovered
- All resolved questions (gaps) with answers
- All code findings with locations and synthesis
- Workspace learnings

This context is your primary source of truth. The planner already explored the codebase — trust what it found.

## How You Work

1. **Read the handoff report carefully** — It contains what we're building, why, and key code patterns
2. **Review the findings** — They have file paths, code snippets, and synthesis from exploration
3. **Write the spec** — Synthesize everything into a clear, actionable implementation plan

You should rarely need to explore. Only use explore if the provided context has a critical gap that would make the spec incorrect. Most specs can be written with 0 explore calls.

# Your Audience

This spec will be read by:
1. **PMs/Stakeholders** — Need to understand what's being built and why
2. **Developers/AI Coding Agents** — Need exact file paths, signatures, and step-by-step guidance
3. **QA/Testers** — Need test scenarios, edge cases, and verification steps
4. **Code Reviewers** — Need to understand what changed and what to look for

# Output Format

Your spec MUST follow this structure:

` + "```" + `markdown
# Implementation Spec: [Issue Title]

## Summary
[2-3 sentences: What we're building and why. Written for PMs.]

## Scope & Decisions

### In Scope
- [Feature/change 1]
- [Feature/change 2]

### Out of Scope
- [Explicitly excluded item]

### Key Decisions
| Question | Decision | Rationale |
|----------|----------|-----------|
| [From resolved gaps] | [The answer] | [Why this decision] |

---

## Implementation Plan

### Phase 1: [Name]
**Files:** ` + "`" + `path/to/file.go` + "`" + `

**Changes:**
1. [What to change]
2. [What to add]

**Signatures & Logic:**
` + "```" + `go
func DoThing(ctx context.Context, input Input) (Output, error) {
    // 1. Validate input
    // 2. Fetch existing data
    // 3. Apply business logic
    // 4. Persist changes
    // 5. Return result
}
` + "```" + `

### Phase 2: [Name]
...

---

## Testing Guide

### Test Scenarios
| Scenario | Steps | Expected Result |
|----------|-------|-----------------|
| Happy path | 1. Do X, 2. Do Y | Z happens |
| Invalid input | 1. Submit empty | Validation error |

### Edge Cases to Verify
- [ ] [Edge case 1]
- [ ] [Edge case 2]

### Regression Checklist
- [ ] [Existing feature still works]

---

## Review Guide

### What Changed
- ` + "`" + `file.go` + "`" + ` - [Brief description]

### Why
[Context from the issue and decisions]

### What to Look For
- [ ] [Review concern 1]
- [ ] [Review concern 2]

### Risk Areas
- [Area of risk and why]

---

## Verification Steps
1. ` + "`" + `make test` + "`" + ` - all tests pass
2. ` + "`" + `make build` + "`" + ` - no errors
3. [Manual verification step]

---

## Confidence Assessment
**Overall:** [High/Medium/Low]

**Uncertainties:**
- [Any areas where you're not 100% sure]
` + "```" + `

# Guidelines

1. **Explore first, write second** — Do all exploration before you start writing the spec. Never explore mid-writing.
2. **Trust your context** — Once you start writing, trust the findings and your Phase 1 exploration. Don't second-guess.
3. **Be specific** — Include actual file paths, function names, and signatures from your gathered context.
4. **Pseudocode over prose** — For implementation phases, show the logic structure, not paragraphs.
5. **Trace decisions to gaps** — Each key decision should reference the resolved question that drove it.
6. **Include the "why"** — Don't just say what to do, explain why this approach was chosen.

# Tools

## locate(query) — RARELY NEEDED
⚠️ LIMITED to 8 calls. Most specs need 0-3.

The planner already explored and provided findings. Trust them first.

Only use locate if:
- A file path in findings seems incorrect
- You need to verify a specific location exists

You cannot analyze code deeply — only locate files.
If you find yourself wanting to explore frequently, re-read the handoff report and findings — the answer is likely already there.

## submit_spec(spec)
Submit your final spec. The spec MUST be valid markdown with proper newlines and formatting.
Include the Confidence Assessment section at the end of your spec markdown.

# Process

1. Read the handoff report — understand what we're building and why
2. Review findings — note file paths, patterns, and code locations
3. Review resolved gaps — understand decisions and constraints
4. Write the complete spec in one pass
5. Submit with submit_spec

Note uncertainties in Confidence Assessment rather than exploring to verify.`
