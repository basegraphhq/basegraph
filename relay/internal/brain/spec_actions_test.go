package brain_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"basegraph.app/relay/internal/brain"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/store"
)

// Mock stores for spec action tests

type mockSpecActionIssueStore struct {
	issue     *model.Issue
	upsertErr error
}

func (m *mockSpecActionIssueStore) GetByID(ctx context.Context, id int64) (*model.Issue, error) {
	if m.issue != nil && m.issue.ID == id {
		return m.issue, nil
	}
	return nil, store.ErrNotFound
}

func (m *mockSpecActionIssueStore) GetByIntegrationAndExternalID(ctx context.Context, integrationID int64, externalID string) (*model.Issue, error) {
	return nil, nil
}

func (m *mockSpecActionIssueStore) Upsert(ctx context.Context, issue *model.Issue) (*model.Issue, error) {
	if m.upsertErr != nil {
		return nil, m.upsertErr
	}
	m.issue = issue
	return issue, nil
}

func (m *mockSpecActionIssueStore) ClaimQueued(ctx context.Context, issueID int64) (bool, *model.Issue, error) {
	return false, nil, nil
}

func (m *mockSpecActionIssueStore) SetIdle(ctx context.Context, issueID int64) error {
	return nil
}

func (m *mockSpecActionIssueStore) QueueIfIdle(ctx context.Context, issueID int64) (bool, error) {
	return false, nil
}

func (m *mockSpecActionIssueStore) ResetQueuedToIdle(ctx context.Context, issueID int64) error {
	return nil
}

type mockSpecActionGapStore struct {
	closedGaps []model.Gap
}

func (m *mockSpecActionGapStore) Create(ctx context.Context, gap model.Gap) (model.Gap, error) {
	return model.Gap{}, nil
}

func (m *mockSpecActionGapStore) GetByID(ctx context.Context, id int64) (model.Gap, error) {
	return model.Gap{}, nil
}

func (m *mockSpecActionGapStore) GetByShortID(ctx context.Context, shortID int64) (model.Gap, error) {
	return model.Gap{}, nil
}

func (m *mockSpecActionGapStore) ListByIssue(ctx context.Context, issueID int64) ([]model.Gap, error) {
	return nil, nil
}

func (m *mockSpecActionGapStore) ListOpenByIssue(ctx context.Context, issueID int64) ([]model.Gap, error) {
	return nil, nil
}

func (m *mockSpecActionGapStore) ListClosedByIssue(ctx context.Context, issueID int64, limit int32) ([]model.Gap, error) {
	return m.closedGaps, nil
}

func (m *mockSpecActionGapStore) Resolve(ctx context.Context, id int64) (model.Gap, error) {
	return model.Gap{}, nil
}

func (m *mockSpecActionGapStore) Skip(ctx context.Context, id int64) (model.Gap, error) {
	return model.Gap{}, nil
}

func (m *mockSpecActionGapStore) Close(ctx context.Context, id int64, status model.GapStatus, reason, note string) (model.Gap, error) {
	return model.Gap{}, nil
}

func (m *mockSpecActionGapStore) SetLearning(ctx context.Context, id int64, learningID int64) (model.Gap, error) {
	return model.Gap{}, nil
}

func (m *mockSpecActionGapStore) CountOpenBlocking(ctx context.Context, issueID int64) (int64, error) {
	return 0, nil
}

type mockSpecActionIntegrationStore struct {
	integration *model.Integration
}

func (m *mockSpecActionIntegrationStore) GetByID(ctx context.Context, id int64) (*model.Integration, error) {
	if m.integration != nil {
		return m.integration, nil
	}
	return nil, store.ErrNotFound
}

func (m *mockSpecActionIntegrationStore) GetByWorkspaceAndProvider(ctx context.Context, workspaceID int64, provider model.Provider) (*model.Integration, error) {
	return nil, nil
}

func (m *mockSpecActionIntegrationStore) Create(ctx context.Context, integration *model.Integration) error {
	return nil
}

func (m *mockSpecActionIntegrationStore) Update(ctx context.Context, integration *model.Integration) error {
	return nil
}

func (m *mockSpecActionIntegrationStore) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	return nil
}

func (m *mockSpecActionIntegrationStore) Delete(ctx context.Context, id int64) error {
	return nil
}

func (m *mockSpecActionIntegrationStore) ListByWorkspace(ctx context.Context, workspaceID int64) ([]model.Integration, error) {
	return nil, nil
}

func (m *mockSpecActionIntegrationStore) ListByOrganization(ctx context.Context, orgID int64) ([]model.Integration, error) {
	return nil, nil
}

