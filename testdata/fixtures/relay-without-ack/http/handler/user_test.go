package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"basegraph.app/relay/internal/http/handler"
	"basegraph.app/relay/internal/model"
)

var _ = Describe("UserHandler", func() {
	var (
		router *gin.Engine
		svc    *mockUserService
	)

	BeforeEach(func() {
		gin.SetMode(gin.TestMode)
		router = gin.New()
		svc = &mockUserService{}
		h := handler.NewUserHandler(svc)
		router.POST("/sync", h.Sync)
	})

	It("returns 200 with user and organizations on success", func() {
		svc.syncFn = func(_ context.Context, _, email string, _ *string) (*model.User, []model.Organization, error) {
			return &model.User{ID: 1, Name: "John", Email: email}, []model.Organization{
				{ID: 10, Name: "Acme", Slug: "acme"},
			}, nil
		}

		body, _ := json.Marshal(map[string]string{
			"name":  "John",
			"email": "john@example.com",
		})

		req := httptest.NewRequest(http.MethodPost, "/sync", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusOK))
		var resp map[string]any
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp["has_organization"]).To(BeTrue())
	})

	It("returns 400 on invalid request body", func() {
		req := httptest.NewRequest(http.MethodPost, "/sync", bytes.NewBufferString(`{`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusBadRequest))
	})

	It("returns 500 when service fails", func() {
		svc.syncFn = func(_ context.Context, _, _ string, _ *string) (*model.User, []model.Organization, error) {
			return nil, nil, errors.New("boom")
		}

		body, _ := json.Marshal(map[string]string{
			"name":  "John",
			"email": "john@example.com",
		})
		req := httptest.NewRequest(http.MethodPost, "/sync", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusInternalServerError))
	})
})
