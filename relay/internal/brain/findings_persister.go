package brain

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"basegraph.app/relay/common/id"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/store"
)

// FindingsPersister allows ExploreAgent to persist findings without direct store dependency.
// This enables caching and deduplication of explore results.
type FindingsPersister interface {
	// AddFinding persists a finding with automatic deduplication.
	// If a similar query already exists, the finding is updated (newer wins).
	AddFinding(ctx context.Context, issueID int64, finding model.CodeFinding) error

	// FindSimilarQuery returns a cached finding if a similar query exists.
	// Returns nil if no similar finding is found.
	FindSimilarQuery(ctx context.Context, issueID int64, query string) (*model.CodeFinding, error)
}

// FindingsCacheDuration is the time after which a finding is considered stale.
// Stale findings may be re-explored even if query matches.
const FindingsCacheDuration = 1 * time.Hour

// QuerySimilarityThreshold is the minimum Jaccard similarity for query matching.
// 0.5 means 50% keyword overlap is required to consider queries similar.
const QuerySimilarityThreshold = 0.5

// stopWords are common words that shouldn't affect query similarity matching.
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
	"with": true, "by": true, "from": true, "as": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true, "being": true,
	"have": true, "has": true, "had": true, "do": true, "does": true, "did": true,
	"will": true, "would": true, "could": true, "should": true, "may": true, "might": true,
	"this": true, "that": true, "these": true, "those": true,
	"i": true, "you": true, "he": true, "she": true, "it": true, "we": true, "they": true,
	"what": true, "which": true, "who": true, "whom": true, "where": true, "when": true, "why": true, "how": true,
	"explore": true, "find": true, "search": true, "look": true, "check": true,
}

// wordSplitter splits on non-alphanumeric characters.
var wordSplitter = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// extractKeywords extracts meaningful keywords from a query string.
// Removes stop words and normalizes to lowercase.
func extractKeywords(query string) map[string]bool {
	words := wordSplitter.Split(strings.ToLower(query), -1)
	keywords := make(map[string]bool)

	for _, word := range words {
		word = strings.TrimSpace(word)
		if len(word) < 2 {
			continue
		}
		if stopWords[word] {
			continue
		}
		keywords[word] = true
	}

	return keywords
}

// jaccardSimilarity computes the Jaccard similarity between two keyword sets.
// Returns a value between 0 (no overlap) and 1 (identical).
func jaccardSimilarity(set1, set2 map[string]bool) float64 {
	if len(set1) == 0 && len(set2) == 0 {
		return 1.0 // Both empty = identical
	}
	if len(set1) == 0 || len(set2) == 0 {
		return 0.0 // One empty = no similarity
	}

	intersection := 0
	for word := range set1 {
		if set2[word] {
			intersection++
		}
	}

	// Union = |set1| + |set2| - |intersection|
	union := len(set1) + len(set2) - intersection

	return float64(intersection) / float64(union)
}

// QuerySimilarity computes similarity between two query strings.
// Returns a value between 0 and 1.
func QuerySimilarity(query1, query2 string) float64 {
	keywords1 := extractKeywords(query1)
	keywords2 := extractKeywords(query2)
	return jaccardSimilarity(keywords1, keywords2)
}

// FindSimilarFinding searches through findings for one with a similar query.
// Returns the best match if similarity exceeds threshold, nil otherwise.
func FindSimilarFinding(findings []model.CodeFinding, query string) *model.CodeFinding {
	queryKeywords := extractKeywords(query)
	if len(queryKeywords) == 0 {
		return nil
	}

	var bestMatch *model.CodeFinding
	bestSimilarity := 0.0

	for i := range findings {
		f := &findings[i]
		if f.Query == "" {
			continue // Skip legacy findings without query
		}

		storedKeywords := extractKeywords(f.Query)
		similarity := jaccardSimilarity(queryKeywords, storedKeywords)

		if similarity > bestSimilarity && similarity >= QuerySimilarityThreshold {
			bestSimilarity = similarity
			bestMatch = f
		}
	}

	return bestMatch
}

// MaxCodeFindings is the maximum number of findings to keep per issue.
// Oldest findings are evicted when this limit is exceeded.
const MaxCodeFindings = 20

// findingsPersister implements FindingsPersister using IssueStore.
type findingsPersister struct {
	issues store.IssueStore
}

// NewFindingsPersister creates a FindingsPersister backed by an IssueStore.
func NewFindingsPersister(issues store.IssueStore) FindingsPersister {
	return &findingsPersister{issues: issues}
}

// AddFinding persists a finding with automatic deduplication.
func (p *findingsPersister) AddFinding(ctx context.Context, issueID int64, finding model.CodeFinding) error {
	issue, err := p.issues.GetByID(ctx, issueID)
	if err != nil {
		return fmt.Errorf("getting issue: %w", err)
	}

	// Ensure finding has an ID
	if finding.ID == "" {
		finding.ID = fmt.Sprintf("%d", id.New())
	}

	// Ensure finding has CreatedAt
	if finding.CreatedAt.IsZero() {
		finding.CreatedAt = time.Now()
	}

	// Check for similar existing finding (deduplication)
	for i, existing := range issue.CodeFindings {
		if existing.Query != "" && QuerySimilarity(existing.Query, finding.Query) >= QuerySimilarityThreshold {
			// Replace existing finding (newer wins)
			slog.InfoContext(ctx, "replacing similar finding",
				"issue_id", issueID,
				"old_query", existing.Query,
				"new_query", finding.Query,
				"similarity", QuerySimilarity(existing.Query, finding.Query))
			issue.CodeFindings[i] = finding
			_, err = p.issues.Upsert(ctx, issue)
			return err
		}
	}

	// Add new finding
	issue.CodeFindings = append(issue.CodeFindings, finding)

	// Evict oldest if over limit
	if len(issue.CodeFindings) > MaxCodeFindings {
		evicted := issue.CodeFindings[0]
		slog.InfoContext(ctx, "evicting oldest finding",
			"issue_id", issueID,
			"evicted_query", evicted.Query,
			"evicted_age", time.Since(evicted.CreatedAt))
		issue.CodeFindings = issue.CodeFindings[1:]
	}

	_, err = p.issues.Upsert(ctx, issue)
	return err
}

// FindSimilarQuery returns a cached finding if a similar query exists.
func (p *findingsPersister) FindSimilarQuery(ctx context.Context, issueID int64, query string) (*model.CodeFinding, error) {
	issue, err := p.issues.GetByID(ctx, issueID)
	if err != nil {
		return nil, fmt.Errorf("getting issue: %w", err)
	}

	return FindSimilarFinding(issue.CodeFindings, query), nil
}
