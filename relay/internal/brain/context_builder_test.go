package brain_test

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"basegraph.app/relay/common/llm"
	"basegraph.app/relay/internal/brain"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/store"
)

// Mock stores for testing

type mockIntegrationStore struct {
	getByIDFn func(ctx context.Context, id int64) (*model.Integration, error)
}

func (m *mockIntegrationStore) GetByID(ctx context.Context, id int64) (*model.Integration, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, store.ErrNotFound
}

func (m *mockIntegrationStore) GetByWorkspaceAndProvider(ctx context.Context, workspaceID int64, provider model.Provider) (*model.Integration, error) {
	return nil, nil
}

func (m *mockIntegrationStore) Create(ctx context.Context, integration *model.Integration) error {
	return nil
}

func (m *mockIntegrationStore) Update(ctx context.Context, integration *model.Integration) error {
	return nil
}

func (m *mockIntegrationStore) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	return nil
}

func (m *mockIntegrationStore) Delete(ctx context.Context, id int64) error { return nil }

func (m *mockIntegrationStore) ListByWorkspace(ctx context.Context, workspaceID int64) ([]model.Integration, error) {
	return nil, nil
}

func (m *mockIntegrationStore) ListByOrganization(ctx context.Context, orgID int64) ([]model.Integration, error) {
	return nil, nil
}

func (m *mockIntegrationStore) ListByCapability(ctx context.Context, workspaceID int64, capability model.Capability) ([]model.Integration, error) {
	return nil, nil
}

type mockIntegrationConfigStore struct {
	getByIntegrationAndKeyFn func(ctx context.Context, integrationID int64, key string) (*model.IntegrationConfig, error)
}

func (m *mockIntegrationConfigStore) GetByID(ctx context.Context, id int64) (*model.IntegrationConfig, error) {
	return nil, nil
}

func (m *mockIntegrationConfigStore) GetByIntegrationAndKey(ctx context.Context, integrationID int64, key string) (*model.IntegrationConfig, error) {
	if m.getByIntegrationAndKeyFn != nil {
		return m.getByIntegrationAndKeyFn(ctx, integrationID, key)
	}
	return nil, store.ErrNotFound
}

func (m *mockIntegrationConfigStore) ListByIntegration(ctx context.Context, integrationID int64) ([]model.IntegrationConfig, error) {
	return nil, nil
}

func (m *mockIntegrationConfigStore) ListByIntegrationAndType(ctx context.Context, integrationID int64, configType string) ([]model.IntegrationConfig, error) {
	return nil, nil
}

func (m *mockIntegrationConfigStore) Create(ctx context.Context, config *model.IntegrationConfig) error {
	return nil
}

func (m *mockIntegrationConfigStore) Update(ctx context.Context, config *model.IntegrationConfig) error {
	return nil
}

func (m *mockIntegrationConfigStore) Upsert(ctx context.Context, config *model.IntegrationConfig) error {
	return nil
}

func (m *mockIntegrationConfigStore) Delete(ctx context.Context, id int64) error { return nil }

func (m *mockIntegrationConfigStore) DeleteByIntegration(ctx context.Context, integrationID int64) error {
	return nil
}

type mockLearningStore struct {
	listByWorkspaceFn func(ctx context.Context, workspaceID int64) ([]model.Learning, error)
}

func (m *mockLearningStore) GetByID(ctx context.Context, id int64) (*model.Learning, error) {
	return nil, nil
}

func (m *mockLearningStore) GetByWorkspaceAndType(ctx context.Context, workspaceID int64, learningType string) (*model.Learning, error) {
	return nil, nil
}

func (m *mockLearningStore) Create(ctx context.Context, learning *model.Learning) error {
	return nil
}

func (m *mockLearningStore) Update(ctx context.Context, learning *model.Learning) error {
	return nil
}

func (m *mockLearningStore) Delete(ctx context.Context, id int64) error { return nil }

func (m *mockLearningStore) ListByWorkspace(ctx context.Context, workspaceID int64) ([]model.Learning, error) {
	if m.listByWorkspaceFn != nil {
		return m.listByWorkspaceFn(ctx, workspaceID)
	}
	return nil, nil
}

func (m *mockLearningStore) ListByWorkspaceAndType(ctx context.Context, workspaceID int64, learningType string) ([]model.Learning, error) {
	return nil, nil
}

