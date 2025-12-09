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

var _ = Describe("OrganizationHandler", func() {
	var (
		router *gin.Engine
		svc    *mockOrganizationService
	)

	BeforeEach(func() {
		gin.SetMode(gin.TestMode)
		router = gin.New()
		svc = &mockOrganizationService{}
		h := handler.NewOrganizationHandler(svc)
		router.POST("/organizations", h.Create)
	})

	It("returns 201 when organization is created", func() {
		svc.createFn = func(_ context.Context, _ string, _ *string, admin int64) (*model.Organization, error) {
			return &model.Organization{ID: 1, Name: "Acme", Slug: "acme", AdminUserID: admin}, nil
		}

		body, _ := json.Marshal(map[string]interface{}{
			"name":          "Acme",
			"admin_user_id": 10,
		})

		req := httptest.NewRequest(http.MethodPost, "/organizations", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusCreated))
		var resp map[string]interface{}
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp["slug"]).To(Equal("acme"))
	})

	It("returns 400 on invalid body", func() {
		req := httptest.NewRequest(http.MethodPost, "/organizations", bytes.NewBufferString(`{`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusBadRequest))
	})

	It("returns 500 on service error", func() {
		svc.createFn = func(_ context.Context, _ string, _ *string, _ int64) (*model.Organization, error) {
			return nil, errors.New("fail")
		}

		body, _ := json.Marshal(map[string]interface{}{
			"name":          "Acme",
			"admin_user_id": 10,
		})
		req := httptest.NewRequest(http.MethodPost, "/organizations", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusInternalServerError))
	})
})
