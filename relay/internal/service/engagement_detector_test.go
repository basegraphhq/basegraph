package service_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"basegraph.app/relay/internal/service"
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
