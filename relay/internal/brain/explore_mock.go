package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"basegraph.app/relay/common/llm"
	"basegraph.app/relay/common/logger"
)

// ExploreFixture represents a pre-written explore response for mock mode.
type ExploreFixture struct {
	ID       string   `json:"id"`
	Intent   string   `json:"intent"`   // What question this answers (helps LLM match)
	Keywords []string `json:"keywords"` // Semantic tags for matching
	Scope    []string `json:"scope"`    // Files/areas covered
	Report   string   `json:"report"`   // Full explore output in expected format
}

// FixtureFile is the JSON structure for explore fixtures.
type FixtureFile struct {
	Version      string           `json:"version"`
	IssueContext string           `json:"issue_context"` // Description of the test issue
	Fixtures     []ExploreFixture `json:"fixtures"`
}

// FixtureSelection is the LLM's response when selecting a fixture.
type FixtureSelection struct {
	FixtureID  *string `json:"fixture_id"`
	Confidence string  `json:"confidence"` // high, medium, low, none
	Reason     string  `json:"reason"`
}

// exploreWithMock uses fixture selection instead of real exploration.
func (e *ExploreAgent) exploreWithMock(ctx context.Context, query string) (string, error) {
	slog.InfoContext(ctx, "explore agent running in mock mode",
		"query", logger.Truncate(query, 100))

	// Load fixtures from file
	fixtures, err := e.loadFixtures()
	if err != nil {
		return "", fmt.Errorf("loading fixtures: %w", err)
	}

	if len(fixtures) == 0 {
		return "", fmt.Errorf("mock explore: no fixtures found in %s", e.fixtureFile)
	}

	// Use LLM to select best matching fixture
	selected, err := e.selectFixture(ctx, query, fixtures)
	if err != nil {
		return "", fmt.Errorf("selecting fixture: %w", err)
	}

	if selected == nil {
		slog.WarnContext(ctx, "no fixture matched explore query",
			"query", query,
			"fixture_count", len(fixtures))
		return "", fmt.Errorf("mock explore: no fixture matches query %q - add fixture to %s",
			logger.Truncate(query, 100), e.fixtureFile)
	}

	slog.InfoContext(ctx, "mock explore selected fixture",
		"fixture_id", selected.ID,
		"query", logger.Truncate(query, 100))

	return selected.Report, nil
}

// loadFixtures reads and parses the fixture JSON file.
func (e *ExploreAgent) loadFixtures() ([]ExploreFixture, error) {
	data, err := os.ReadFile(e.fixtureFile)
	if err != nil {
		return nil, fmt.Errorf("reading fixture file %s: %w", e.fixtureFile, err)
	}

	var file FixtureFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parsing fixture file %s: %w", e.fixtureFile, err)
	}

	return file.Fixtures, nil
}

// selectFixture uses the mock LLM to select the best matching fixture.
func (e *ExploreAgent) selectFixture(ctx context.Context, query string, fixtures []ExploreFixture) (*ExploreFixture, error) {
	// Build the fixture list for the prompt
	var fixtureList strings.Builder
	for i, f := range fixtures {
		fixtureList.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, f.ID))
		fixtureList.WriteString(fmt.Sprintf("   Intent: %s\n", f.Intent))
		fixtureList.WriteString(fmt.Sprintf("   Scope: %s\n", strings.Join(f.Scope, ", ")))
		fixtureList.WriteString(fmt.Sprintf("   Keywords: %s\n\n", strings.Join(f.Keywords, ", ")))
	}

	prompt := fmt.Sprintf(`You are selecting a pre-written code exploration result that best matches a query.

## Available Fixtures

%s
## Query

%s

## Instructions

Select the fixture that best answers this query. Consider:
- Does the intent match what the query is asking?
- Does the scope cover files the query mentions?
- Do keywords overlap with query terms?

Respond with ONLY this JSON (no markdown, no explanation):
{"fixture_id": "the-id-here", "confidence": "high|medium|low", "reason": "one sentence why"}

If no fixture is a good match:
{"fixture_id": null, "confidence": "none", "reason": "why no match"}`, fixtureList.String(), query)

	resp, err := e.mockLLM.ChatWithTools(ctx, llm.AgentRequest{
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		MaxTokens: 200, // Keep response small
	})
	if err != nil {
		return nil, fmt.Errorf("LLM fixture selection: %w", err)
	}

	// Parse the selection response
	// Clean up response in case LLM wrapped it in markdown
	content := strings.TrimSpace(resp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var selection FixtureSelection
	if err := json.Unmarshal([]byte(content), &selection); err != nil {
		slog.WarnContext(ctx, "failed to parse fixture selection response",
			"response", resp.Content,
			"error", err)
		return nil, fmt.Errorf("parsing selection response: %w (response: %s)", err, logger.Truncate(resp.Content, 200))
	}

	// Log the selection decision
	slog.DebugContext(ctx, "fixture selection decision",
		"fixture_id", selection.FixtureID,
		"confidence", selection.Confidence,
		"reason", selection.Reason)

	// No match case
	if selection.FixtureID == nil || selection.Confidence == "none" {
		return nil, nil
	}

	// Find the selected fixture
	for i := range fixtures {
		if fixtures[i].ID == *selection.FixtureID {
			return &fixtures[i], nil
		}
	}

	// Selected ID not found (shouldn't happen if LLM follows instructions)
	slog.WarnContext(ctx, "selected fixture ID not found",
		"fixture_id", *selection.FixtureID,
		"available_ids", fixtureIDs(fixtures))
	return nil, fmt.Errorf("selected fixture ID not found: %s", *selection.FixtureID)
}

// fixtureIDs returns a slice of fixture IDs for logging.
func fixtureIDs(fixtures []ExploreFixture) []string {
	ids := make([]string, len(fixtures))
	for i, f := range fixtures {
		ids[i] = f.ID
	}
	return ids
}
