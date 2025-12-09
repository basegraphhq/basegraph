package service_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"basegraph.app/relay/common/id"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/service"
	"basegraph.app/relay/internal/store"
)

var _ = Describe("OrganizationService", func() {
	var (
		svc     service.OrganizationService
		mockOrg *mockOrganizationStore
		ctx     context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockOrg = &mockOrganizationStore{}
		svc = service.NewOrganizationService(mockOrg)
		Expect(id.Init(1)).To(Succeed())
	})

	It("creates organization with provided slug", func() {
		mockOrg.getBySlugFn = func(_ context.Context, slug string) (*model.Organization, error) {
			Expect(slug).To(Equal("custom-slug"))
			return nil, store.ErrNotFound
		}
		mockOrg.createFn = func(_ context.Context, org *model.Organization) error {
			Expect(org.Slug).To(Equal("custom-slug"))
			return nil
		}

		org, err := svc.Create(ctx, "Acme", strPtr("custom-slug"), 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(org.Slug).To(Equal("custom-slug"))
		Expect(org.AdminUserID).To(Equal(int64(10)))
	})

	It("generates slug from name when missing", func() {
		mockOrg.getBySlugFn = func(_ context.Context, slug string) (*model.Organization, error) {
			Expect(slug).To(Equal("acme-corp"))
			return nil, store.ErrNotFound
		}
		mockOrg.createFn = func(_ context.Context, org *model.Organization) error {
			Expect(org.Slug).To(Equal("acme-corp"))
			return nil
		}

		org, err := svc.Create(ctx, "Acme Corp", nil, 20)
		Expect(err).NotTo(HaveOccurred())
		Expect(org.Slug).To(Equal("acme-corp"))
	})

	It("adds numeric suffix when slug is taken", func() {
		call := 0
		mockOrg.getBySlugFn = func(_ context.Context, slug string) (*model.Organization, error) {
			if call == 0 && slug == "acme" {
				call++
				return &model.Organization{}, nil // taken
			}
			// first suffix attempt should be available
			return nil, store.ErrNotFound
		}
		mockOrg.createFn = func(_ context.Context, org *model.Organization) error {
			Expect(org.Slug).To(Equal("acme-1"))
			return nil
		}

		org, err := svc.Create(ctx, "Acme", nil, 30)
		Expect(err).NotTo(HaveOccurred())
		Expect(org.Slug).To(Equal("acme-1"))
	})

	It("returns error when slug checks fail", func() {
		mockOrg.getBySlugFn = func(_ context.Context, _ string) (*model.Organization, error) {
			return nil, errors.New("db error")
		}

		_, err := svc.Create(ctx, "Acme", nil, 40)
		Expect(err).To(HaveOccurred())
	})
})

func strPtr(s string) *string { return &s }
