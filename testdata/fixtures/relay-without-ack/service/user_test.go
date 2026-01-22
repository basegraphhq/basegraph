package service_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"basegraph.co/relay/common/id"
	"basegraph.co/relay/internal/model"
	"basegraph.co/relay/internal/service"
)

var _ = Describe("UserService", func() {
	var (
		svc       service.UserService
		mockStore *mockUserStore
		mockOrg   *mockOrganizationStore
		ctx       context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockStore = &mockUserStore{}
		mockOrg = &mockOrganizationStore{}

		err := id.Init(1)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Create", func() {
		Context("when user data is valid", func() {
			It("should create user with generated snowflake ID", func() {
				var capturedUser *model.User
				mockStore.createFn = func(_ context.Context, u *model.User) error {
					capturedUser = u
					return nil
				}

				svc = service.NewUserService(mockStore, mockOrg)
				user, err := svc.Create(ctx, "Test User", "test@example.com", nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(user).NotTo(BeNil())
				Expect(user.ID).NotTo(BeZero())
				Expect(user.Name).To(Equal("Test User"))
				Expect(user.Email).To(Equal("test@example.com"))
				Expect(user.AvatarURL).To(BeNil())

				Expect(capturedUser).NotTo(BeNil())
				Expect(capturedUser.ID).To(Equal(user.ID))
			})

			It("should include avatar URL when provided", func() {
				mockStore.createFn = func(_ context.Context, _ *model.User) error {
					return nil
				}

				avatarURL := "https://example.com/avatar.png"
				svc = service.NewUserService(mockStore, mockOrg)
				user, err := svc.Create(ctx, "Test User", "test@example.com", &avatarURL)

				Expect(err).NotTo(HaveOccurred())
				Expect(user.AvatarURL).NotTo(BeNil())
				Expect(*user.AvatarURL).To(Equal(avatarURL))
			})
		})

		Context("when store returns an error", func() {
			It("should propagate the error", func() {
				mockStore.createFn = func(_ context.Context, _ *model.User) error {
					return errors.New("database connection failed")
				}

				svc = service.NewUserService(mockStore, mockOrg)
				user, err := svc.Create(ctx, "Test User", "test@example.com", nil)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("database connection failed"))
				Expect(user).To(BeNil())
			})
		})
	})

	Describe("Sync", func() {
		It("upserts user and returns organizations", func() {
			mockStore.upsertFn = func(_ context.Context, u *model.User) error {
				u.ID = 42
				return nil
			}
			mockOrg.listByAdminUserFn = func(_ context.Context, userID int64) ([]model.Organization, error) {
				Expect(userID).To(Equal(int64(42)))
				return []model.Organization{
					{ID: 100, Name: "Acme", Slug: "acme"},
				}, nil
			}

			svc = service.NewUserService(mockStore, mockOrg)
			user, orgs, err := svc.Sync(ctx, "Sync User", "sync@example.com", nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(user.ID).To(Equal(int64(42)))
			Expect(len(orgs)).To(Equal(1))
			Expect(orgs[0].Slug).To(Equal("acme"))
		})

		It("propagates upsert errors", func() {
			mockStore.upsertFn = func(_ context.Context, _ *model.User) error {
				return errors.New("upsert failure")
			}

			svc = service.NewUserService(mockStore, mockOrg)
			user, orgs, err := svc.Sync(ctx, "Sync User", "sync@example.com", nil)

			Expect(err).To(HaveOccurred())
			Expect(user).To(BeNil())
			Expect(orgs).To(BeNil())
		})

		It("propagates organization listing errors", func() {
			mockStore.upsertFn = func(_ context.Context, _ *model.User) error {
				return nil
			}
			mockOrg.listByAdminUserFn = func(_ context.Context, _ int64) ([]model.Organization, error) {
				return nil, errors.New("list failure")
			}

			svc = service.NewUserService(mockStore, mockOrg)
			_, _, err := svc.Sync(ctx, "Sync User", "sync@example.com", nil)

			Expect(err).To(HaveOccurred())
		})
	})
})