func (m *mockSpecActionIntegrationStore) ListByCapability(ctx context.Context, workspaceID int64, capability model.Capability) ([]model.Integration, error) {
	return nil, nil
}

type mockSpecActionLearningStore struct {
	learnings []model.Learning
}

func (m *mockSpecActionLearningStore) GetByID(ctx context.Context, id int64) (*model.Learning, error) {
	return nil, nil
}

func (m *mockSpecActionLearningStore) GetByWorkspaceAndType(ctx context.Context, workspaceID int64, learningType string) (*model.Learning, error) {
	return nil, nil
}

func (m *mockSpecActionLearningStore) Create(ctx context.Context, learning *model.Learning) error {
	return nil
}

func (m *mockSpecActionLearningStore) Update(ctx context.Context, learning *model.Learning) error {
	return nil
}

func (m *mockSpecActionLearningStore) Delete(ctx context.Context, id int64) error {
	return nil
}

func (m *mockSpecActionLearningStore) ListByWorkspace(ctx context.Context, workspaceID int64) ([]model.Learning, error) {
	return m.learnings, nil
}

func (m *mockSpecActionLearningStore) ListByWorkspaceAndType(ctx context.Context, workspaceID int64, learningType string) ([]model.Learning, error) {
	return nil, nil
}

type mockSpecActionConfigStore struct{}

func (m *mockSpecActionConfigStore) GetByID(ctx context.Context, id int64) (*model.IntegrationConfig, error) {
	return nil, nil
}

func (m *mockSpecActionConfigStore) GetByIntegrationAndKey(ctx context.Context, integrationID int64, key string) (*model.IntegrationConfig, error) {
	return &model.IntegrationConfig{
		Value: []byte(`{"username":"relay-bot"}`),
	}, nil
}

func (m *mockSpecActionConfigStore) ListByIntegration(ctx context.Context, integrationID int64) ([]model.IntegrationConfig, error) {
	return nil, nil
}

func (m *mockSpecActionConfigStore) ListByIntegrationAndType(ctx context.Context, integrationID int64, configType string) ([]model.IntegrationConfig, error) {
	return nil, nil
}

func (m *mockSpecActionConfigStore) Create(ctx context.Context, config *model.IntegrationConfig) error {
	return nil
}

func (m *mockSpecActionConfigStore) Update(ctx context.Context, config *model.IntegrationConfig) error {
	return nil
}

func (m *mockSpecActionConfigStore) Upsert(ctx context.Context, config *model.IntegrationConfig) error {
	return nil
}

func (m *mockSpecActionConfigStore) Delete(ctx context.Context, id int64) error {
	return nil
}

func (m *mockSpecActionConfigStore) DeleteByIntegration(ctx context.Context, integrationID int64) error {
	return nil
}

type mockSpecActionSpecStore struct {
	specs   map[string]string // path -> content
	refs    map[string]model.SpecRef
	readErr error
}

func newMockSpecActionSpecStore() *mockSpecActionSpecStore {
	return &mockSpecActionSpecStore{
		specs: make(map[string]string),
		refs:  make(map[string]model.SpecRef),
	}
}

func (m *mockSpecActionSpecStore) Read(ctx context.Context, ref model.SpecRef) (string, model.SpecMeta, error) {
	if m.readErr != nil {
		return "", model.SpecMeta{}, m.readErr
	}
	content, ok := m.specs[ref.Path]
	if !ok {
		return "", model.SpecMeta{}, store.ErrSpecNotFound
	}
	return content, model.SpecMeta{
		UpdatedAt: time.Now(),
		SHA256:    ref.SHA256,
	}, nil
}

func (m *mockSpecActionSpecStore) Write(ctx context.Context, issueID int64, provider, externalIssueID, slug, content string) (model.SpecRef, error) {
	if content == "" {
		return model.SpecRef{}, fmt.Errorf("spec content cannot be empty")
	}
	path := "issue_" + externalIssueID + "_" + slug + "/spec.md"
	ref := model.SpecRef{
		Version:   1,
		Backend:   "local",
		Path:      path,
		UpdatedAt: time.Now(),
		SHA256:    "mock_sha256_" + externalIssueID,
		Format:    "markdown",
	}
	m.specs[path] = content
	m.refs[path] = ref
	return ref, nil
}

func (m *mockSpecActionSpecStore) Exists(ctx context.Context, ref model.SpecRef) (bool, error) {
	_, ok := m.specs[ref.Path]
	return ok, nil
}