var _ = Describe("ContextBuilder", func() {
	var (
		builder interface {
			BuildPlannerMessages(ctx context.Context, issue model.Issue, triggerThreadID string) ([]llm.Message, error)
		}
		mockInteg     *mockIntegrationStore
		mockConfig    *mockIntegrationConfigStore
		mockLearnings *mockLearningStore
		ctx           context.Context
		baseTime      time.Time
		workspaceID   int64
		integrationID int64
		relayUsername string
	)

	BeforeEach(func() {
		ctx = context.Background()
		baseTime = time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
		workspaceID = 100
		integrationID = 200
		relayUsername = "relaybot"

		mockInteg = &mockIntegrationStore{
			getByIDFn: func(ctx context.Context, id int64) (*model.Integration, error) {
				return &model.Integration{
					ID:          integrationID,
					WorkspaceID: workspaceID,
					Provider:    model.ProviderGitLab,
				}, nil
			},
		}

		mockConfig = &mockIntegrationConfigStore{
			getByIntegrationAndKeyFn: func(ctx context.Context, integrationID int64, key string) (*model.IntegrationConfig, error) {
				if key == model.ConfigKeyServiceAccount {
					value, _ := json.Marshal(model.ServiceAccountConfig{
						Username: relayUsername,
						UserID:   12345,
					})
					return &model.IntegrationConfig{
						Value: value,
					}, nil
				}
				return nil, store.ErrNotFound
			},
		}

		mockLearnings = &mockLearningStore{
			listByWorkspaceFn: func(ctx context.Context, wID int64) ([]model.Learning, error) {
				if wID == workspaceID {
					return []model.Learning{
						{ID: 1, Type: model.LearningTypeProjectStandards, Content: "Batch ops must be idempotent"},
						{ID: 2, Type: model.LearningTypeCodebaseStandards, Content: "Use JobQueue for >100 items"},
					}, nil
				}
				return nil, nil
			},
		}

		builder = brain.NewContextBuilder(mockInteg, mockConfig, mockLearnings)
	})

	Describe("BuildPlannerMessages", func() {
		Context("with a basic issue", func() {
			It("returns system message with self-identity", func() {
				title := "Add bulk refund support"
				desc := "We need to support bulk refunds"
				issue := model.Issue{
					ID:            1,
					IntegrationID: integrationID,
					Title:         &title,
					Description:   &desc,
				}

				messages, err := builder.BuildPlannerMessages(ctx, issue, "")

				Expect(err).NotTo(HaveOccurred())
				Expect(len(messages)).To(BeNumerically(">=", 2))

				// First message should be system prompt with self-identity
				Expect(messages[0].Role).To(Equal("system"))
				Expect(messages[0].Content).To(ContainSubstring("You are Relay"))
				Expect(messages[0].Content).To(ContainSubstring("@" + relayUsername))
			})

			It("includes context dump with issue metadata", func() {
				title := "Add bulk refund support"
				desc := "We need to support bulk refunds"
				reporter := "alice"
				issue := model.Issue{
					ID:            1,
					IntegrationID: integrationID,
					Title:         &title,
					Description:   &desc,
					Reporter:      &reporter,
					Assignees:     []string{"bob"},
				}

				messages, err := builder.BuildPlannerMessages(ctx, issue, "")

				Expect(err).NotTo(HaveOccurred())
				Expect(len(messages)).To(BeNumerically(">=", 2))

				// Second message should be context dump
				contextDump := messages[1]
				Expect(contextDump.Role).To(Equal("user"))
				Expect(contextDump.Content).To(ContainSubstring("Add bulk refund support"))
				Expect(contextDump.Content).To(ContainSubstring("@alice"))
				Expect(contextDump.Content).To(ContainSubstring("@bob"))
			})

			It("includes learnings from workspace", func() {
				title := "Test issue"
				issue := model.Issue{
					ID:            1,
					IntegrationID: integrationID,
					Title:         &title,
				}

				messages, err := builder.BuildPlannerMessages(ctx, issue, "")

				Expect(err).NotTo(HaveOccurred())

				// Context dump should include learnings
				contextDump := messages[1]
				Expect(contextDump.Content).To(ContainSubstring("Learnings"))
				Expect(contextDump.Content).To(ContainSubstring("project_standards"))
				Expect(contextDump.Content).To(ContainSubstring("Batch ops must be idempotent"))
				Expect(contextDump.Content).To(ContainSubstring("codebase_standards"))
				Expect(contextDump.Content).To(ContainSubstring("Use JobQueue for >100 items"))
			})
		})

		Context("with code findings", func() {
			It("includes code findings in context dump", func() {
				title := "Test issue"
				issue := model.Issue{
					ID:            1,
					IntegrationID: integrationID,
					Title:         &title,
					CodeFindings: []model.CodeFinding{
						{
							Synthesis: "PaymentService processes refunds synchronously",
							Sources: []model.CodeSource{
								{Location: "internal/payment/service.go:145", Snippet: "func processRefund()..."},
							},
						},
					},
				}

				messages, err := builder.BuildPlannerMessages(ctx, issue, "")

				Expect(err).NotTo(HaveOccurred())

				contextDump := messages[1]
				Expect(contextDump.Content).To(ContainSubstring("Code Findings"))
				Expect(contextDump.Content).To(ContainSubstring("internal/payment/service.go:145"))
				Expect(contextDump.Content).To(ContainSubstring("PaymentService processes refunds synchronously"))
			})
		})

		Context("with discussions", func() {
			It("maps relay comments to assistant role", func() {
				title := "Test issue"
				issue := model.Issue{
					ID:            1,
					IntegrationID: integrationID,
					Title:         &title,
					Discussions: []model.Discussion{
						{Author: "alice", Body: "Please help with this", CreatedAt: baseTime},
						{Author: relayUsername, Body: "I'll look into this", CreatedAt: baseTime.Add(time.Minute)},
					},
				}

				messages, err := builder.BuildPlannerMessages(ctx, issue, "")

				Expect(err).NotTo(HaveOccurred())

				// Should have system + context + 2 discussion messages
				Expect(len(messages)).To(Equal(4))

				// Third message: alice's comment -> user role
				Expect(messages[2].Role).To(Equal("user"))
				Expect(messages[2].Name).To(Equal("alice"))
				Expect(messages[2].Content).To(Equal("Please help with this"))

				// Fourth message: relay's comment -> assistant role
				Expect(messages[3].Role).To(Equal("assistant"))
				Expect(messages[3].Name).To(BeEmpty()) // assistant messages don't have Name
				Expect(messages[3].Content).To(Equal("I'll look into this"))
			})

			It("sorts discussions chronologically", func() {
				title := "Test issue"
				threadID := "thread1"
				issue := model.Issue{
					ID:            1,
					IntegrationID: integrationID,
					Title:         &title,
					Discussions: []model.Discussion{
						{Author: "charlie", Body: "Third", CreatedAt: baseTime.Add(2 * time.Minute)},
						{Author: "alice", Body: "First", CreatedAt: baseTime, ThreadID: &threadID},
						{Author: "bob", Body: "Second", CreatedAt: baseTime.Add(time.Minute)},
					},
				}

				messages, err := builder.BuildPlannerMessages(ctx, issue, "")

				Expect(err).NotTo(HaveOccurred())
				Expect(len(messages)).To(Equal(5)) // system + context + 3 discussions

				Expect(messages[2].Content).To(Equal("First"))
				Expect(messages[3].Content).To(Equal("Second"))
				Expect(messages[4].Content).To(Equal("Third"))
			})

			It("adds reply context for threaded discussions", func() {
				title := "Test issue"
				threadID := "thread1"
				issue := model.Issue{
					ID:            1,
					IntegrationID: integrationID,
					Title:         &title,
					Discussions: []model.Discussion{
						{Author: "alice", Body: "Original question", CreatedAt: baseTime, ThreadID: &threadID},
						{Author: "bob", Body: "Here's my answer", CreatedAt: baseTime.Add(time.Minute), ThreadID: &threadID},
					},
				}

				messages, err := builder.BuildPlannerMessages(ctx, issue, "")

				Expect(err).NotTo(HaveOccurred())
				Expect(len(messages)).To(Equal(4))

				// First thread message: no reply prefix
				Expect(messages[2].Content).To(Equal("Original question"))

				// Second thread message: has reply prefix
				Expect(messages[3].Content).To(ContainSubstring("(replying to @alice)"))
				Expect(messages[3].Content).To(ContainSubstring("Here's my answer"))
			})

			It("truncates to max 100 discussions keeping most recent", func() {
				title := "Test issue"
				discussions := make([]model.Discussion, 150)
				for i := 0; i < 150; i++ {
					discussions[i] = model.Discussion{
						Author:    "user",
						Body:      "Message " + string(rune('A'+i%26)),
						CreatedAt: baseTime.Add(time.Duration(i) * time.Minute),
					}
				}

				issue := model.Issue{
					ID:            1,
					IntegrationID: integrationID,
					Title:         &title,
					Discussions:   discussions,
				}

				messages, err := builder.BuildPlannerMessages(ctx, issue, "")

				Expect(err).NotTo(HaveOccurred())
				// system + context + 100 discussions (truncated)
				Expect(len(messages)).To(Equal(102))
			})

			It("sanitizes user names for API compatibility", func() {
				title := "Test issue"
				issue := model.Issue{
					ID:            1,
					IntegrationID: integrationID,
					Title:         &title,
					Discussions: []model.Discussion{
						{Author: "user.with.dots", Body: "Test", CreatedAt: baseTime},
						{Author: "user@example.com", Body: "Test2", CreatedAt: baseTime.Add(time.Minute)},
					},
				}

				messages, err := builder.BuildPlannerMessages(ctx, issue, "")

				Expect(err).NotTo(HaveOccurred())

				// Names should be sanitized (dots and @ replaced with underscores)
				Expect(messages[2].Name).To(Equal("user_with_dots"))
				Expect(messages[3].Name).To(Equal("user_example_com"))
			})
		})

		Context("error handling", func() {
			It("returns error when service account config not found", func() {
				mockConfig.getByIntegrationAndKeyFn = func(ctx context.Context, integrationID int64, key string) (*model.IntegrationConfig, error) {
					return nil, store.ErrNotFound
				}

				title := "Test"
				issue := model.Issue{
					ID:            1,
					IntegrationID: integrationID,
					Title:         &title,
				}

				_, err := builder.BuildPlannerMessages(ctx, issue, "")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("service account"))
			})

			It("returns error when integration not found", func() {
				mockInteg.getByIDFn = func(ctx context.Context, id int64) (*model.Integration, error) {
					return nil, store.ErrNotFound
				}

				title := "Test"
				issue := model.Issue{
					ID:            1,
					IntegrationID: integrationID,
					Title:         &title,
				}

				_, err := builder.BuildPlannerMessages(ctx, issue, "")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("integration"))
			})
		})
	})
})
