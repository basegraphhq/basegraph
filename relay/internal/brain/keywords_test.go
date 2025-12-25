package brain_test

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"basegraph.app/relay/common/id"
	"basegraph.app/relay/common/llm"
	"basegraph.app/relay/internal/brain"
	"basegraph.app/relay/internal/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = BeforeSuite(func() {
	// Initialize snowflake ID generator for tests
	err := id.Init(99)
	Expect(err).NotTo(HaveOccurred())
})

// mockLLMClient implements llm.Client for testing.
type mockLLMClient struct {
	chatFn    func(ctx context.Context, req llm.Request, result any) (*llm.Response, error)
	callCount int
}

func (m *mockLLMClient) Chat(ctx context.Context, req llm.Request, result any) (*llm.Response, error) {
	m.callCount++
	if m.chatFn != nil {
		return m.chatFn(ctx, req, result)
	}
	return nil, errors.New("mock not configured")
}

func (m *mockLLMClient) Model() string {
	return "test-model"
}

// mockLLMEvalStore implements store.LLMEvalStore for testing.
type mockLLMEvalStore struct {
	createFn  func(ctx context.Context, eval *model.LLMEval) (*model.LLMEval, error)
	callCount int
}

func (m *mockLLMEvalStore) Create(ctx context.Context, eval *model.LLMEval) (*model.LLMEval, error) {
	m.callCount++
	if m.createFn != nil {
		return m.createFn(ctx, eval)
	}
	return eval, nil
}

func (m *mockLLMEvalStore) GetByID(context.Context, int64) (*model.LLMEval, error) {
	return nil, nil
}

func (m *mockLLMEvalStore) ListByIssue(context.Context, int64) ([]model.LLMEval, error) {
	return nil, nil
}

func (m *mockLLMEvalStore) ListByStage(context.Context, string, int32) ([]model.LLMEval, error) {
	return nil, nil
}

func (m *mockLLMEvalStore) ListUnrated(context.Context, string, int32) ([]model.LLMEval, error) {
	return nil, nil
}

func (m *mockLLMEvalStore) Rate(context.Context, int64, int, string, int64) error {
	return nil
}

func (m *mockLLMEvalStore) SetExpected(context.Context, int64, []byte, float64) error {
	return nil
}

func (m *mockLLMEvalStore) GetStats(context.Context, string, time.Time) (*model.LLMEvalStats, error) {
	return nil, nil
}

func stringPtr(s string) *string {
	return &s
}

var _ = Describe("KeywordsExtractor", func() {
	var (
		extractor     *brain.KeywordsExtractor
		mockLLM       *mockLLMClient
		mockEvalStore *mockLLMEvalStore
		ctx           context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockLLM = &mockLLMClient{}
		mockEvalStore = &mockLLMEvalStore{}
		extractor = brain.NewKeywordsExtractor(mockLLM)
	})

	Describe("Extract", func() {
		Context("happy path", func() {
			It("extracts keywords from LLM response", func() {
				mockLLM.chatFn = func(ctx context.Context, req llm.Request, result any) (*llm.Response, error) {
					// Populate the result via JSON unmarshal simulation
					response := map[string]any{
						"keywords": []map[string]any{
							{"value": "authentication", "weight": 0.9, "category": "concept", "source": "title"},
							{"value": "login", "weight": 0.8, "category": "concept", "source": "description"},
						},
					}
					data, _ := json.Marshal(response)
					json.Unmarshal(data, result)
					return &llm.Response{PromptTokens: 100, CompletionTokens: 50}, nil
				}

				issue := &model.Issue{
					ID:          123,
					Title:       stringPtr("Fix authentication bug"),
					Description: stringPtr("Users cannot login"),
				}

				result, err := extractor.Extract(ctx, issue, mockEvalStore)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Keywords).To(HaveLen(2))
				Expect(result.Keywords[0].Value).To(Equal("authentication"))
				Expect(result.Keywords[0].Weight).To(Equal(0.9))
				Expect(result.Keywords[1].Value).To(Equal("login"))
				Expect(mockLLM.callCount).To(Equal(1))
				Expect(mockEvalStore.callCount).To(Equal(1))
			})
		})

		Context("no content", func() {
			It("returns unchanged issue without calling LLM", func() {
				issue := &model.Issue{
					ID: 123,
					// No title, description, or discussions
				}

				result, err := extractor.Extract(ctx, issue, mockEvalStore)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(issue))
				Expect(mockLLM.callCount).To(Equal(0))
				Expect(mockEvalStore.callCount).To(Equal(0))
			})
		})

		Context("retryable error", func() {
			It("retries on network error and succeeds", func() {
				attempts := 0
				mockLLM.chatFn = func(ctx context.Context, req llm.Request, result any) (*llm.Response, error) {
					attempts++
					if attempts < 2 {
						// First attempt fails with network error (retryable)
						return nil, errors.New("connection refused")
					}
					// Second attempt succeeds
					response := map[string]any{
						"keywords": []map[string]any{
							{"value": "retry_success", "weight": 0.85, "category": "concept", "source": "title"},
						},
					}
					data, _ := json.Marshal(response)
					json.Unmarshal(data, result)
					return &llm.Response{}, nil
				}

				issue := &model.Issue{
					ID:    123,
					Title: stringPtr("Test retry"),
				}

				result, err := extractor.Extract(ctx, issue, nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Keywords).To(HaveLen(1))
				Expect(result.Keywords[0].Value).To(Equal("retry_success"))
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

				_, err := extractor.Extract(ctx, issue, nil)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("keywords extraction"))
				Expect(mockLLM.callCount).To(Equal(1)) // No retries
			})
		})

		Context("eval store nil", func() {
			It("extracts keywords without crashing when eval store is nil", func() {
				mockLLM.chatFn = func(ctx context.Context, req llm.Request, result any) (*llm.Response, error) {
					response := map[string]any{
						"keywords": []map[string]any{
							{"value": "nil_store", "weight": 0.7, "category": "entity", "source": "title"},
						},
					}
					data, _ := json.Marshal(response)
					json.Unmarshal(data, result)
					return &llm.Response{}, nil
				}

				issue := &model.Issue{
					ID:    123,
					Title: stringPtr("Test nil store"),
				}

				result, err := extractor.Extract(ctx, issue, nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Keywords).To(HaveLen(1))
				Expect(result.Keywords[0].Value).To(Equal("nil_store"))
			})
		})
	})
})
