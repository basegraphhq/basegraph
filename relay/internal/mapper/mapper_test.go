package mapper_test

import (
	"context"
	"errors"

	"basegraph.co/relay/internal/mapper"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Mapper", func() {
	Describe("CanonicalEventType", func() {
		It("has the correct string values", func() {
			Expect(string(mapper.EventIssueCreated)).To(Equal("issue_created"))
			Expect(string(mapper.EventIssueClosed)).To(Equal("issue_closed"))
			Expect(string(mapper.EventReply)).To(Equal("reply"))
			Expect(string(mapper.EventPRCreated)).To(Equal("pull_request_created"))
		})
	})

	Describe("EventMapper Interface", func() {
		When("creating a stub mapper", func() {
			It("should implement the EventMapper interface", func() {
				// Create a stub mapper using a type assertion
				var stubMapper mapper.EventMapper = &stubMapperImpl{}
				Expect(stubMapper).ToNot(BeNil())
			})
		})
	})
})

// stubMapperImpl is a test implementation that always returns an error
type stubMapperImpl struct{}

func (s *stubMapperImpl) Map(ctx context.Context, body map[string]any, headers map[string]string) (mapper.CanonicalEventType, error) {
	return "", errors.New("not implemented")
}
