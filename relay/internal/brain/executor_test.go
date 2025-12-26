package brain_test

import (
	"context"
	"errors"

	"basegraph.app/relay/internal/brain"
	"basegraph.app/relay/internal/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// mockCodeGraphRetriever implements brain.CodeGraphRetriever for testing.
type mockCodeGraphRetriever struct {
	queryFn   func(ctx context.Context, issue *model.Issue, job brain.RetrieverJob) ([]model.CodeFinding, error)
	callCount int
}

func (m *mockCodeGraphRetriever) Query(ctx context.Context, issue *model.Issue, job brain.RetrieverJob) ([]model.CodeFinding, error) {
	m.callCount++
	if m.queryFn != nil {
		return m.queryFn(ctx, issue, job)
	}
	return []model.CodeFinding{}, nil
}

// mockLearningsRetriever implements brain.LearningsRetriever for testing.
type mockLearningsRetriever struct {
	queryFn   func(ctx context.Context, issue *model.Issue, job brain.RetrieverJob) ([]model.Learning, error)
	callCount int
}

func (m *mockLearningsRetriever) Query(ctx context.Context, issue *model.Issue, job brain.RetrieverJob) ([]model.Learning, error) {
	m.callCount++
	if m.queryFn != nil {
		return m.queryFn(ctx, issue, job)
	}
	return []model.Learning{}, nil
}

var _ = Describe("Executor", func() {
	var (
		executor      *brain.Executor
		mockCodeGraph *mockCodeGraphRetriever
		mockLearnings *mockLearningsRetriever
		ctx           context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockCodeGraph = &mockCodeGraphRetriever{}
		mockLearnings = &mockLearningsRetriever{}
		executor = brain.NewExecutor(mockCodeGraph, mockLearnings)
	})

	Describe("Execute", func() {
		Context("no jobs", func() {
			It("returns unchanged issue", func() {
				issue := &model.Issue{
					ID:    123,
					Title: stringPtr("Test issue"),
				}

				result, err := executor.Execute(ctx, issue, []brain.RetrieverJob{})

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(issue))
				Expect(mockCodeGraph.callCount).To(Equal(0))
				Expect(mockLearnings.callCount).To(Equal(0))
			})
		})

		Context("codegraph job", func() {
			It("executes and appends code findings", func() {
				mockCodeGraph.queryFn = func(ctx context.Context, issue *model.Issue, job brain.RetrieverJob) ([]model.CodeFinding, error) {
					return []model.CodeFinding{
						{
							Observation: "Found BillingService at internal/billing/service.go",
							Sources: []model.CodeSource{
								{Location: "internal/billing/service.go:15", Snippet: "type BillingService struct{}"},
							},
							Confidence: 0.85,
						},
					}, nil
				}

				issue := &model.Issue{ID: 123}
				jobs := []brain.RetrieverJob{
					{
						Type:        brain.RetrieverTypeCodeGraph,
						Query:       "BillingService",
						Intent:      "Find billing service",
						Priority:    1,
						SymbolHints: []string{"BillingService"},
					},
				}

				result, err := executor.Execute(ctx, issue, jobs)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.CodeFindings).To(HaveLen(1))
				Expect(result.CodeFindings[0].Observation).To(ContainSubstring("BillingService"))
				Expect(mockCodeGraph.callCount).To(Equal(1))
				Expect(mockLearnings.callCount).To(Equal(0))
			})
		})

		Context("learnings job", func() {
			It("executes and appends learnings", func() {
				mockLearnings.queryFn = func(ctx context.Context, issue *model.Issue, job brain.RetrieverJob) ([]model.Learning, error) {
					return []model.Learning{
						{
							ID:      1,
							Type:    "codebase_standards",
							Content: "Always use exponential backoff for retries",
						},
					}, nil
				}

				issue := &model.Issue{ID: 123}
				jobs := []brain.RetrieverJob{
					{
						Type:          brain.RetrieverTypeLearnings,
						Query:         "retry backoff",
						Intent:        "Find retry conventions",
						Priority:      1,
						LearningTypes: []string{"codebase_standards"},
					},
				}

				result, err := executor.Execute(ctx, issue, jobs)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Learnings).To(HaveLen(1))
				Expect(result.Learnings[0].Content).To(ContainSubstring("exponential backoff"))
				Expect(mockCodeGraph.callCount).To(Equal(0))
				Expect(mockLearnings.callCount).To(Equal(1))
			})
		})

		Context("multiple jobs", func() {
			It("executes jobs in priority order", func() {
				var executionOrder []string

				mockCodeGraph.queryFn = func(ctx context.Context, issue *model.Issue, job brain.RetrieverJob) ([]model.CodeFinding, error) {
					executionOrder = append(executionOrder, "codegraph:"+job.Query)
					return []model.CodeFinding{{Observation: job.Query, Sources: []model.CodeSource{}}}, nil
				}

				mockLearnings.queryFn = func(ctx context.Context, issue *model.Issue, job brain.RetrieverJob) ([]model.Learning, error) {
					executionOrder = append(executionOrder, "learnings:"+job.Query)
					return []model.Learning{{Content: job.Query}}, nil
				}

				issue := &model.Issue{ID: 123}
				jobs := []brain.RetrieverJob{
					{Type: brain.RetrieverTypeLearnings, Query: "low-priority", Priority: 3},
					{Type: brain.RetrieverTypeCodeGraph, Query: "high-priority", Priority: 1},
					{Type: brain.RetrieverTypeCodeGraph, Query: "medium-priority", Priority: 2},
				}

				result, err := executor.Execute(ctx, issue, jobs)

				Expect(err).NotTo(HaveOccurred())
				Expect(executionOrder).To(HaveLen(3))
				// Should be sorted by priority: 1, 2, 3
				Expect(executionOrder[0]).To(Equal("codegraph:high-priority"))
				Expect(executionOrder[1]).To(Equal("codegraph:medium-priority"))
				Expect(executionOrder[2]).To(Equal("learnings:low-priority"))
				Expect(result.CodeFindings).To(HaveLen(2))
				Expect(result.Learnings).To(HaveLen(1))
			})
		})

		Context("graceful degradation", func() {
			It("continues when codegraph fails", func() {
				mockCodeGraph.queryFn = func(ctx context.Context, issue *model.Issue, job brain.RetrieverJob) ([]model.CodeFinding, error) {
					return nil, errors.New("codegraph service unavailable")
				}

				mockLearnings.queryFn = func(ctx context.Context, issue *model.Issue, job brain.RetrieverJob) ([]model.Learning, error) {
					return []model.Learning{{Content: "learning found"}}, nil
				}

				issue := &model.Issue{ID: 123}
				jobs := []brain.RetrieverJob{
					{Type: brain.RetrieverTypeCodeGraph, Query: "fail", Priority: 1},
					{Type: brain.RetrieverTypeLearnings, Query: "succeed", Priority: 2},
				}

				result, err := executor.Execute(ctx, issue, jobs)

				// Should not fail - graceful degradation
				Expect(err).NotTo(HaveOccurred())
				Expect(result.CodeFindings).To(BeEmpty())
				Expect(result.Learnings).To(HaveLen(1))
				Expect(mockCodeGraph.callCount).To(Equal(1))
				Expect(mockLearnings.callCount).To(Equal(1))
			})

			It("continues when learnings fails", func() {
				mockCodeGraph.queryFn = func(ctx context.Context, issue *model.Issue, job brain.RetrieverJob) ([]model.CodeFinding, error) {
					return []model.CodeFinding{{Observation: "code found", Sources: []model.CodeSource{}}}, nil
				}

				mockLearnings.queryFn = func(ctx context.Context, issue *model.Issue, job brain.RetrieverJob) ([]model.Learning, error) {
					return nil, errors.New("learnings service unavailable")
				}

				issue := &model.Issue{ID: 123}
				jobs := []brain.RetrieverJob{
					{Type: brain.RetrieverTypeCodeGraph, Query: "succeed", Priority: 1},
					{Type: brain.RetrieverTypeLearnings, Query: "fail", Priority: 2},
				}

				result, err := executor.Execute(ctx, issue, jobs)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.CodeFindings).To(HaveLen(1))
				Expect(result.Learnings).To(BeEmpty())
			})
		})

		Context("nil retrievers", func() {
			It("handles nil codegraph retriever gracefully", func() {
				executor = brain.NewExecutor(nil, mockLearnings)
				mockLearnings.queryFn = func(ctx context.Context, issue *model.Issue, job brain.RetrieverJob) ([]model.Learning, error) {
					return []model.Learning{{Content: "learning"}}, nil
				}

				issue := &model.Issue{ID: 123}
				jobs := []brain.RetrieverJob{
					{Type: brain.RetrieverTypeCodeGraph, Query: "test", Priority: 1},
					{Type: brain.RetrieverTypeLearnings, Query: "test", Priority: 2},
				}

				result, err := executor.Execute(ctx, issue, jobs)

				// Should not fail - graceful degradation
				Expect(err).NotTo(HaveOccurred())
				Expect(result.CodeFindings).To(BeEmpty())
				Expect(result.Learnings).To(HaveLen(1))
			})
		})

		Context("aggregates with existing data", func() {
			It("appends to existing code findings", func() {
				mockCodeGraph.queryFn = func(ctx context.Context, issue *model.Issue, job brain.RetrieverJob) ([]model.CodeFinding, error) {
					return []model.CodeFinding{{Observation: "new finding", Sources: []model.CodeSource{}}}, nil
				}

				issue := &model.Issue{
					ID: 123,
					CodeFindings: []model.CodeFinding{
						{Observation: "existing finding", Sources: []model.CodeSource{}},
					},
				}
				jobs := []brain.RetrieverJob{
					{Type: brain.RetrieverTypeCodeGraph, Query: "search", Priority: 1},
				}

				result, err := executor.Execute(ctx, issue, jobs)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.CodeFindings).To(HaveLen(2))
				Expect(result.CodeFindings[0].Observation).To(Equal("existing finding"))
				Expect(result.CodeFindings[1].Observation).To(Equal("new finding"))
			})
		})
	})
})
