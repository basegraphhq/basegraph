package service_test

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/service"
	issue_tracker "basegraph.app/relay/internal/service/issue_tracker"
)

var _ = Describe("Engagement Detector Mention Logic", func() {
	Describe("ExtractMentions", func() {
		It("returns empty slice for text without mentions", func() {
			result := service.ExtractMentions("Hello, this has no mentions")
			Expect(result).To(BeEmpty())
		})

		It("extracts a single mention", func() {
			result := service.ExtractMentions("Hey @alice can you help?")
			Expect(result).To(Equal([]string{"alice"}))
		})

		It("extracts multiple mentions", func() {
			result := service.ExtractMentions("@alice @bob please review")
			Expect(result).To(ConsistOf("alice", "bob"))
		})

		It("deduplicates mentions", func() {
			result := service.ExtractMentions("@alice @bob @alice again")
			Expect(result).To(ConsistOf("alice", "bob"))
		})

		It("lowercases all mentions", func() {
			result := service.ExtractMentions("@ALICE @Bob @charlie")
			Expect(result).To(ConsistOf("alice", "bob", "charlie"))
		})

		It("handles mentions with hyphens and underscores", func() {
			result := service.ExtractMentions("@relay-bot @some_user help")
			Expect(result).To(ConsistOf("relay-bot", "some_user"))
		})

		It("handles mentions at end of sentence", func() {
			result := service.ExtractMentions("Question for @alice.")
			Expect(result).To(Equal([]string{"alice"}))
		})

		It("ignores email addresses", func() {
			result := service.ExtractMentions("email me at test@example.com")
			Expect(result).To(BeEmpty())
		})

		It("handles mixed emails and mentions", func() {
			result := service.ExtractMentions("contact test@example.com or ask @alice")
			Expect(result).To(Equal([]string{"alice"}))
		})

		It("handles mentions after punctuation", func() {
			result := service.ExtractMentions("Hey (@alice) can you help?")
			Expect(result).To(Equal([]string{"alice"}))
		})

		It("handles mention at start of text", func() {
			result := service.ExtractMentions("@alice please help")
			Expect(result).To(Equal([]string{"alice"}))
		})

		It("handles multiple emails without mentions", func() {
			result := service.ExtractMentions("Contact admin@company.com or support@company.com")
			Expect(result).To(BeEmpty())
		})
	})

	Describe("IsCommentDirectedAtOthers", func() {
		const relayUsername = "relay-bot"

		DescribeTable("engagement rules based on mentions",
			func(commentBody string, expectedDirectedAtOthers bool) {
				result := service.IsCommentDirectedAtOthers(commentBody, relayUsername)
				Expect(result).To(Equal(expectedDirectedAtOthers))
			},
			Entry("no mentions - not directed at others",
				"This is a general question about the approach",
				false),
			Entry("relay mentioned alone - not directed at others",
				"Hey @relay-bot can you help with this?",
				false),
			Entry("relay and others mentioned - not directed at others",
				"@alice @relay-bot please review this together",
				false),
			Entry("only another person mentioned - directed at others",
				"@alice what do you think about this?",
				true),
			Entry("multiple others mentioned without relay - directed at others",
				"@alice @bob thoughts on this approach?",
				true),
			Entry("case insensitive relay mention - not directed at others",
				"@RELAY-BOT please check this",
				false),
			Entry("case insensitive mixed - not directed at others",
				"@Alice @Relay-Bot review please",
				false),
			Entry("partial match should not count - directed at others",
				"@relay-bot-admin can you help?",
				true),
			Entry("email address only - not directed at others",
				"Contact me at test@example.com for details",
				false),
			Entry("email with mention of relay - not directed at others",
				"Email test@example.com or ask @relay-bot",
				false),
			Entry("email with mention of others - directed at others",
				"Email test@example.com or ask @alice",
				true),
		)
	})
})

