package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"basegraph.app/relay/internal/domain"
)

// Result captures the output of the planner for a single event.
type Result struct {
	Event             domain.Event
	Issue             *domain.Issue
	Jobs              []domain.Job
	ContextSufficient bool
	Notes             []string
}

// Planner decides which retriever jobs should run based on the incoming event.
type Planner interface {
	Plan(ctx context.Context, event domain.Event, issue *domain.Issue) (*Result, error)
}

type planner struct{}

// New returns a planner that produces a deterministic plan for the current MVP scenario.
func New() Planner {
	return &planner{}
}

func (p *planner) Plan(ctx context.Context, event domain.Event, issue *domain.Issue) (*Result, error) { //nolint: revive // context unused now but kept for future logic
	if issue == nil {
		return nil, fmt.Errorf("issue context is required")
	}

	jobs := make([]domain.Job, 0, 3)
	notes := make([]string, 0)

	switch event.Type {
	case domain.EventTypeIssueCreated:
		text := extractIssueText(event.Payload)
		jobs = append(jobs, keywordJob(text))
		jobs = append(jobs, codeJob())
		jobs = append(jobs, learningsJob())
		notes = append(notes, "initial issue created; running full retrieval cycle")
	case domain.EventTypeReply:
		replyText, metaNote := extractReplyText(event.Payload)
		if replyText != "" {
			jobs = append(jobs, keywordJob(replyText))
			notes = append(notes, "reply detected; extracting new keywords")
		}
		jobs = append(jobs, codeJob())
		jobs = append(jobs, learningsJob())
		if metaNote != "" {
			notes = append(notes, metaNote)
		}
	default:
		// Unknown events default to minimal plan: still attempt keyword extraction to keep state fresh.
		jobs = append(jobs, domain.Job{Type: domain.JobExtractKeywords, Reason: "unknown event type"})
		notes = append(notes, fmt.Sprintf("unrecognized event_type=%s; running keywords-only plan", event.Type))
	}

	return &Result{
		Event:             event,
		Issue:             issue,
		Jobs:              jobs,
		ContextSufficient: len(jobs) == 0,
		Notes:             notes,
	}, nil
}

func keywordJob(text string) domain.Job {
	payload := map[string]any{}
	if strings.TrimSpace(text) != "" {
		payload["text"] = text
	}
	return domain.Job{
		Type:    domain.JobExtractKeywords,
		Payload: payload,
		Reason:  "identify salient entities for subsequent retrieval",
	}
}

func codeJob() domain.Job {
	return domain.Job{
		Type:   domain.JobRetrieveCode,
		Reason: "expand context using code graph based on latest keywords",
	}
}

func learningsJob() domain.Job {
	return domain.Job{
		Type:   domain.JobRetrieveLearnings,
		Reason: "refresh domain/project learnings relevant to the issue",
	}
}

func extractIssueText(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var body struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Labels      []string `json:"labels"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return string(payload)
	}

	parts := []string{body.Title, body.Description}
	if len(body.Labels) > 0 {
		parts = append(parts, strings.Join(body.Labels, " "))
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func extractReplyText(payload json.RawMessage) (text string, note string) {
	if len(payload) == 0 {
		return "", ""
	}
	var body struct {
		Author string `json:"author"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return string(payload), "reply payload could not be decoded; passing raw body to keyword extractor"
	}

	meta := ""
	if body.Author != "" {
		meta = fmt.Sprintf("reply authored by %s", body.Author)
	}

	return strings.TrimSpace(body.Body), meta
}
