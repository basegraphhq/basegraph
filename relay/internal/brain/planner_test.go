package brain_test

import (
	"context"
	"encoding/json"
	"errors"

	"basegraph.app/relay/common/llm"
	"basegraph.app/relay/internal/brain"
	"basegraph.app/relay/internal/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Planner", func() {
	var (
		planner       *brain.Planner
		mockLLM       *mockLLMClient
		mockEvalStore *mockLLMEvalStore
		ctx           context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockLLM = &mockLLMClient{}
		mockEvalStore = &mockLLMEvalStore{}
		planner = brain.NewPlanner(mockLLM)
	})

	Describe("Plan", func() {
		Context("empty issue", func() {
			It("returns context sufficient without calling LLM", func() {
				issue := &model.Issue{
					ID: 123,
					// No title, description, keywords, or discussions
				}

				result, err := planner.Plan(ctx, issue, mockEvalStore)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.ContextSufficient).To(BeTrue())
				Expect(result.Reasoning).To(ContainSubstring("Empty issue"))
				Expect(result.Jobs).To(BeEmpty())
				Expect(mockLLM.callCount).To(Equal(0))
				Expect(mockEvalStore.callCount).To(Equal(0))
			})
		})

		Context("simple bug fix (context sufficient)", func() {
			It("returns no retrieval jobs", func() {
				mockLLM.chatFn = func(ctx context.Context, req llm.Request, result any) (*llm.Response, error) {
					response := map[string]any{
						"context_sufficient": true,
						"reasoning":          "Clear typo fix. Keywords point to auth_handler.",
						"jobs":               []any{},
					}
					data, _ := json.Marshal(response)
					_ = json.Unmarshal(data, result)
					return &llm.Response{PromptTokens: 150, CompletionTokens: 30}, nil
				}

				issue := &model.Issue{
					ID:          123,
					Title:       stringPtr("Fix typo in login error message"),
					Description: stringPtr("Says 'pasword' instead of 'password'"),
					Keywords: []model.Keyword{
						{Value: "login", Weight: 0.9, Category: "concept"},
						{Value: "auth_handler", Weight: 0.8, Category: "entity"},
					},
				}

				result, err := planner.Plan(ctx, issue, mockEvalStore)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.ContextSufficient).To(BeTrue())
				Expect(result.Jobs).To(BeEmpty())
				Expect(mockLLM.callCount).To(Equal(1))
				Expect(mockEvalStore.callCount).To(Equal(1))
			})
		})

		Context("feature addition (context insufficient)", func() {
			It("returns retrieval jobs", func() {
				mockLLM.chatFn = func(ctx context.Context, req llm.Request, result any) (*llm.Response, error) {
					response := map[string]any{
						"context_sufficient": false,
						"reasoning":          "Need to understand existing payment architecture",
						"jobs": []map[string]any{
							{
								"type":         "codegraph",
								"query":        "BillingService payment processing",
								"intent":       "Understand existing payment architecture",
								"priority":     1,
								"symbol_hints": []string{"BillingService", "PaymentProcessor"},
							},
							{
								"type":           "learnings",
								"query":          "payment security PCI compliance",
								"intent":         "Find team security requirements",
								"priority":       2,
								"learning_types": []string{"codebase_standards"},
							},
						},
					}
					data, _ := json.Marshal(response)
					_ = json.Unmarshal(data, result)
					return &llm.Response{PromptTokens: 200, CompletionTokens: 100}, nil
				}

				issue := &model.Issue{
					ID:          456,
					Title:       stringPtr("Add Stripe payment integration"),
					Description: stringPtr("We need to accept credit card payments via Stripe"),
					Keywords: []model.Keyword{
						{Value: "stripe", Weight: 0.95, Category: "library"},
						{Value: "payment", Weight: 0.9, Category: "concept"},
						{Value: "billing_service", Weight: 0.8, Category: "entity"},
					},
				}

				result, err := planner.Plan(ctx, issue, mockEvalStore)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.ContextSufficient).To(BeFalse())
				Expect(result.Jobs).To(HaveLen(2))

				// Check first job (codegraph)
				Expect(result.Jobs[0].Type).To(Equal(brain.RetrieverTypeCodeGraph))
				Expect(result.Jobs[0].Query).To(ContainSubstring("BillingService"))
				Expect(result.Jobs[0].Priority).To(Equal(1))
				Expect(result.Jobs[0].SymbolHints).To(ContainElement("BillingService"))

				// Check second job (learnings)
				Expect(result.Jobs[1].Type).To(Equal(brain.RetrieverTypeLearnings))
				Expect(result.Jobs[1].Priority).To(Equal(2))
				Expect(result.Jobs[1].LearningTypes).To(ContainElement("codebase_standards"))
			})
		})

		Context("issue with existing code findings", func() {
			It("includes code findings in prompt context", func() {
				var capturedPrompt string
				mockLLM.chatFn = func(ctx context.Context, req llm.Request, result any) (*llm.Response, error) {
					capturedPrompt = req.UserPrompt
					response := map[string]any{
						"context_sufficient": true,
						"reasoning":          "Have enough context from previous retrieval",
						"jobs":               []any{},
					}
					data, _ := json.Marshal(response)
					_ = json.Unmarshal(data, result)
					return &llm.Response{}, nil
				}

				issue := &model.Issue{
					ID:          789,
					Title:       stringPtr("Update rate limiting"),
					Description: stringPtr("Adjust rate limits"),
					CodeFindings: []model.CodeFinding{
						{
							Observation: "RateLimiter uses token bucket algorithm",
							Sources: []model.CodeSource{
								{Location: "internal/middleware/rate_limiter.go:42", Snippet: "type RateLimiter struct{}"},
							},
							Confidence: 0.9,
						},
					},
				}

				_, err := planner.Plan(ctx, issue, nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(capturedPrompt).To(ContainSubstring("Code Context (Already Retrieved)"))
				Expect(capturedPrompt).To(ContainSubstring("RateLimiter uses token bucket"))
				Expect(capturedPrompt).To(ContainSubstring("rate_limiter.go"))
			})
		})

		Context("retryable error", func() {
			It("retries on network error and succeeds", func() {
				attempts := 0
				mockLLM.chatFn = func(ctx context.Context, req llm.Request, result any) (*llm.Response, error) {
					attempts++
					if attempts < 2 {
						return nil, errors.New("connection refused")
					}
					response := map[string]any{
						"context_sufficient": true,
						"reasoning":          "Retry succeeded",
						"jobs":               []any{},
					}
					data, _ := json.Marshal(response)
					_ = json.Unmarshal(data, result)
					return &llm.Response{}, nil
				}

				issue := &model.Issue{
					ID:    123,
					Title: stringPtr("Test retry"),
				}

				result, err := planner.Plan(ctx, issue, nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.ContextSufficient).To(BeTrue())
				Expect(mockLLM.callCount).To(Equal(2))
			})
		})

		Context("non-retryable error", func() {
			It("fails immediately on context cancellation", func() {
				mockLLM.chatFn = func(ctx context.Context, req llm.Request, result any) (*llm.Response, error) {
					return nil, context.Canceled
				}

				issue := &model.Issue{
					ID:    123,
					Title: stringPtr("Test non-retryable"),
				}

				_, err := planner.Plan(ctx, issue, nil)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("planner"))
				Expect(mockLLM.callCount).To(Equal(1))
			})
		})

		Context("nil eval store", func() {
			It("plans without crashing when eval store is nil", func() {
				mockLLM.chatFn = func(ctx context.Context, req llm.Request, result any) (*llm.Response, error) {
					response := map[string]any{
						"context_sufficient": true,
						"reasoning":          "Simple task",
						"jobs":               []any{},
					}
					data, _ := json.Marshal(response)
					_ = json.Unmarshal(data, result)
					return &llm.Response{}, nil
				}

				issue := &model.Issue{
					ID:    123,
					Title: stringPtr("Test nil store"),
				}

				result, err := planner.Plan(ctx, issue, nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.ContextSufficient).To(BeTrue())
			})
		})
	})
})
