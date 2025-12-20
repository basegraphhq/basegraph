package service_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"basegraph.app/relay/common/id"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/queue"
	"basegraph.app/relay/internal/service"
	"basegraph.app/relay/internal/store"
)

// Mock IntegrationStore - now easy to mock since it's an interface
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

func (m *mockIntegrationStore) Delete(ctx context.Context, id int64) error {
	return nil
}

func (m *mockIntegrationStore) ListByWorkspace(ctx context.Context, workspaceID int64) ([]model.Integration, error) {
	return nil, nil
}

func (m *mockIntegrationStore) ListByOrganization(ctx context.Context, organizationID int64) ([]model.Integration, error) {
	return nil, nil
}

func (m *mockIntegrationStore) ListByCapability(ctx context.Context, workspaceID int64, capability model.Capability) ([]model.Integration, error) {
	return nil, nil
}

// Mock IssueStore
type mockIssueStore struct {
	upsertFn      func(ctx context.Context, issue *model.Issue) (*model.Issue, error)
	capturedIssue *model.Issue
}

func (m *mockIssueStore) Upsert(ctx context.Context, issue *model.Issue) (*model.Issue, error) {
	m.capturedIssue = issue
	if m.upsertFn != nil {
		return m.upsertFn(ctx, issue)
	}
	return issue, nil
}

func (m *mockIssueStore) GetByID(ctx context.Context, id int64) (*model.Issue, error) {
	return nil, nil
}

func (m *mockIssueStore) GetByIntegrationAndExternalID(ctx context.Context, integrationID int64, externalID string) (*model.Issue, error) {
	return nil, store.ErrNotFound
}

func (m *mockIssueStore) QueueIfIdle(ctx context.Context, issueID int64) (bool, error) {
	return true, nil
}

func (m *mockIssueStore) ClaimQueued(ctx context.Context, issueID int64) (bool, *model.Issue, error) {
	return true, nil, nil
}

func (m *mockIssueStore) SetProcessed(ctx context.Context, issueID int64) error {
	return nil
}

// Mock EventLogStore
type mockEventLogStore struct {
	createOrGetFn func(ctx context.Context, log *model.EventLog) (*model.EventLog, bool, error)
}

func (m *mockEventLogStore) Create(ctx context.Context, eventLog *model.EventLog) (*model.EventLog, error) {
	return eventLog, nil
}

func (m *mockEventLogStore) CreateOrGet(ctx context.Context, log *model.EventLog) (*model.EventLog, bool, error) {
	if m.createOrGetFn != nil {
		return m.createOrGetFn(ctx, log)
	}
	return log, false, nil
}

func (m *mockEventLogStore) GetByID(ctx context.Context, id int64) (*model.EventLog, error) {
	return nil, nil
}

func (m *mockEventLogStore) ListUnprocessed(ctx context.Context, limit int32) ([]model.EventLog, error) {
	return nil, nil
}

func (m *mockEventLogStore) MarkProcessed(ctx context.Context, id int64) error {
	return nil
}

func (m *mockEventLogStore) MarkFailed(ctx context.Context, id int64, errMsg string) error {
	return nil
}

func (m *mockEventLogStore) ListUnprocessedByIssue(ctx context.Context, issueID int64) ([]model.EventLog, error) {
	return nil, nil
}

func (m *mockEventLogStore) MarkBatchProcessed(ctx context.Context, ids []int64) error {
	return nil
}

// Mock QueueProducer
type mockQueueProducer struct {
	enqueueFn func(ctx context.Context, msg queue.EventMessage) error
}

func (m *mockQueueProducer) Enqueue(ctx context.Context, msg queue.EventMessage) error {
	if m.enqueueFn != nil {
		return m.enqueueFn(ctx, msg)
	}
	return nil
}

func (m *mockQueueProducer) Close() error {
	return nil
}

