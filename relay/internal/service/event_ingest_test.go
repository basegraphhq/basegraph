package service_test

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"basegraph.co/relay/common/id"
	"basegraph.co/relay/internal/model"
	"basegraph.co/relay/internal/queue"
	"basegraph.co/relay/internal/service"
	"basegraph.co/relay/internal/service/issue_tracker"
	"basegraph.co/relay/internal/store"
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
	upsertFn                        func(ctx context.Context, issue *model.Issue) (*model.Issue, error)
	getByIntegrationAndExternalIDFn func(ctx context.Context, integrationID int64, externalID string) (*model.Issue, error)
	capturedIssue                   *model.Issue
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
	if m.getByIntegrationAndExternalIDFn != nil {
		return m.getByIntegrationAndExternalIDFn(ctx, integrationID, externalID)
	}
	return nil, store.ErrNotFound
}

func (m *mockIssueStore) QueueIfIdle(ctx context.Context, issueID int64) (bool, error) {
	return true, nil
}

func (m *mockIssueStore) ClaimQueued(ctx context.Context, issueID int64) (bool, *model.Issue, error) {
	return true, nil, nil
}

func (m *mockIssueStore) SetIdle(ctx context.Context, issueID int64) error {
	return nil
}

func (m *mockIssueStore) ReclaimStuckIssues(ctx context.Context, stuckDuration time.Duration, limit int) ([]int64, error) {
	return nil, nil
}

func (m *mockIssueStore) FindStuckQueuedIssues(ctx context.Context, stuckDuration time.Duration, limit int) ([]int64, error) {
	return nil, nil
}

func (m *mockIssueStore) ResetQueuedToIdle(ctx context.Context, issueID int64) error {
	return nil
}

func (m *mockIssueStore) GetByIDForUpdate(ctx context.Context, id int64) (*model.Issue, error) {
	return nil, nil
}

func (m *mockIssueStore) UpdateCodeFindings(ctx context.Context, id int64, findings []model.CodeFinding) error {
	return nil
}

func (m *mockIssueStore) UpdateSpec(ctx context.Context, id int64, spec *string) error {
	return nil
}

func (m *mockIssueStore) UpdateSpecStatus(ctx context.Context, id int64, status model.SpecStatus) error {
	return nil
}

type mockIssueTrackerService struct {
	fetchFn            func(ctx context.Context, params issue_tracker.FetchIssueParams) (*model.Issue, error)
	fetchDiscussionsFn func(ctx context.Context, params issue_tracker.FetchDiscussionsParams) ([]model.Discussion, error)
	isReplyToUserFn    func(ctx context.Context, params issue_tracker.IsReplyParams) (bool, error)
}

func (m *mockIssueTrackerService) FetchIssue(ctx context.Context, params issue_tracker.FetchIssueParams) (*model.Issue, error) {
	if m.fetchFn != nil {
		return m.fetchFn(ctx, params)
	}
	var (
		title    = "Test issue"
		desc     = "Test description"
		reporter = "nithin"
		url      = "https://gitlab.com/test/project/-/issues/1"
	)
	return &model.Issue{
		Title:            &title,
		Description:      &desc,
		Labels:           []string{"bug"},
		Assignees:        []string{"alice"},
		Reporter:         &reporter,
		ExternalIssueURL: &url,
	}, nil
}

func (m *mockIssueTrackerService) FetchDiscussions(ctx context.Context, params issue_tracker.FetchDiscussionsParams) ([]model.Discussion, error) {
	if m.fetchDiscussionsFn != nil {
		return m.fetchDiscussionsFn(ctx, params)
	}
	return nil, nil
}

func (m *mockIssueTrackerService) IsReplyToUser(ctx context.Context, params issue_tracker.IsReplyParams) (bool, error) {
	if m.isReplyToUserFn != nil {
		return m.isReplyToUserFn(ctx, params)
	}
	return false, nil
}

