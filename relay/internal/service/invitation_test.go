package service_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"basegraph.app/relay/common/id"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/service"
	"basegraph.app/relay/internal/store"
)

var _ = Describe("InvitationService", func() {
	var (
		svc          service.InvitationService
		mockStore    *mockInvitationStore
		ctx          context.Context
		dashboardURL string
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockStore = &mockInvitationStore{}
		dashboardURL = "https://basegraph.app"

		err := id.Init(1)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Create", func() {
		Context("when email is valid and no pending invitation exists", func() {
			It("should create invitation with generated token", func() {
				var capturedInv *model.Invitation
				mockStore.getByEmailFn = func(_ context.Context, _ string) (*model.Invitation, error) {
					return nil, store.ErrNotFound
				}
				mockStore.createFn = func(_ context.Context, inv *model.Invitation) error {
					capturedInv = inv
					return nil
				}

				svc = service.NewInvitationService(mockStore, dashboardURL)
				inv, inviteURL, err := svc.Create(ctx, "test@example.com", nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(inv).NotTo(BeNil())
				Expect(inv.ID).NotTo(BeZero())
				Expect(inv.Email).To(Equal("test@example.com"))
				Expect(inv.Token).NotTo(BeEmpty())
				Expect(inv.Status).To(Equal(model.InvitationStatusPending))
				Expect(inviteURL).To(ContainSubstring(dashboardURL + "/invite?token="))
				Expect(capturedInv).NotTo(BeNil())
			})

			It("should normalize email to lowercase", func() {
				mockStore.getByEmailFn = func(_ context.Context, email string) (*model.Invitation, error) {
					Expect(email).To(Equal("test@example.com"))
					return nil, store.ErrNotFound
				}
				mockStore.createFn = func(_ context.Context, inv *model.Invitation) error {
					Expect(inv.Email).To(Equal("test@example.com"))
					return nil
				}

				svc = service.NewInvitationService(mockStore, dashboardURL)
				inv, _, err := svc.Create(ctx, "  TEST@EXAMPLE.COM  ", nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(inv.Email).To(Equal("test@example.com"))
			})

			It("should set expiry to 7 days in the future", func() {
				mockStore.getByEmailFn = func(_ context.Context, _ string) (*model.Invitation, error) {
					return nil, store.ErrNotFound
				}
				mockStore.createFn = func(_ context.Context, inv *model.Invitation) error {
					// Verify expiry is approximately 7 days from now
					expectedExpiry := time.Now().Add(7 * 24 * time.Hour)
					Expect(inv.ExpiresAt).To(BeTemporally("~", expectedExpiry, time.Minute))
					return nil
				}

				svc = service.NewInvitationService(mockStore, dashboardURL)
				_, _, err := svc.Create(ctx, "test@example.com", nil)

				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when a pending invitation already exists for the email", func() {
			It("should return ErrInvitePendingExists", func() {
				mockStore.getByEmailFn = func(_ context.Context, _ string) (*model.Invitation, error) {
					return &model.Invitation{
						ID:        1,
						Email:     "test@example.com",
						Status:    model.InvitationStatusPending,
						ExpiresAt: time.Now().Add(24 * time.Hour),
					}, nil
				}

				svc = service.NewInvitationService(mockStore, dashboardURL)
				inv, inviteURL, err := svc.Create(ctx, "test@example.com", nil)

				Expect(err).To(MatchError(service.ErrInvitePendingExists))
				Expect(inv).To(BeNil())
				Expect(inviteURL).To(BeEmpty())
			})
		})

		Context("when store returns an error on create", func() {
			It("should propagate the error", func() {
				mockStore.getByEmailFn = func(_ context.Context, _ string) (*model.Invitation, error) {
					return nil, store.ErrNotFound
				}
				mockStore.createFn = func(_ context.Context, _ *model.Invitation) error {
					return store.ErrNotFound
				}

				svc = service.NewInvitationService(mockStore, dashboardURL)
				_, _, err := svc.Create(ctx, "test@example.com", nil)

				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("ValidateToken", func() {
		Context("when token is valid", func() {
			It("should return the invitation", func() {
				expectedInv := &model.Invitation{
					ID:        1,
					Email:     "test@example.com",
					Token:     "valid-token",
					Status:    model.InvitationStatusPending,
					ExpiresAt: time.Now().Add(24 * time.Hour),
				}
				mockStore.getValidByTokenFn = func(_ context.Context, token string) (*model.Invitation, error) {
					Expect(token).To(Equal("valid-token"))
					return expectedInv, nil
				}

				svc = service.NewInvitationService(mockStore, dashboardURL)
				inv, err := svc.ValidateToken(ctx, "valid-token")

				Expect(err).NotTo(HaveOccurred())
				Expect(inv).To(Equal(expectedInv))
			})
		})

		Context("when token is not found", func() {
			It("should return ErrInviteNotFound", func() {
				mockStore.getValidByTokenFn = func(_ context.Context, _ string) (*model.Invitation, error) {
					return nil, store.ErrNotFound
				}
				mockStore.getByTokenFn = func(_ context.Context, _ string) (*model.Invitation, error) {
					return nil, store.ErrNotFound
				}

				svc = service.NewInvitationService(mockStore, dashboardURL)
				inv, err := svc.ValidateToken(ctx, "invalid-token")

				Expect(err).To(MatchError(service.ErrInviteNotFound))
				Expect(inv).To(BeNil())
			})
		})

		Context("when invitation has expired", func() {
			It("should return ErrInviteExpired", func() {
				mockStore.getValidByTokenFn = func(_ context.Context, _ string) (*model.Invitation, error) {
					return nil, store.ErrNotFound
				}
				mockStore.getByTokenFn = func(_ context.Context, _ string) (*model.Invitation, error) {
					return &model.Invitation{
						ID:        1,
						Status:    model.InvitationStatusPending,
						ExpiresAt: time.Now().Add(-24 * time.Hour), // expired
					}, nil
				}

				svc = service.NewInvitationService(mockStore, dashboardURL)
				inv, err := svc.ValidateToken(ctx, "expired-token")

				Expect(err).To(MatchError(service.ErrInviteExpired))
				Expect(inv).To(BeNil())
			})
		})

		Context("when invitation was already accepted", func() {
			It("should return ErrInviteAlreadyUsed", func() {
				mockStore.getValidByTokenFn = func(_ context.Context, _ string) (*model.Invitation, error) {
					return nil, store.ErrNotFound
				}
				mockStore.getByTokenFn = func(_ context.Context, _ string) (*model.Invitation, error) {
					return &model.Invitation{
						ID:     1,
						Status: model.InvitationStatusAccepted,
					}, nil
				}

				svc = service.NewInvitationService(mockStore, dashboardURL)
				inv, err := svc.ValidateToken(ctx, "used-token")

				Expect(err).To(MatchError(service.ErrInviteAlreadyUsed))
				Expect(inv).To(BeNil())
			})
		})

		Context("when invitation was revoked", func() {
			It("should return ErrInviteRevoked", func() {
				mockStore.getValidByTokenFn = func(_ context.Context, _ string) (*model.Invitation, error) {
					return nil, store.ErrNotFound
				}
				mockStore.getByTokenFn = func(_ context.Context, _ string) (*model.Invitation, error) {
					return &model.Invitation{
						ID:     1,
						Status: model.InvitationStatusRevoked,
					}, nil
				}

				svc = service.NewInvitationService(mockStore, dashboardURL)
				inv, err := svc.ValidateToken(ctx, "revoked-token")

				Expect(err).To(MatchError(service.ErrInviteRevoked))
				Expect(inv).To(BeNil())
			})
		})
	})

	Describe("Accept", func() {
		Context("when token is valid and email matches", func() {
			It("should accept the invitation", func() {
				pendingInv := &model.Invitation{
					ID:        1,
					Email:     "test@example.com",
					Token:     "valid-token",
					Status:    model.InvitationStatusPending,
					ExpiresAt: time.Now().Add(24 * time.Hour),
				}
				acceptedInv := &model.Invitation{
					ID:        1,
					Email:     "test@example.com",
					Token:     "valid-token",
					Status:    model.InvitationStatusAccepted,
					ExpiresAt: time.Now().Add(24 * time.Hour),
				}
				user := &model.User{
					ID:    42,
					Email: "test@example.com",
				}

				mockStore.getValidByTokenFn = func(_ context.Context, _ string) (*model.Invitation, error) {
					return pendingInv, nil
				}
				mockStore.acceptFn = func(_ context.Context, invID int64, userID int64) (*model.Invitation, error) {
					Expect(invID).To(Equal(int64(1)))
					Expect(userID).To(Equal(int64(42)))
					return acceptedInv, nil
				}

				svc = service.NewInvitationService(mockStore, dashboardURL)
				inv, err := svc.Accept(ctx, "valid-token", user)

				Expect(err).NotTo(HaveOccurred())
				Expect(inv.Status).To(Equal(model.InvitationStatusAccepted))
			})

			It("should handle case-insensitive email matching", func() {
				pendingInv := &model.Invitation{
					ID:        1,
					Email:     "test@example.com",
					Token:     "valid-token",
					Status:    model.InvitationStatusPending,
					ExpiresAt: time.Now().Add(24 * time.Hour),
				}
				user := &model.User{
					ID:    42,
					Email: "TEST@EXAMPLE.COM",
				}

				mockStore.getValidByTokenFn = func(_ context.Context, _ string) (*model.Invitation, error) {
					return pendingInv, nil
				}
				mockStore.acceptFn = func(_ context.Context, _ int64, _ int64) (*model.Invitation, error) {
					return pendingInv, nil
				}

				svc = service.NewInvitationService(mockStore, dashboardURL)
				_, err := svc.Accept(ctx, "valid-token", user)

				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when email does not match", func() {
			It("should return ErrEmailMismatch", func() {
				pendingInv := &model.Invitation{
					ID:        1,
					Email:     "invited@example.com",
					Token:     "valid-token",
					Status:    model.InvitationStatusPending,
					ExpiresAt: time.Now().Add(24 * time.Hour),
				}
				user := &model.User{
					ID:    42,
					Email: "different@example.com",
				}

				mockStore.getValidByTokenFn = func(_ context.Context, _ string) (*model.Invitation, error) {
					return pendingInv, nil
				}

				svc = service.NewInvitationService(mockStore, dashboardURL)
				inv, err := svc.Accept(ctx, "valid-token", user)

				Expect(err).To(MatchError(service.ErrEmailMismatch))
				Expect(inv).To(BeNil())
			})
		})

		Context("when token is invalid", func() {
			It("should return validation error", func() {
				user := &model.User{
					ID:    42,
					Email: "test@example.com",
				}

				mockStore.getValidByTokenFn = func(_ context.Context, _ string) (*model.Invitation, error) {
					return nil, store.ErrNotFound
				}
				mockStore.getByTokenFn = func(_ context.Context, _ string) (*model.Invitation, error) {
					return nil, store.ErrNotFound
				}

				svc = service.NewInvitationService(mockStore, dashboardURL)
				_, err := svc.Accept(ctx, "invalid-token", user)

				Expect(err).To(MatchError(service.ErrInviteNotFound))
			})
		})
	})

	Describe("Revoke", func() {
		Context("when invitation exists and is pending", func() {
			It("should revoke the invitation", func() {
				revokedInv := &model.Invitation{
					ID:     1,
					Email:  "test@example.com",
					Status: model.InvitationStatusRevoked,
				}
				mockStore.revokeFn = func(_ context.Context, invID int64) (*model.Invitation, error) {
					Expect(invID).To(Equal(int64(1)))
					return revokedInv, nil
				}

				svc = service.NewInvitationService(mockStore, dashboardURL)
				inv, err := svc.Revoke(ctx, 1)

				Expect(err).NotTo(HaveOccurred())
				Expect(inv.Status).To(Equal(model.InvitationStatusRevoked))
			})
		})

		Context("when invitation does not exist", func() {
			It("should return ErrInviteNotFound", func() {
				mockStore.revokeFn = func(_ context.Context, _ int64) (*model.Invitation, error) {
					return nil, store.ErrNotFound
				}

				svc = service.NewInvitationService(mockStore, dashboardURL)
				inv, err := svc.Revoke(ctx, 999)

				Expect(err).To(MatchError(service.ErrInviteNotFound))
				Expect(inv).To(BeNil())
			})
		})
	})

	Describe("List", func() {
		It("should return all invitations with pagination", func() {
			expectedInvitations := []model.Invitation{
				{ID: 1, Email: "a@example.com"},
				{ID: 2, Email: "b@example.com"},
			}
			mockStore.listFn = func(_ context.Context, limit, offset int32) ([]model.Invitation, error) {
				Expect(limit).To(Equal(int32(10)))
				Expect(offset).To(Equal(int32(0)))
				return expectedInvitations, nil
			}

			svc = service.NewInvitationService(mockStore, dashboardURL)
			invitations, err := svc.List(ctx, 10, 0)

			Expect(err).NotTo(HaveOccurred())
			Expect(invitations).To(Equal(expectedInvitations))
		})
	})

	Describe("ListPending", func() {
		It("should return only pending invitations", func() {
			expectedInvitations := []model.Invitation{
				{ID: 1, Email: "pending@example.com", Status: model.InvitationStatusPending},
			}
			mockStore.listPendingFn = func(_ context.Context) ([]model.Invitation, error) {
				return expectedInvitations, nil
			}

			svc = service.NewInvitationService(mockStore, dashboardURL)
			invitations, err := svc.ListPending(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(invitations).To(HaveLen(1))
			Expect(invitations[0].Status).To(Equal(model.InvitationStatusPending))
		})
	})
})
