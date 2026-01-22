package service_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"basegraph.co/relay/common/id"
	"basegraph.co/relay/internal/model"
	"basegraph.co/relay/internal/service"
	"basegraph.co/relay/internal/store"
)

var _ = Describe("OrganizationService", func() {
	var (
		svc      service.OrganizationService
		mockOrg  *mockOrganizationStore
		mockWork *mockWorkspaceStore
		ctx      context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockOrg = &mockOrganizationStore{}
		mockWork = &mockWorkspaceStore{}
		svc = service.NewOrganizationService(&mockTxRunner{
			withTxFn: func(ctx context.Context, fn func(stores service.StoreProvider) error) error {
				return fn(&mockStoreProvider{org: mockOrg, work: mockWork})
			},
		})
		Expect(id.Init(1)).To(Succeed())
	})

	It("creates organization with provided slug", func() {
		mockOrg.getBySlugFn = func(_ context.Context, slug string) (*model.Organization, error) {
			Expect(slug).To(Equal("custom-slug"))
			return nil, store.ErrNotFound
		}
		mockWork.getByOrgAndSlugFn = func(_ context.Context, orgID int64, slug string) (*model.Workspace, error) {
			Expect(slug).To(Equal("acme"))
			Expect(orgID).NotTo(BeZero())
			return nil, store.ErrNotFound
		}
		mockOrg.createFn = func(_ context.Context, org *model.Organization) error {
			Expect(org.Slug).To(Equal("custom-slug"))
			return nil
		}
		mockWork.createFn = func(_ context.Context, ws *model.Workspace) error {
			Expect(ws.Slug).To(Equal("acme"))
			Expect(ws.Name).To(Equal("Acme workspace"))
			Expect(ws.OrganizationID).NotTo(BeZero())
			return nil
		}

		org, err := svc.Create(ctx, "Acme", strPtr("custom-slug"), 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(org.Slug).To(Equal("custom-slug"))
		Expect(org.AdminUserID).To(Equal(int64(10)))
		Expect(mockOrg.createCalls).To(Equal(1))
		Expect(mockWork.createCalls).To(Equal(1))
	})

	It("generates slug from name when missing", func() {
		mockOrg.getBySlugFn = func(_ context.Context, slug string) (*model.Organization, error) {
			Expect(slug).To(Equal("acme-corp"))
			return nil, store.ErrNotFound
		}
		mockWork.getByOrgAndSlugFn = func(_ context.Context, _ int64, slug string) (*model.Workspace, error) {
			Expect(slug).To(Equal("acme-corp"))
			return nil, store.ErrNotFound
		}
		mockOrg.createFn = func(_ context.Context, org *model.Organization) error {
			Expect(org.Slug).To(Equal("acme-corp"))
			return nil
		}
		mockWork.createFn = func(_ context.Context, ws *model.Workspace) error {
			Expect(ws.Slug).To(Equal("acme-corp"))
			Expect(ws.Name).To(Equal("Acme Corp workspace"))
			return nil
		}

		org, err := svc.Create(ctx, "Acme Corp", nil, 20)
		Expect(err).NotTo(HaveOccurred())
		Expect(org.Slug).To(Equal("acme-corp"))
		Expect(mockOrg.createCalls).To(Equal(1))
		Expect(mockWork.createCalls).To(Equal(1))
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
		mockWork.getByOrgAndSlugFn = func(_ context.Context, _ int64, slug string) (*model.Workspace, error) {
			// base available
			Expect(slug == "acme" || slug == "acme-1").To(BeTrue())
			if slug == "acme" {
				return nil, store.ErrNotFound
			}
			return nil, store.ErrNotFound
		}
		mockOrg.createFn = func(_ context.Context, org *model.Organization) error {
			Expect(org.Slug).To(Equal("acme-1"))
			return nil
		}
		mockWork.createFn = func(_ context.Context, ws *model.Workspace) error {
			Expect(ws.Slug).To(Equal("acme"))
			return nil
		}

		org, err := svc.Create(ctx, "Acme", nil, 30)
		Expect(err).NotTo(HaveOccurred())
		Expect(org.Slug).To(Equal("acme-1"))
		Expect(mockOrg.createCalls).To(Equal(1))
		Expect(mockWork.createCalls).To(Equal(1))
	})

	It("adds suffix when workspace slug collides within org", func() {
		mockOrg.getBySlugFn = func(_ context.Context, slug string) (*model.Organization, error) {
			Expect(slug).To(Equal("acme"))
			return nil, store.ErrNotFound
		}
		call := 0
		mockWork.getByOrgAndSlugFn = func(_ context.Context, _ int64, slug string) (*model.Workspace, error) {
			if call == 0 {
				Expect(slug).To(Equal("acme"))
				call++
				return &model.Workspace{}, nil // taken
			}
			Expect(slug).To(Equal("acme-1"))
			return nil, store.ErrNotFound
		}
		mockOrg.createFn = func(_ context.Context, org *model.Organization) error {
			return nil
		}
		mockWork.createFn = func(_ context.Context, ws *model.Workspace) error {
			Expect(ws.Slug).To(Equal("acme-1"))
			return nil
		}

		_, err := svc.Create(ctx, "Acme", nil, 50)
		Expect(err).NotTo(HaveOccurred())
		Expect(mockOrg.createCalls).To(Equal(1))
		Expect(mockWork.createCalls).To(Equal(1))
	})

	It("returns error when slug checks fail", func() {
		mockOrg.getBySlugFn = func(_ context.Context, _ string) (*model.Organization, error) {
			return nil, errors.New("db error")
		}

		_, err := svc.Create(ctx, "Acme", nil, 40)
		Expect(err).To(HaveOccurred())
		Expect(mockOrg.createCalls).To(Equal(0))
		Expect(mockWork.createCalls).To(Equal(0))
	})

	It("fails when workspace create fails and does not commit org", func() {
		mockOrg.getBySlugFn = func(_ context.Context, slug string) (*model.Organization, error) {
			Expect(slug).To(Equal("acme"))
			return nil, store.ErrNotFound
		}
		mockWork.getByOrgAndSlugFn = func(_ context.Context, _ int64, slug string) (*model.Workspace, error) {
			Expect(slug).To(Equal("acme"))
			return nil, store.ErrNotFound
		}
		mockOrg.createFn = func(_ context.Context, org *model.Organization) error {
			return nil
		}
		mockWork.createFn = func(_ context.Context, ws *model.Workspace) error {
			return errors.New("workspace create failed")
		}

		_, err := svc.Create(ctx, "Acme", nil, 60)
		Expect(err).To(HaveOccurred())
		Expect(mockWork.createCalls).To(Equal(1))
		Expect(mockOrg.createCalls).To(Equal(1))
	})

	It("propagates tx runner error", func() {
		mockTxErr := errors.New("tx failed")
		svc = service.NewOrganizationService(&mockTxRunner{
			withTxFn: func(_ context.Context, _ func(stores service.StoreProvider) error) error {
				return mockTxErr
			},
		})

		_, err := svc.Create(ctx, "Acme", nil, 70)
		Expect(err).To(MatchError(mockTxErr))
	})
})

func strPtr(s string) *string { return &s }