func (m *mockIssueTrackerService) CreateDiscussion(ctx context.Context, params issue_tracker.CreateDiscussionParams) (issue_tracker.CreateDiscussionResult, error) {
	return issue_tracker.CreateDiscussionResult{}, nil
}

func (m *mockIssueTrackerService) ReplyToThread(ctx context.Context, params issue_tracker.ReplyToThreadParams) (issue_tracker.ReplyToThreadResult, error) {
	return issue_tracker.ReplyToThreadResult{}, nil
}

func (m *mockIssueTrackerService) AddReaction(ctx context.Context, params issue_tracker.AddReactionParams) error {
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
	enqueueFn func(ctx context.Context, task queue.Task) error
}

func (m *mockQueueProducer) Enqueue(ctx context.Context, task queue.Task) error {
	if m.enqueueFn != nil {
		return m.enqueueFn(ctx, task)
	}
	return nil
}

func (m *mockQueueProducer) Close() error {
	return nil
}

// Mock EngagementDetector
type mockEngagementDetector struct {
	shouldEngageFn    func(ctx context.Context, integrationID int64, req service.EngagementRequest) (service.EngagementResult, error)
	isSelfTriggeredFn func(ctx context.Context, integrationID int64, username string) (bool, error)
}

func (m *mockEngagementDetector) ShouldEngage(ctx context.Context, integrationID int64, req service.EngagementRequest) (service.EngagementResult, error) {
	if m.shouldEngageFn != nil {
		return m.shouldEngageFn(ctx, integrationID, req)
	}
	return service.EngagementResult{ShouldEngage: false}, nil
}

func (m *mockEngagementDetector) IsSelfTriggered(ctx context.Context, integrationID int64, username string) (bool, error) {
	if m.isSelfTriggeredFn != nil {
		return m.isSelfTriggeredFn(ctx, integrationID, username)
	}
	return false, nil
}

