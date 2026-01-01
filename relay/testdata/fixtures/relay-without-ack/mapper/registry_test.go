package mapper_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"basegraph.app/relay/internal/mapper"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MapperRegistry", func() {
	var (
		registry *mapper.MapperRegistry
		ctx      context.Context
	)

	BeforeEach(func() {
		// Create a new registry instance for each test
		registry = mapper.NewMapperRegistry()
		ctx = context.Background()
	})

	Describe("Register", func() {
		It("should register a new mapper", func() {
			testMapper := &stubMapperImpl{}

			registry.Register("test", testMapper)

			retrievedMapper, err := registry.Get("test")
			Expect(err).ToNot(HaveOccurred())
			Expect(retrievedMapper).To(Equal(testMapper))
		})

		It("should overwrite an existing mapper", func() {
			firstMapper := &stubMapperImpl{}
			secondMapper := &stubMapperImpl{}

			registry.Register("test", firstMapper)
			registry.Register("test", secondMapper)

			retrievedMapper, err := registry.Get("test")
			Expect(err).ToNot(HaveOccurred())
			Expect(retrievedMapper).To(Equal(secondMapper))
		})
	})

	Describe("Get", func() {
		It("should return registered mapper", func() {
			testMapper := &stubMapperImpl{}
			registry.Register("github", testMapper)

			retrievedMapper, err := registry.Get("github")
			Expect(err).ToNot(HaveOccurred())
			Expect(retrievedMapper).To(Equal(testMapper))
		})

		It("should return error for unregistered provider", func() {
			_, err := registry.Get("nonexistent")
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("unsupported provider")))
			Expect(err).To(MatchError(ContainSubstring("nonexistent")))
		})
	})

	Describe("MustGet", func() {
		It("should return registered mapper", func() {
			testMapper := &stubMapperImpl{}
			registry.Register("linear", testMapper)

			retrievedMapper := registry.MustGet("linear")
			Expect(retrievedMapper).To(Equal(testMapper))
		})

		It("should panic for unregistered provider", func() {
			Expect(func() {
				registry.MustGet("nonexistent")
			}).To(Panic())
		})
	})

	Describe("Concurrency", func() {
		It("should handle concurrent reads and writes safely", func() {
			testMapper := &stubMapperImpl{}

			// Register a mapper
			registry.Register("concurrent", testMapper)

			var wg sync.WaitGroup
			numReaders := 10
			numWriters := 5

			// Start multiple goroutines reading from the registry
			for i := 0; i < numReaders; i++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					for j := 0; j < 100; j++ {
						m, err := registry.Get("concurrent")
						Expect(err).ToNot(HaveOccurred())
						Expect(m).To(Equal(testMapper))

						// Add small delay to increase chance of race conditions
						time.Sleep(time.Microsecond)
					}
				}(i)
			}

			// Start multiple goroutines writing to the registry
			for i := 0; i < numWriters; i++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					for j := 0; j < 20; j++ {
						newMapper := &stubMapperImpl{}
						providerName := fmt.Sprintf("provider_%d_%d", id, j)
						registry.Register(providerName, newMapper)

						// Immediately try to retrieve it
						m, err := registry.Get(providerName)
						Expect(err).ToNot(HaveOccurred())
						Expect(m).To(Equal(newMapper))

						time.Sleep(time.Microsecond)
					}
				}(i)
			}

			wg.Wait()
		})
	})

	Describe("Integration with existing mappers", func() {
		It("should work with the default GitLab mapper", func() {
			// Create a new registry which should have GitLab registered by default
			globalRegistry := mapper.NewMapperRegistry()

			gitlabMapper, err := globalRegistry.Get("gitlab")
			Expect(err).ToNot(HaveOccurred())
			Expect(gitlabMapper).ToNot(BeNil())

			// Test that it can map events
			headers := map[string]string{
				"X-Gitlab-Event": "Issue Hook",
			}
			body := map[string]any{
				"object_kind": "issue",
			}

			eventType, err := gitlabMapper.Map(ctx, body, headers)
			Expect(err).ToNot(HaveOccurred())
			Expect(eventType).To(Equal(mapper.EventIssueCreated))
		})
	})

	When("using the registry with different mapper implementations", func() {
		It("should handle GitHub mapper (stub)", func() {
			// Register a stub GitHub mapper
			githubMapper := &stubMapperImpl{}
			registry.Register("github", githubMapper)

			retrievedMapper, err := registry.Get("github")
			Expect(err).ToNot(HaveOccurred())

			_, err = retrievedMapper.Map(ctx, map[string]any{}, map[string]string{})
			Expect(err).To(MatchError("not implemented"))
		})

		It("should handle Linear mapper (stub)", func() {
			// Register a stub Linear mapper
			linearMapper := &stubMapperImpl{}
			registry.Register("linear", linearMapper)

			retrievedMapper := registry.MustGet("linear")
			Expect(retrievedMapper).To(Equal(linearMapper))

			_, err := retrievedMapper.Map(ctx, map[string]any{}, map[string]string{})
			Expect(err).To(MatchError("not implemented"))
		})
	})

	Describe("Error handling", func() {
		It("should handle empty provider names", func() {
			testMapper := &stubMapperImpl{}
			registry.Register("", testMapper)

			retrievedMapper, err := registry.Get("")
			Expect(err).ToNot(HaveOccurred())
			Expect(retrievedMapper).To(Equal(testMapper))
		})

		It("should handle nil mapper registration", func() {
			// This should not panic, though it's not recommended
			Expect(func() {
				registry.Register("nil", nil)
			}).ToNot(Panic())

			retrievedMapper, err := registry.Get("nil")
			Expect(err).ToNot(HaveOccurred())
			Expect(retrievedMapper).To(BeNil())
		})
	})
})