var _ = Describe("Spec Actions", func() {
	var (
		ctx              context.Context
		mockIssues       *mockSpecActionIssueStore
		mockGaps         *mockSpecActionGapStore
		mockIntegrations *mockSpecActionIntegrationStore
		mockLearnings    *mockSpecActionLearningStore
		mockSpecStore    *mockSpecActionSpecStore
		integrationID    int64
		workspaceID      int64
	)

	BeforeEach(func() {
		ctx = context.Background()
		integrationID = 100
		workspaceID = 200

		mockIntegrations = &mockSpecActionIntegrationStore{
			integration: &model.Integration{
				ID:          integrationID,
				WorkspaceID: workspaceID,
			},
		}
		mockLearnings = &mockSpecActionLearningStore{
			learnings: []model.Learning{
				{ID: 1, Type: "code_learnings", Content: "Always wrap errors"},
			},
		}
		mockGaps = &mockSpecActionGapStore{
			closedGaps: []model.Gap{
				{
					ID:           1,
					ShortID:      11,
					Question:     "What is the expected behavior?",
					Status:       model.GapStatusResolved,
					ClosedReason: "answered",
					ClosedNote:   "Use existing pattern",
				},
			},
		}
		mockSpecStore = newMockSpecActionSpecStore()
	})

	Describe("update_spec action", func() {
		BeforeEach(func() {
			title := "Test Issue"
			mockIssues = &mockSpecActionIssueStore{
				issue: &model.Issue{
					ID:              1,
					IntegrationID:   integrationID,
					ExternalIssueID: "123",
					Provider:        model.ProviderGitLab,
					Title:           &title,
				},
			}
		})

		It("writes spec to store and updates issue", func() {
			executor := brain.NewActionExecutor(nil, mockIssues, mockGaps, mockIntegrations, &mockSpecActionConfigStore{}, mockLearnings, mockSpecStore, nil)

			dataJSON, _ := json.Marshal(map[string]any{
				"content_markdown": "# Test Spec\n\nThis is a test.",
				"reason":           "Initial spec creation",
				"mode":             "overwrite",
			})

			action := brain.Action{
				Type: brain.ActionTypeUpdateSpec,
				Data: dataJSON,
			}

			errs := executor.ExecuteBatch(ctx, *mockIssues.issue, []brain.Action{action})

			Expect(errs).To(BeEmpty())

			// Verify spec was written
			Expect(mockSpecStore.specs).To(HaveLen(1))

			// Verify issue was updated with spec ref
			Expect(mockIssues.issue.Spec).NotTo(BeNil())

			var ref model.SpecRef
			err := json.Unmarshal([]byte(*mockIssues.issue.Spec), &ref)
			Expect(err).NotTo(HaveOccurred())
			Expect(ref.Backend).To(Equal("local"))
			Expect(ref.Path).To(ContainSubstring("123"))
		})

		It("returns error when content is empty", func() {
			executor := brain.NewActionExecutor(nil, mockIssues, mockGaps, mockIntegrations, &mockSpecActionConfigStore{}, mockLearnings, mockSpecStore, nil)

			dataJSON, _ := json.Marshal(map[string]any{
				"content_markdown": "",
				"mode":             "overwrite",
			})

			action := brain.Action{
				Type: brain.ActionTypeUpdateSpec,
				Data: dataJSON,
			}

			errs := executor.ExecuteBatch(ctx, *mockIssues.issue, []brain.Action{action})

			Expect(errs).To(HaveLen(1))
		})
	})

	Describe("SpecStore integration", func() {
		It("can write and read back spec content", func() {
			content := "# Test Spec\n\nThis is a test."

			// Write
			ref, err := mockSpecStore.Write(ctx, 1, "gitlab", "123", "test", content)
			Expect(err).NotTo(HaveOccurred())
			Expect(ref.Backend).To(Equal("local"))
			Expect(ref.Path).To(ContainSubstring("123"))

			// Read
			readContent, meta, err := mockSpecStore.Read(ctx, ref)
			Expect(err).NotTo(HaveOccurred())
			Expect(readContent).To(Equal(content))
			Expect(meta.SHA256).To(Equal(ref.SHA256))
		})

		It("returns error when spec not found", func() {
			_, _, err := mockSpecStore.Read(ctx, model.SpecRef{Path: "nonexistent/spec.md"})
			Expect(err).To(Equal(store.ErrSpecNotFound))
		})

		It("checks spec existence correctly", func() {
			// Non-existent
			exists, err := mockSpecStore.Exists(ctx, model.SpecRef{Path: "nonexistent/spec.md"})
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse())

			// Write and check
			ref, err := mockSpecStore.Write(ctx, 1, "gitlab", "456", "test", "content")
			Expect(err).NotTo(HaveOccurred())

			exists, err = mockSpecStore.Exists(ctx, ref)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue())
		})
	})
})
