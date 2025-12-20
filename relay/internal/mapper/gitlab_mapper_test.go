package mapper_test

import (
	"context"

	"basegraph.app/relay/internal/mapper"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GitLabMapper", func() {
	var (
		gitlabMapper mapper.EventMapper
		ctx          context.Context
	)

	BeforeEach(func() {
		gitlabMapper = mapper.NewGitLabEventMapper()
		ctx = context.Background()
	})

	Describe("Map", func() {
		Context("when mapping Issue Hook events", func() {
			It("should map Issue Hook to issue_created", func() {
				headers := map[string]string{
					"X-Gitlab-Event": "Issue Hook",
				}
				body := map[string]any{
					"object_kind": "issue",
					"object_attributes": map[string]any{
						"action": "open",
					},
				}

				eventType, err := gitlabMapper.Map(ctx, body, headers)
				Expect(err).ToNot(HaveOccurred())
				Expect(eventType).To(Equal(mapper.EventIssueCreated))
			})

			It("should map Issue Hook with any action to issue_created", func() {
				testCases := []string{"open", "close", "update", "reopen"}

				for _, action := range testCases {
					headers := map[string]string{
						"X-Gitlab-Event": "Issue Hook",
					}
					body := map[string]any{
						"object_kind": "issue",
						"object_attributes": map[string]any{
							"action": action,
						},
					}

					eventType, err := gitlabMapper.Map(ctx, body, headers)
					Expect(err).ToNot(HaveOccurred())
					Expect(eventType).To(Equal(mapper.EventIssueCreated))
				}
			})
		})

		Context("when mapping Note Hook events", func() {
			It("should map Note Hook to reply", func() {
				headers := map[string]string{
					"X-Gitlab-Event": "Note Hook",
				}
				body := map[string]any{
					"object_kind": "note",
					"object_attributes": map[string]any{
						"noteable_type": "Issue",
					},
				}

				eventType, err := gitlabMapper.Map(ctx, body, headers)
				Expect(err).ToNot(HaveOccurred())
				Expect(eventType).To(Equal(mapper.EventReply))
			})

			It("should map Note Hook on any noteable_type to reply", func() {
				testCases := []string{"Issue", "MergeRequest", "Commit", "Snippet"}

				for _, noteableType := range testCases {
					headers := map[string]string{
						"X-Gitlab-Event": "Note Hook",
					}
					body := map[string]any{
						"object_kind": "note",
						"object_attributes": map[string]any{
							"noteable_type": noteableType,
						},
					}

					eventType, err := gitlabMapper.Map(ctx, body, headers)
					Expect(err).ToNot(HaveOccurred())
					Expect(eventType).To(Equal(mapper.EventReply))
				}
			})
		})

		Context("when mapping Merge Request Hook events", func() {
			It("should map Merge Request Hook to pull_request_created", func() {
				headers := map[string]string{
					"X-Gitlab-Event": "Merge Request Hook",
				}
				body := map[string]any{
					"object_kind": "merge_request",
					"object_attributes": map[string]any{
						"action": "open",
					},
				}

				eventType, err := gitlabMapper.Map(ctx, body, headers)
				Expect(err).ToNot(HaveOccurred())
				Expect(eventType).To(Equal(mapper.EventPRCreated))
			})

			It("should map Merge Request Hook with any action to pull_request_created", func() {
				testCases := []string{"open", "close", "update", "merge"}

				for _, action := range testCases {
					headers := map[string]string{
						"X-Gitlab-Event": "Merge Request Hook",
					}
					body := map[string]any{
						"object_kind": "merge_request",
						"object_attributes": map[string]any{
							"action": action,
						},
					}

					eventType, err := gitlabMapper.Map(ctx, body, headers)
					Expect(err).ToNot(HaveOccurred())
					Expect(eventType).To(Equal(mapper.EventPRCreated))
				}
			})
		})

		Context("when handling unsupported events", func() {
			It("should error on Push Hook events", func() {
				headers := map[string]string{
					"X-Gitlab-Event": "Push Hook",
				}
				body := map[string]any{
					"object_kind": "push",
				}

				eventType, err := gitlabMapper.Map(ctx, body, headers)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("unknown gitlab event type")))
				Expect(eventType).To(Equal(mapper.CanonicalEventType("")))
			})

			It("should error on Pipeline Hook events", func() {
				headers := map[string]string{
					"X-Gitlab-Event": "Pipeline Hook",
				}
				body := map[string]any{
					"object_kind": "pipeline",
				}

				eventType, err := gitlabMapper.Map(ctx, body, headers)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("unknown gitlab event type")))
				Expect(eventType).To(BeEmpty())
			})
		})

		Context("when handling edge cases", func() {
			It("should fallback to object_kind when header is missing", func() {
				headers := map[string]string{}
				body := map[string]any{
					"object_kind": "issue",
				}

				eventType, err := gitlabMapper.Map(ctx, body, headers)
				Expect(err).ToNot(HaveOccurred())
				Expect(eventType).To(Equal(mapper.EventIssueCreated))
			})

			It("should fallback to header when object_kind is missing", func() {
				headers := map[string]string{
					"X-Gitlab-Event": "Issue Hook",
				}
				body := map[string]any{}

				eventType, err := gitlabMapper.Map(ctx, body, headers)
				Expect(err).ToNot(HaveOccurred())
				Expect(eventType).To(Equal(mapper.EventIssueCreated))
			})

			It("should error when both header and object_kind are missing", func() {
				headers := map[string]string{}
				body := map[string]any{}

				eventType, err := gitlabMapper.Map(ctx, body, headers)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("unknown gitlab event type")))
				Expect(eventType).To(BeEmpty())
			})

			It("should handle nil body gracefully", func() {
				headers := map[string]string{
					"X-Gitlab-Event": "Issue Hook",
				}

				eventType, err := gitlabMapper.Map(ctx, nil, headers)
				// The implementation doesn't explicitly check for nil, so it should
				// handle it based on header
				Expect(err).ToNot(HaveOccurred())
				Expect(eventType).To(Equal(mapper.EventIssueCreated))
			})
		})
	})
})