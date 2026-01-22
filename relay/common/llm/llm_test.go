package llm_test

import (
	"strings"

	"basegraph.co/relay/common/llm"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SanitizeName", func() {
	DescribeTable("sanitizes usernames for OpenAI name parameter",
		func(input, expected string) {
			Expect(llm.SanitizeName(input)).To(Equal(expected))
		},
		Entry("valid name unchanged", "alice", "alice"),
		Entry("dots replaced with underscore", "alice.smith", "alice_smith"),
		Entry("@ replaced with underscore", "alice@dev", "alice_dev"),
		Entry("hyphens preserved", "alice-dev", "alice-dev"),
		Entry("underscores preserved", "alice_dev", "alice_dev"),
		Entry("numbers preserved", "alice123", "alice123"),
		Entry("mixed case preserved", "AliceSmith", "AliceSmith"),
		Entry("multiple special chars replaced", "alice.smith@dev!", "alice_smith_dev_"),
		Entry("spaces replaced", "alice smith", "alice_smith"),
		Entry("long name truncated to 64 chars", strings.Repeat("a", 100), strings.Repeat("a", 64)),
		Entry("exactly 64 chars unchanged", strings.Repeat("b", 64), strings.Repeat("b", 64)),
		Entry("empty string unchanged", "", ""),
	)
})

var _ = Describe("Message", func() {
	Describe("Name field", func() {
		It("accepts a name for user messages", func() {
			msg := llm.Message{
				Role:    "user",
				Name:    "alice",
				Content: "Hello world",
			}
			Expect(msg.Role).To(Equal("user"))
			Expect(msg.Name).To(Equal("alice"))
			Expect(msg.Content).To(Equal("Hello world"))
		})

		It("allows empty name (optional field)", func() {
			msg := llm.Message{
				Role:    "user",
				Content: "Hello world",
			}
			Expect(msg.Name).To(BeEmpty())
		})

		It("can be used with sanitized GitLab usernames", func() {
			gitlabUsername := "alice.smith@company"
			msg := llm.Message{
				Role:    "user",
				Name:    llm.SanitizeName(gitlabUsername),
				Content: "We need bulk refund support",
			}
			Expect(msg.Name).To(Equal("alice_smith_company"))
		})
	})
})
