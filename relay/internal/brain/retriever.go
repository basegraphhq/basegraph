package brain

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/store"
)

//go:embed testdata/codegraph_stubs.golden.json
var codegraphStubsJSON []byte

type stubGoldenFile struct {
	SymbolFindings struct {
		Examples []struct {
			Symbol  string            `json:"symbol"`
			Finding model.CodeFinding `json:"finding"`
		} `json:"examples"`
	} `json:"symbol_findings"`
	QueryFindings struct {
		Examples []struct {
			Query    string              `json:"query"`
			Findings []model.CodeFinding `json:"findings"`
		} `json:"examples"`
	} `json:"query_findings"`
	Fallback struct {
		Findings []model.CodeFinding `json:"findings"`
	} `json:"fallback"`
}

// TODO: Replace with real implementation that queries ArangoDB + Typesense
type StubCodeGraphRetriever struct {
	stubs stubGoldenFile
}

func NewStubCodeGraphRetriever() *StubCodeGraphRetriever {
	var stubs stubGoldenFile
	if err := json.Unmarshal(codegraphStubsJSON, &stubs); err != nil {
		slog.Error("failed to parse codegraph stubs golden file", "error", err)
	}
	return &StubCodeGraphRetriever{stubs: stubs}
}

func (r *StubCodeGraphRetriever) Query(ctx context.Context, issue *model.Issue, job RetrieverJob) ([]model.CodeFinding, error) {
	slog.DebugContext(ctx, "stub codegraph query",
		"issue_id", issue.ID,
		"query", job.Query,
		"intent", job.Intent,
		"symbol_hints", job.SymbolHints)

	findings := r.generateStubFindings(job)

	slog.InfoContext(ctx, "stub codegraph returned findings",
		"issue_id", issue.ID,
		"query", job.Query,
		"finding_count", len(findings))

	return findings, nil
}

func (r *StubCodeGraphRetriever) generateStubFindings(job RetrieverJob) []model.CodeFinding {
	if len(job.SymbolHints) > 0 {
		return r.findingsFromSymbolHints(job)
	}
	return r.findingsFromQuery(job)
}

func (r *StubCodeGraphRetriever) findingsFromSymbolHints(job RetrieverJob) []model.CodeFinding {
	var findings []model.CodeFinding

	for _, symbol := range job.SymbolHints {
		// Check if we have a curated example for this symbol
		if finding, ok := r.lookupSymbol(symbol); ok {
			findings = append(findings, finding)
			continue
		}
		// Generate a synthetic finding for unknown symbols
		findings = append(findings, model.CodeFinding{
			Observation: fmt.Sprintf("Found %s defined as function with 3 callers. Accepts context and returns error. Used by service layer handlers.", symbol),
			Sources: []model.CodeSource{
				{
					Location: fmt.Sprintf("internal/service/%s.go:42", toSnakeCase(symbol)),
					Snippet:  fmt.Sprintf("func %s(ctx context.Context, req Request) (*Response, error) {\n\t// Implementation\n\treturn nil, nil\n}", symbol),
				},
			},
			Confidence: 0.92,
		})
	}

	return findings
}

func (r *StubCodeGraphRetriever) lookupSymbol(symbol string) (model.CodeFinding, bool) {
	symbolLower := strings.ToLower(symbol)
	for _, ex := range r.stubs.SymbolFindings.Examples {
		if strings.ToLower(ex.Symbol) == symbolLower {
			return ex.Finding, true
		}
	}
	return model.CodeFinding{}, false
}

func (r *StubCodeGraphRetriever) findingsFromQuery(job RetrieverJob) []model.CodeFinding {
	queryLower := strings.ToLower(job.Query)

	// Check for matching curated query examples
	for _, ex := range r.stubs.QueryFindings.Examples {
		if strings.Contains(queryLower, strings.ToLower(ex.Query)) {
			return ex.Findings
		}
	}

	// Return fallback findings
	return r.stubs.Fallback.Findings
}

func toSnakeCase(s string) string {
	var result []byte
	for i, c := range s {
		if c >= 'A' && c <= 'Z' {
			if i > 0 {
				result = append(result, '_')
			}
			result = append(result, byte(c+'a'-'A'))
		} else {
			result = append(result, byte(c))
		}
	}
	return string(result)
}

// DBLearningsRetriever fetches all learnings for the issue's workspace
type DBLearningsRetriever struct {
	learnings    store.LearningStore
	integrations store.IntegrationStore
}

func NewDBLearningsRetriever(learnings store.LearningStore, integrations store.IntegrationStore) *DBLearningsRetriever {
	return &DBLearningsRetriever{
		learnings:    learnings,
		integrations: integrations,
	}
}

func (r *DBLearningsRetriever) Query(ctx context.Context, issue *model.Issue, job RetrieverJob) ([]model.Learning, error) {
	integration, err := r.integrations.GetByID(ctx, issue.IntegrationID)
	if err != nil {
		return nil, fmt.Errorf("get integration: %w", err)
	}

	learnings, err := r.learnings.ListByWorkspace(ctx, integration.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("list learnings: %w", err)
	}

	slog.DebugContext(ctx, "fetched learnings",
		"issue_id", issue.ID,
		"workspace_id", integration.WorkspaceID,
		"count", len(learnings))

	return learnings, nil
}

// StubLearningsRetriever returns empty results for testing
type StubLearningsRetriever struct{}

func NewStubLearningsRetriever() *StubLearningsRetriever {
	return &StubLearningsRetriever{}
}

func (r *StubLearningsRetriever) Query(ctx context.Context, issue *model.Issue, job RetrieverJob) ([]model.Learning, error) {
	slog.DebugContext(ctx, "stub learnings query",
		"issue_id", issue.ID,
		"query", job.Query,
		"intent", job.Intent)

	return []model.Learning{}, nil
}