var _ = Describe("EventIngestService", func() {
	var (
		svc              service.EventIngestService
		mockIntegrations *mockIntegrationStore
		mockIssues       *mockIssueStore
		mockEventLogs    *mockEventLogStore
		mockQueue        *mockQueueProducer
		ctx              context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockIntegrations = &mockIntegrationStore{}
		mockIssues = &mockIssueStore{}
		mockEventLogs = &mockEventLogStore{}
		mockQueue = &mockQueueProducer{}

		err := id.Init(1)
		Expect(err).NotTo(HaveOccurred())

		// Create TxRunner that uses our mocks
		txRunner := &mockTxRunner{
			withTxFn: func(ctx context.Context, fn func(stores service.StoreProvider) error) error {
				return fn(&mockStoreProvider{
					issue:    mockIssues,
					eventLog: mockEventLogs,
				})
			},
		}

		svc = service.NewEventIngestService(mockIntegrations, txRunner, mockQueue)
	})

	Describe("Ingest", func() {
		Context("with valid GitLab integration", func() {
			BeforeEach(func() {
				mockIntegrations.getByIDFn = func(ctx context.Context, id int64) (*model.Integration, error) {
					return &model.Integration{
						ID:             123,
						WorkspaceID:    1,
						Provider:       model.ProviderGitLab,
						IsEnabled:      true,
						OrganizationID: 1,
						SetupByUserID:  1,
					}, nil
				}
			})

			It("creates issue with provider=gitlab", func() {
				payload := json.RawMessage(`{"action":"open"}`)
				params := service.EventIngestParams{
					IntegrationID:       123,
					ExternalIssueID:     "42",
					TriggeredByUsername: "alice",
					EventType:           "issue_created",
					Payload:             payload,
				}

				result, err := svc.Ingest(ctx, params)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Issue).NotTo(BeNil())

				// Verify issue has correct provider
				Expect(mockIssues.capturedIssue).NotTo(BeNil())
				Expect(mockIssues.capturedIssue.Provider).To(Equal(model.ProviderGitLab))
				Expect(mockIssues.capturedIssue.IntegrationID).To(Equal(int64(123)))
				Expect(mockIssues.capturedIssue.ExternalIssueID).To(Equal("42"))
			})
		})

		Context("with valid GitHub integration", func() {
			BeforeEach(func() {
				mockIntegrations.getByIDFn = func(ctx context.Context, id int64) (*model.Integration, error) {
					return &model.Integration{
						ID:             456,
						WorkspaceID:    1,
						Provider:       model.ProviderGitHub,
						IsEnabled:      true,
						OrganizationID: 1,
						SetupByUserID:  1,
					}, nil
				}
			})

			It("creates issue with provider=github", func() {
				payload := json.RawMessage(`{"action":"opened"}`)
				params := service.EventIngestParams{
					IntegrationID:       456,
					ExternalIssueID:     "99",
					TriggeredByUsername: "bob",
					EventType:           "issue_created",
					Payload:             payload,
				}

				_, err := svc.Ingest(ctx, params)

				Expect(err).NotTo(HaveOccurred())
				Expect(mockIssues.capturedIssue.Provider).To(Equal(model.ProviderGitHub))
			})
		})

		Context("when integration not found", func() {
			BeforeEach(func() {
				mockIntegrations.getByIDFn = func(ctx context.Context, id int64) (*model.Integration, error) {
					return nil, store.ErrNotFound
				}
			})

			It("returns ErrIntegrationNotFound", func() {
				payload := json.RawMessage(`{"action":"open"}`)
				params := service.EventIngestParams{
					IntegrationID:       999,
					ExternalIssueID:     "42",
					TriggeredByUsername: "alice",
					EventType:           "issue_created",
					Payload:             payload,
				}

				_, err := svc.Ingest(ctx, params)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("integration not found"))
				Expect(mockIssues.capturedIssue).To(BeNil())
			})
		})

		Context("when integration is disabled", func() {
			BeforeEach(func() {
				mockIntegrations.getByIDFn = func(ctx context.Context, id int64) (*model.Integration, error) {
					return &model.Integration{
						ID:             123,
						WorkspaceID:    1,
						Provider:       model.ProviderGitLab,
						IsEnabled:      false,
						OrganizationID: 1,
						SetupByUserID:  1,
					}, nil
				}
			})

			It("returns error without creating issue", func() {
				payload := json.RawMessage(`{"action":"open"}`)
				params := service.EventIngestParams{
					IntegrationID:       123,
					ExternalIssueID:     "42",
					TriggeredByUsername: "alice",
					EventType:           "issue_created",
					Payload:             payload,
				}

				_, err := svc.Ingest(ctx, params)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("disabled"))
				Expect(mockIssues.capturedIssue).To(BeNil())
			})
		})

		Context("with missing required params", func() {
			It("returns error when integration_id is zero", func() {
				payload := json.RawMessage(`{"action":"open"}`)
				params := service.EventIngestParams{
					IntegrationID:       0,
					ExternalIssueID:     "42",
					TriggeredByUsername: "alice",
					EventType:           "issue_created",
					Payload:             payload,
				}

				_, err := svc.Ingest(ctx, params)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("required"))
			})

			It("returns error when payload is empty", func() {
				params := service.EventIngestParams{
					IntegrationID:       123,
					ExternalIssueID:     "42",
					TriggeredByUsername: "alice",
					EventType:           "issue_created",
					Payload:             nil,
				}

				_, err := svc.Ingest(ctx, params)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("payload"))
			})
		})

		Context("provider values", func() {
			DescribeTable("creates issue with correct provider",
				func(provider model.Provider) {
					mockIntegrations.getByIDFn = func(ctx context.Context, id int64) (*model.Integration, error) {
						return &model.Integration{
							ID:             100,
							WorkspaceID:    1,
							Provider:       provider,
							IsEnabled:      true,
							OrganizationID: 1,
							SetupByUserID:  1,
						}, nil
					}

					payload := json.RawMessage(`{"action":"test"}`)
					params := service.EventIngestParams{
						IntegrationID:       100,
						ExternalIssueID:     "1",
						TriggeredByUsername: "user",
						EventType:           "issue_created",
						Payload:             payload,
					}

					_, err := svc.Ingest(ctx, params)

					Expect(err).NotTo(HaveOccurred())
					Expect(mockIssues.capturedIssue.Provider).To(Equal(provider))
				},
				Entry("GitLab", model.ProviderGitLab),
				Entry("GitHub", model.ProviderGitHub),
				Entry("Linear", model.ProviderLinear),
				Entry("Jira", model.ProviderJira),
			)
		})
	})
})