var _ = Describe("EventIngestService", func() {
	var (
		svc               service.EventIngestService
		mockIntegrations  *mockIntegrationStore
		mockIssuesStore   *mockIssueStore
		mockIssues        *mockIssueStore
		mockEventLogs     *mockEventLogStore
		mockQueue         *mockQueueProducer
		mockProvider      *mockIssueTrackerService
		mockEngagementDet *mockEngagementDetector
		ctx               context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockIntegrations = &mockIntegrationStore{}
		mockIssuesStore = &mockIssueStore{}
		mockIssues = &mockIssueStore{}
		mockEventLogs = &mockEventLogStore{}
		mockQueue = &mockQueueProducer{}
		mockProvider = &mockIssueTrackerService{}
		mockEngagementDet = &mockEngagementDetector{}

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

		issueTrackers := map[model.Provider]issue_tracker.IssueTrackerService{
			model.ProviderGitLab: mockProvider,
			model.ProviderGitHub: mockProvider,
			model.ProviderLinear: mockProvider,
			model.ProviderJira:   mockProvider,
		}

		svc = service.NewEventIngestService(
			mockIntegrations,
			mockIssuesStore,
			txRunner,
			mockQueue,
			issueTrackers,
			mockEngagementDet,
		)
	})

	Describe("Ingest", func() {
		Context("with subscribed issue (already exists)", func() {
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
				// Issue already exists (subscribed)
				mockIssuesStore.getByIntegrationAndExternalIDFn = func(ctx context.Context, integrationID int64, externalID string) (*model.Issue, error) {
					return &model.Issue{
						ID:              12345,
						IntegrationID:   integrationID,
						ExternalIssueID: externalID,
						Provider:        model.ProviderGitLab,
					}, nil
				}
				// Configure engagement detector - subscribed issues still need @mention or reply
				mockEngagementDet.shouldEngageFn = func(ctx context.Context, integrationID int64, req service.EngagementRequest) (service.EngagementResult, error) {
					return service.EngagementResult{ShouldEngage: true}, nil
				}
			})

			It("processes event when engaged via @mention", func() {
				payload := json.RawMessage(`{"action":"open"}`)
				params := service.EventIngestParams{
					IntegrationID:       123,
					ExternalIssueID:     "42",
					ExternalProjectID:   1,
					Provider:            model.ProviderGitLab,
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

		Context("with new issue and @mention trigger", func() {
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
				// Issue doesn't exist
				mockIssuesStore.getByIntegrationAndExternalIDFn = func(ctx context.Context, integrationID int64, externalID string) (*model.Issue, error) {
					return nil, store.ErrNotFound
				}
				// Configure engagement detector to return true when @mentioned
				mockEngagementDet.shouldEngageFn = func(ctx context.Context, integrationID int64, req service.EngagementRequest) (service.EngagementResult, error) {
					engaged := req.IssueBody == "Hey @relaybot please help" || req.CommentBody == "cc @relaybot"
					return service.EngagementResult{ShouldEngage: engaged}, nil
				}
			})

			It("creates issue when triggered by @mention in issue body", func() {
				payload := json.RawMessage(`{"action":"opened"}`)
				params := service.EventIngestParams{
					IntegrationID:       456,
					ExternalIssueID:     "99",
					ExternalProjectID:   789,
					Provider:            model.ProviderGitHub,
					IssueBody:           "Hey @relaybot please help",
					TriggeredByUsername: "bob",
					EventType:           "issue_created",
					Payload:             payload,
				}

				result, err := svc.Ingest(ctx, params)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(mockIssues.capturedIssue.Provider).To(Equal(model.ProviderGitHub))
			})

			It("creates issue when triggered by @mention in comment", func() {
				payload := json.RawMessage(`{"action":"created"}`)
				params := service.EventIngestParams{
					IntegrationID:       456,
					ExternalIssueID:     "99",
					ExternalProjectID:   789,
					Provider:            model.ProviderGitHub,
					IssueBody:           "Some issue without mention",
					CommentBody:         "cc @relaybot",
					TriggeredByUsername: "bob",
					EventType:           "comment_created",
					Payload:             payload,
				}

				result, err := svc.Ingest(ctx, params)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(mockIssues.capturedIssue.Provider).To(Equal(model.ProviderGitHub))
			})

			It("ignores event when no @mention", func() {
				payload := json.RawMessage(`{"action":"opened"}`)
				params := service.EventIngestParams{
					IntegrationID:       456,
					ExternalIssueID:     "99",
					ExternalProjectID:   789,
					Provider:            model.ProviderGitHub,
					IssueBody:           "Some issue without any mention",
					TriggeredByUsername: "bob",
					EventType:           "issue_created",
					Payload:             payload,
				}

				result, err := svc.Ingest(ctx, params)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.EventPublished).To(BeFalse())
				Expect(result.IssuePickedUp).To(BeFalse())
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
					IntegrationID:       999, // does not exists, 456 exists
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

		Context("provider values with subscribed issues", func() {
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
					// Issue already subscribed
					mockIssuesStore.getByIntegrationAndExternalIDFn = func(ctx context.Context, integrationID int64, externalID string) (*model.Issue, error) {
						return &model.Issue{
							ID:              12345,
							IntegrationID:   integrationID,
							ExternalIssueID: externalID,
							Provider:        provider,
						}, nil
					}
					// Configure engagement detector - subscribed issues need @mention or reply to engage
					mockEngagementDet.shouldEngageFn = func(ctx context.Context, integrationID int64, req service.EngagementRequest) (service.EngagementResult, error) {
						return service.EngagementResult{ShouldEngage: true}, nil
					}

					payload := json.RawMessage(`{"action":"test"}`)
					params := service.EventIngestParams{
						IntegrationID:       100,
						ExternalIssueID:     "1",
						ExternalProjectID:   1,
						Provider:            provider,
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
