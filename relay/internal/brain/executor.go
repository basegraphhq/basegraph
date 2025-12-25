package brain

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"basegraph.app/relay/internal/model"
)

type CodeGraphRetriever interface {
	Query(ctx context.Context, issue *model.Issue, job RetrieverJob) ([]model.CodeFinding, error)
}

type LearningsRetriever interface {
	Query(ctx context.Context, issue *model.Issue, job RetrieverJob) ([]model.Learning, error)
}

type Executor struct {
	codegraph CodeGraphRetriever
	learnings LearningsRetriever
}

func NewExecutor(cg CodeGraphRetriever, lr LearningsRetriever) *Executor {
	return &Executor{
		codegraph: cg,
		learnings: lr,
	}
}

func (e *Executor) Execute(ctx context.Context, issue *model.Issue, jobs []RetrieverJob) (*model.Issue, error) {
	if len(jobs) == 0 {
		slog.DebugContext(ctx, "no retrieval jobs to execute", "issue_id", issue.ID)
		return issue, nil
	}

	sortedJobs := make([]RetrieverJob, len(jobs))
	copy(sortedJobs, jobs)
	sort.Slice(sortedJobs, func(i, j int) bool {
		return sortedJobs[i].Priority < sortedJobs[j].Priority
	})

	start := time.Now()
	var totalFindings int
	var totalLearnings int

	for _, job := range sortedJobs {
		switch job.Type {
		case RetrieverTypeCodeGraph:
			findings, err := e.executeCodeGraphJob(ctx, issue, job)
			if err != nil {
				// Graceful degradation: log and continue with other jobs
				slog.ErrorContext(ctx, "codegraph retrieval failed",
					"issue_id", issue.ID,
					"query", job.Query,
					"error", err)
				continue
			}
			issue.CodeFindings = append(issue.CodeFindings, findings...)
			totalFindings += len(findings)

		case RetrieverTypeLearnings:
			learnings, err := e.executeLearningsJob(ctx, issue, job)
			if err != nil {
				// Graceful degradation: log and continue with other jobs
				slog.ErrorContext(ctx, "learnings retrieval failed",
					"issue_id", issue.ID,
					"query", job.Query,
					"error", err)
				continue
			}
			issue.Learnings = append(issue.Learnings, learnings...)
			totalLearnings += len(learnings)

		default:
			slog.WarnContext(ctx, "unknown retriever type",
				"issue_id", issue.ID,
				"type", job.Type)
		}
	}

	slog.InfoContext(ctx, "executor completed",
		"issue_id", issue.ID,
		"jobs_executed", len(sortedJobs),
		"code_findings", totalFindings,
		"learnings", totalLearnings,
		"latency_ms", time.Since(start).Milliseconds())

	return issue, nil
}

func (e *Executor) executeCodeGraphJob(ctx context.Context, issue *model.Issue, job RetrieverJob) ([]model.CodeFinding, error) {
	if e.codegraph == nil {
		return nil, fmt.Errorf("codegraph retriever not configured")
	}

	start := time.Now()
	findings, err := e.codegraph.Query(ctx, issue, job)
	if err != nil {
		return nil, fmt.Errorf("codegraph query: %w", err)
	}

	slog.DebugContext(ctx, "codegraph job completed",
		"issue_id", issue.ID,
		"query", job.Query,
		"findings", len(findings),
		"latency_ms", time.Since(start).Milliseconds())

	return findings, nil
}

func (e *Executor) executeLearningsJob(ctx context.Context, issue *model.Issue, job RetrieverJob) ([]model.Learning, error) {
	if e.learnings == nil {
		return nil, fmt.Errorf("learnings retriever not configured")
	}

	start := time.Now()
	learnings, err := e.learnings.Query(ctx, issue, job)
	if err != nil {
		return nil, fmt.Errorf("learnings query: %w", err)
	}

	slog.DebugContext(ctx, "learnings job completed",
		"issue_id", issue.ID,
		"query", job.Query,
		"learnings", len(learnings),
		"latency_ms", time.Since(start).Milliseconds())

	return learnings, nil
}
