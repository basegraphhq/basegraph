package brain_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"basegraph.app/relay/internal/brain"
	"basegraph.app/relay/internal/model"
)

var _ = Describe("StubCodeGraphRetriever", func() {
	var (
		retriever *brain.StubCodeGraphRetriever
		ctx       context.Context
		issue     *model.Issue
	)

	BeforeEach(func() {
		retriever = brain.NewStubCodeGraphRetriever()
		ctx = context.Background()
		issue = &model.Issue{ID: 123}
	})

	Describe("Query with SymbolHints", func() {
		It("returns curated finding for known symbol", func() {
			job := brain.RetrieverJob{
				Query:       "find user service",
				SymbolHints: []string{"UserService"},
			}

			findings, err := retriever.Query(ctx, issue, job)

			Expect(err).NotTo(HaveOccurred())
			Expect(findings).To(HaveLen(1))
			Expect(findings[0].Observation).To(ContainSubstring("UserService"))
			Expect(findings[0].Observation).To(ContainSubstring("lifecycle operations"))
			Expect(findings[0].Sources).To(HaveLen(1))
			Expect(findings[0].Sources[0].Location).To(Equal("internal/service/user_service.go:24"))
			Expect(findings[0].Sources[0].Snippet).To(ContainSubstring("type UserService struct"))
			Expect(findings[0].Confidence).To(Equal(0.92))
		})

		It("generates synthetic finding for unknown symbol", func() {
			job := brain.RetrieverJob{
				Query:       "find custom handler",
				SymbolHints: []string{"CustomHandler"},
			}

			findings, err := retriever.Query(ctx, issue, job)

			Expect(err).NotTo(HaveOccurred())
			Expect(findings).To(HaveLen(1))
			Expect(findings[0].Observation).To(ContainSubstring("CustomHandler"))
			Expect(findings[0].Sources[0].Location).To(ContainSubstring("custom_handler.go"))
			Expect(findings[0].Sources[0].Snippet).To(ContainSubstring("func CustomHandler"))
		})

		It("returns multiple findings for multiple symbols", func() {
			job := brain.RetrieverJob{
				Query:       "find payment code",
				SymbolHints: []string{"UserService", "PaymentProcessor"},
			}

			findings, err := retriever.Query(ctx, issue, job)

			Expect(err).NotTo(HaveOccurred())
			Expect(findings).To(HaveLen(2))
		})
	})

	Describe("Query without SymbolHints", func() {
		It("returns curated findings for matching query", func() {
			job := brain.RetrieverJob{
				Query:  "authentication flow and login",
				Intent: "understand auth",
			}

			findings, err := retriever.Query(ctx, issue, job)

			Expect(err).NotTo(HaveOccurred())
			Expect(findings).To(HaveLen(2))
			Expect(findings[0].Observation).To(ContainSubstring("AuthService"))
		})

		It("returns curated findings for payment query", func() {
			job := brain.RetrieverJob{
				Query:  "how does payment processing work",
				Intent: "understand billing",
			}

			findings, err := retriever.Query(ctx, issue, job)

			Expect(err).NotTo(HaveOccurred())
			Expect(findings).To(HaveLen(2))
			Expect(findings[0].Observation).To(ContainSubstring("BillingService"))
		})

		It("returns fallback findings for unmatched query", func() {
			job := brain.RetrieverJob{
				Query:  "something completely random",
				Intent: "unknown",
			}

			findings, err := retriever.Query(ctx, issue, job)

			Expect(err).NotTo(HaveOccurred())
			Expect(findings).To(HaveLen(2))
			Expect(findings[0].Observation).To(ContainSubstring("matched 4 functions"))
		})
	})
})