var _ = Describe("Engagement Detector ShouldEngage", func() {
	var (
		detector      service.EngagementDetector
		mockConfig    *mockIntegrationConfigStore
		mockTracker   *mockIssueTrackerService
		ctx           context.Context
		integrationID int64
	)

	serviceAccountConfig := func() []byte {
		cfg := model.ServiceAccountConfig{
			UserID:   123,
			Username: "relay-bot",
		}
		data, _ := json.Marshal(cfg)
		return data
	}

	BeforeEach(func() {
		ctx = context.Background()
		integrationID = 1

		mockConfig = &mockIntegrationConfigStore{
			getByIntegrationAndKeyFn: func(_ context.Context, _ int64, _ string) (*model.IntegrationConfig, error) {
				return &model.IntegrationConfig{
					Value: serviceAccountConfig(),
				}, nil
			},
		}
		mockTracker = &mockIssueTrackerService{}

		detector = service.NewEngagementDetector(
			mockConfig,
			map[model.Provider]issue_tracker.IssueTrackerService{
				model.ProviderGitLab: mockTracker,
			},
		)
	})

	Describe("reply to Relay's top-level comment", func() {
		It("should engage when user replies to Relay's top-level comment", func() {
			// Relay posted a top-level comment (ThreadID is nil)
			// User replies to it (DiscussionID = Relay's comment ExternalID)
			relayCommentID := "note_12345"

			mockTracker.fetchDiscussionsFn = func(_ context.Context, _ issue_tracker.FetchDiscussionsParams) ([]model.Discussion, error) {
				return []model.Discussion{
					{
						ExternalID: relayCommentID,
						ThreadID:   nil, // Top-level comment has no parent
						Author:     "relay-bot",
						Body:       "I have a few questions...",
						CreatedAt:  time.Now(),
					},
				}, nil
			}

			result, err := detector.ShouldEngage(ctx, integrationID, service.EngagementRequest{
				Provider:          model.ProviderGitLab,
				CommentBody:       "Here are my answers",
				DiscussionID:      relayCommentID, // User is replying to Relay's comment
				ExternalProjectID: 100,
				ExternalIssueIID:  1,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.ShouldEngage).To(BeTrue())
		})

		It("should engage when user replies in a thread where Relay has replied", func() {
			// Alice started a thread, Relay replied in it
			aliceCommentID := "note_11111"
			relayReplyID := "note_22222"

			mockTracker.fetchDiscussionsFn = func(_ context.Context, _ issue_tracker.FetchDiscussionsParams) ([]model.Discussion, error) {
				return []model.Discussion{
					{
						ExternalID: aliceCommentID,
						ThreadID:   nil,
						Author:     "alice",
						Body:       "Original comment",
						CreatedAt:  time.Now().Add(-2 * time.Hour),
					},
					{
						ExternalID: relayReplyID,
						ThreadID:   &aliceCommentID,
						Author:     "relay-bot",
						Body:       "Relay's reply",
						CreatedAt:  time.Now().Add(-1 * time.Hour),
					},
				}, nil
			}

			result, err := detector.ShouldEngage(ctx, integrationID, service.EngagementRequest{
				Provider:          model.ProviderGitLab,
				CommentBody:       "New reply in thread",
				DiscussionID:      aliceCommentID, // Replying in Alice's thread
				ExternalProjectID: 100,
				ExternalIssueIID:  1,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.ShouldEngage).To(BeTrue())
		})

		It("should NOT engage when replying in a thread where Relay has not participated", func() {
			aliceCommentID := "note_11111"
			bobReplyID := "note_22222"

			mockTracker.fetchDiscussionsFn = func(_ context.Context, _ issue_tracker.FetchDiscussionsParams) ([]model.Discussion, error) {
				return []model.Discussion{
					{
						ExternalID: aliceCommentID,
						ThreadID:   nil,
						Author:     "alice",
						Body:       "Original comment",
						CreatedAt:  time.Now().Add(-2 * time.Hour),
					},
					{
						ExternalID: bobReplyID,
						ThreadID:   &aliceCommentID,
						Author:     "bob",
						Body:       "Bob's reply",
						CreatedAt:  time.Now().Add(-1 * time.Hour),
					},
				}, nil
			}

			result, err := detector.ShouldEngage(ctx, integrationID, service.EngagementRequest{
				Provider:          model.ProviderGitLab,
				CommentBody:       "Another reply in thread",
				DiscussionID:      aliceCommentID,
				ExternalProjectID: 100,
				ExternalIssueIID:  1,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.ShouldEngage).To(BeFalse())
		})

		It("should NOT engage when comment is directed at someone else", func() {
			relayCommentID := "note_12345"

			mockTracker.fetchDiscussionsFn = func(_ context.Context, _ issue_tracker.FetchDiscussionsParams) ([]model.Discussion, error) {
				return []model.Discussion{
					{
						ExternalID: relayCommentID,
						ThreadID:   nil,
						Author:     "relay-bot",
						Body:       "I have a few questions...",
						CreatedAt:  time.Now(),
					},
				}, nil
			}

			result, err := detector.ShouldEngage(ctx, integrationID, service.EngagementRequest{
				Provider:          model.ProviderGitLab,
				CommentBody:       "@alice what do you think?", // Directed at Alice, not Relay
				DiscussionID:      relayCommentID,
				ExternalProjectID: 100,
				ExternalIssueIID:  1,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.ShouldEngage).To(BeFalse())
		})
	})
})
