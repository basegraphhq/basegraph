package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"basegraph.app/relay/internal/http/handler"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/service"
)

var _ = Describe("InvitationHandler", func() {
	var (
		router      *gin.Engine
		svc         *mockInvitationService
		authSvc     *mockAuthService
		adminAPIKey string
	)

	BeforeEach(func() {
		gin.SetMode(gin.TestMode)
		router = gin.New()
		svc = &mockInvitationService{}
		authSvc = &mockAuthService{}
		adminAPIKey = "test-admin-key"
		h := handler.NewInvitationHandler(svc, authSvc, adminAPIKey)

		// Public routes
		router.GET("/invites/validate", h.Validate)

		// Admin routes (with middleware)
		admin := router.Group("/admin/invites")
		admin.Use(h.RequireAdminAPIKey())
		{
			admin.POST("", h.Create)
			admin.GET("", h.List)
			admin.GET("/pending", h.ListPending)
			admin.POST("/revoke", h.Revoke)
		}
	})

	Describe("Create", func() {
		Context("with valid admin API key", func() {
			It("returns 201 with invitation details on success", func() {
				svc.createFn = func(_ context.Context, email string, _ *int64) (*model.Invitation, string, error) {
					return &model.Invitation{
						ID:        1,
						Email:     email,
						Token:     "generated-token",
						Status:    model.InvitationStatusPending,
						ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
					}, "https://basegraph.app/invite?token=generated-token", nil
				}

				body, _ := json.Marshal(map[string]string{
					"email": "test@example.com",
				})

				req := httptest.NewRequest(http.MethodPost, "/admin/invites", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Admin-API-Key", adminAPIKey)
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusCreated))

				var resp map[string]any
				Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
				Expect(resp["email"]).To(Equal("test@example.com"))
				Expect(resp["invite_url"]).To(ContainSubstring("token=generated-token"))
			})

			It("returns 409 when pending invitation exists", func() {
				svc.createFn = func(_ context.Context, _ string, _ *int64) (*model.Invitation, string, error) {
					return nil, "", service.ErrInvitePendingExists
				}

				body, _ := json.Marshal(map[string]string{
					"email": "existing@example.com",
				})

				req := httptest.NewRequest(http.MethodPost, "/admin/invites", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Admin-API-Key", adminAPIKey)
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusConflict))
			})

			It("returns 400 on invalid request body", func() {
				body, _ := json.Marshal(map[string]string{
					"email": "not-an-email",
				})

				req := httptest.NewRequest(http.MethodPost, "/admin/invites", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Admin-API-Key", adminAPIKey)
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusBadRequest))
			})
		})

		Context("without admin API key", func() {
			It("returns 401 unauthorized", func() {
				body, _ := json.Marshal(map[string]string{
					"email": "test@example.com",
				})

				req := httptest.NewRequest(http.MethodPost, "/admin/invites", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusUnauthorized))
			})
		})

		Context("with invalid admin API key", func() {
			It("returns 401 unauthorized", func() {
				body, _ := json.Marshal(map[string]string{
					"email": "test@example.com",
				})

				req := httptest.NewRequest(http.MethodPost, "/admin/invites", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Admin-API-Key", "wrong-key")
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusUnauthorized))
			})
		})
	})

	Describe("Validate", func() {
		It("returns 200 with invitation info for valid token", func() {
			svc.validateTokenFn = func(_ context.Context, token string) (*model.Invitation, error) {
				Expect(token).To(Equal("valid-token"))
				return &model.Invitation{
					ID:        1,
					Email:     "test@example.com",
					Token:     token,
					Status:    model.InvitationStatusPending,
					ExpiresAt: time.Now().Add(24 * time.Hour),
				}, nil
			}

			req := httptest.NewRequest(http.MethodGet, "/invites/validate?token=valid-token", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var resp map[string]any
			Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["email"]).To(Equal("test@example.com"))
			Expect(resp["valid"]).To(BeTrue())
		})

		It("returns 400 when token is missing", func() {
			req := httptest.NewRequest(http.MethodGet, "/invites/validate", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusBadRequest))
		})

		It("returns 404 when invitation not found", func() {
			svc.validateTokenFn = func(_ context.Context, _ string) (*model.Invitation, error) {
				return nil, service.ErrInviteNotFound
			}

			req := httptest.NewRequest(http.MethodGet, "/invites/validate?token=nonexistent", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusNotFound))

			var resp map[string]any
			Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["code"]).To(Equal("not_found"))
		})

		It("returns 410 when invitation has expired", func() {
			svc.validateTokenFn = func(_ context.Context, _ string) (*model.Invitation, error) {
				return nil, service.ErrInviteExpired
			}
			svc.getByTokenFn = func(_ context.Context, _ string) (*model.Invitation, error) {
				return &model.Invitation{Email: "expired@example.com"}, nil
			}

			req := httptest.NewRequest(http.MethodGet, "/invites/validate?token=expired", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusGone))

			var resp map[string]any
			Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["code"]).To(Equal("expired"))
		})

		It("returns 410 when invitation was already used", func() {
			svc.validateTokenFn = func(_ context.Context, _ string) (*model.Invitation, error) {
				return nil, service.ErrInviteAlreadyUsed
			}
			svc.getByTokenFn = func(_ context.Context, _ string) (*model.Invitation, error) {
				return &model.Invitation{Email: "used@example.com"}, nil
			}

			req := httptest.NewRequest(http.MethodGet, "/invites/validate?token=used", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusGone))

			var resp map[string]any
			Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["code"]).To(Equal("already_used"))
		})

		It("returns 410 when invitation was revoked", func() {
			svc.validateTokenFn = func(_ context.Context, _ string) (*model.Invitation, error) {
				return nil, service.ErrInviteRevoked
			}
			svc.getByTokenFn = func(_ context.Context, _ string) (*model.Invitation, error) {
				return &model.Invitation{Email: "revoked@example.com"}, nil
			}

			req := httptest.NewRequest(http.MethodGet, "/invites/validate?token=revoked", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusGone))

			var resp map[string]any
			Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["code"]).To(Equal("revoked"))
		})
	})

	Describe("List", func() {
		It("returns 200 with all invitations", func() {
			svc.listFn = func(_ context.Context, _, _ int32) ([]model.Invitation, error) {
				return []model.Invitation{
					{
						ID:        1,
						Email:     "a@example.com",
						Status:    model.InvitationStatusPending,
						ExpiresAt: time.Now().Add(24 * time.Hour),
						CreatedAt: time.Now(),
					},
					{
						ID:        2,
						Email:     "b@example.com",
						Status:    model.InvitationStatusAccepted,
						ExpiresAt: time.Now().Add(24 * time.Hour),
						CreatedAt: time.Now(),
					},
				}, nil
			}

			req := httptest.NewRequest(http.MethodGet, "/admin/invites", nil)
			req.Header.Set("X-Admin-API-Key", adminAPIKey)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var resp map[string]any
			Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
			invitations := resp["invitations"].([]any)
			Expect(invitations).To(HaveLen(2))
		})
	})

	Describe("ListPending", func() {
		It("returns 200 with only pending invitations", func() {
			svc.listPendingFn = func(_ context.Context) ([]model.Invitation, error) {
				return []model.Invitation{
					{
						ID:        1,
						Email:     "pending@example.com",
						Status:    model.InvitationStatusPending,
						ExpiresAt: time.Now().Add(24 * time.Hour),
						CreatedAt: time.Now(),
					},
				}, nil
			}

			req := httptest.NewRequest(http.MethodGet, "/admin/invites/pending", nil)
			req.Header.Set("X-Admin-API-Key", adminAPIKey)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var resp map[string]any
			Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
			invitations := resp["invitations"].([]any)
			Expect(invitations).To(HaveLen(1))
		})
	})

	Describe("Revoke", func() {
		It("returns 200 when invitation is revoked successfully", func() {
			svc.revokeFn = func(_ context.Context, id int64) (*model.Invitation, error) {
				Expect(id).To(Equal(int64(1)))
				return &model.Invitation{
					ID:        1,
					Email:     "test@example.com",
					Status:    model.InvitationStatusRevoked,
					ExpiresAt: time.Now().Add(24 * time.Hour),
					CreatedAt: time.Now(),
				}, nil
			}

			body, _ := json.Marshal(map[string]int64{
				"id": 1,
			})

			req := httptest.NewRequest(http.MethodPost, "/admin/invites/revoke", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Admin-API-Key", adminAPIKey)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var resp map[string]any
			Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["status"]).To(Equal("revoked"))
		})

		It("returns 404 when invitation not found", func() {
			svc.revokeFn = func(_ context.Context, _ int64) (*model.Invitation, error) {
				return nil, service.ErrInviteNotFound
			}

			body, _ := json.Marshal(map[string]int64{
				"id": 999,
			})

			req := httptest.NewRequest(http.MethodPost, "/admin/invites/revoke", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Admin-API-Key", adminAPIKey)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusNotFound))
		})
	})

	Describe("RequireAdminAPIKey middleware", func() {
		It("accepts Bearer token authorization", func() {
			svc.listFn = func(_ context.Context, _, _ int32) ([]model.Invitation, error) {
				return []model.Invitation{}, nil
			}

			req := httptest.NewRequest(http.MethodGet, "/admin/invites", nil)
			req.Header.Set("Authorization", "Bearer "+adminAPIKey)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
		})
	})
})
