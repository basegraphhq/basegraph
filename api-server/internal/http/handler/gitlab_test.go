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

	"basegraph.app/api-server/internal/http/handler"
	"basegraph.app/api-server/internal/service/integration"
)

type mockGitLabService struct {
	listProjectsFn     func(ctx context.Context, instanceURL, token string) ([]integration.GitLabProject, error)
	setupIntegrationFn func(ctx context.Context, params integration.SetupIntegrationParams) (*integration.SetupResult, error)
	statusFn           func(ctx context.Context, workspaceID int64) (*integration.StatusResult, error)
	refreshFn          func(ctx context.Context, workspaceID int64, webhookBaseURL string) (*integration.SetupResult, error)
}

func (m *mockGitLabService) ListProjects(ctx context.Context, instanceURL, token string) ([]integration.GitLabProject, error) {
	if m.listProjectsFn != nil {
		return m.listProjectsFn(ctx, instanceURL, token)
	}
	return nil, nil
}

func (m *mockGitLabService) SetupIntegration(ctx context.Context, params integration.SetupIntegrationParams) (*integration.SetupResult, error) {
	if m.setupIntegrationFn != nil {
		return m.setupIntegrationFn(ctx, params)
	}
	return nil, nil
}

func (m *mockGitLabService) Status(ctx context.Context, workspaceID int64) (*integration.StatusResult, error) {
	if m.statusFn != nil {
		return m.statusFn(ctx, workspaceID)
	}
	return &integration.StatusResult{Connected: false}, nil
}

func (m *mockGitLabService) RefreshIntegration(ctx context.Context, workspaceID int64, webhookBaseURL string) (*integration.SetupResult, error) {
	if m.refreshFn != nil {
		return m.refreshFn(ctx, workspaceID, webhookBaseURL)
	}
	return nil, nil
}

var _ = Describe("GitLabHandler", func() {
	var (
		router *gin.Engine
		svc    *mockGitLabService
	)

	BeforeEach(func() {
		gin.SetMode(gin.TestMode)
		router = gin.New()
		svc = &mockGitLabService{}
		h := handler.NewGitLabHandler(svc, "https://relay")

		router.POST("/integrations/gitlab/projects", h.ListProjects)
		router.POST("/integrations/gitlab/setup", h.SetupIntegration)
	})

	It("lists projects", func() {
		svc.listProjectsFn = func(_ context.Context, _, _ string) ([]integration.GitLabProject, error) {
			return []integration.GitLabProject{
				{ID: 1, Name: "p1", PathWithNS: "g/p1", WebURL: "http://git/p1"},
			}, nil
		}

		body, _ := json.Marshal(map[string]string{
			"instance_url": "http://git",
			"token":        "pat-long-enough",
		})
		req := httptest.NewRequest(http.MethodPost, "/integrations/gitlab/projects", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusOK))
		var resp []map[string]interface{}
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp).To(HaveLen(1))
		Expect(resp[0]["path_with_namespace"]).To(Equal("g/p1"))
	})

	It("returns 502 when listing projects fails with generic error", func() {
		svc.listProjectsFn = func(_ context.Context, _, _ string) ([]integration.GitLabProject, error) {
			return nil, errors.New("boom")
		}

		body, _ := json.Marshal(map[string]string{
			"instance_url": "http://git",
			"token":        "pat-long-enough",
		})
		req := httptest.NewRequest(http.MethodPost, "/integrations/gitlab/projects", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusBadGateway))
		var resp map[string]interface{}
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp["error"]).To(Equal("boom"))
	})

	It("returns 400 with error message when no projects found", func() {
		svc.listProjectsFn = func(_ context.Context, _, _ string) ([]integration.GitLabProject, error) {
			return nil, errors.New("no projects found with maintainer access - ensure the token belongs to a user with Maintainer role on at least one project")
		}

		body, _ := json.Marshal(map[string]string{
			"instance_url": "http://git",
			"token":        "pat-long-enough",
		})
		req := httptest.NewRequest(http.MethodPost, "/integrations/gitlab/projects", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusBadRequest))
		var resp map[string]interface{}
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp["error"]).To(ContainSubstring("no projects found with maintainer access"))
	})

	It("sets up integration", func() {
		svc.setupIntegrationFn = func(_ context.Context, _ integration.SetupIntegrationParams) (*integration.SetupResult, error) {
			return &integration.SetupResult{
				IntegrationID:   42,
				Projects:        []integration.GitLabProject{{ID: 1, Name: "p1", PathWithNS: "g/p1"}},
				WebhooksCreated: 1,
				Errors:          nil,
			}, nil
		}

		body, _ := json.Marshal(map[string]interface{}{
			"instance_url":     "http://git",
			"token":            "pat-long-enough",
			"workspace_id":     "1",
			"organization_id":  "2",
			"setup_by_user_id": "3",
		})
		req := httptest.NewRequest(http.MethodPost, "/integrations/gitlab/setup", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusOK))
		var resp map[string]interface{}
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp["integration_id"]).To(Equal("42"))
		Expect(resp["webhooks_created"]).To(Equal(float64(1)))
	})

	It("returns 502 when setup fails with generic error", func() {
		svc.setupIntegrationFn = func(_ context.Context, _ integration.SetupIntegrationParams) (*integration.SetupResult, error) {
			return nil, errors.New("fail")
		}

		body, _ := json.Marshal(map[string]interface{}{
			"instance_url":     "http://git",
			"token":            "pat-long-enough",
			"workspace_id":     "1",
			"organization_id":  "2",
			"setup_by_user_id": "3",
		})
		req := httptest.NewRequest(http.MethodPost, "/integrations/gitlab/setup", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusBadGateway))
		var resp map[string]interface{}
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp["error"]).To(Equal("fail"))
	})

	It("returns 400 with error message when no projects found during setup", func() {
		svc.setupIntegrationFn = func(_ context.Context, _ integration.SetupIntegrationParams) (*integration.SetupResult, error) {
			return nil, errors.New("no projects found with maintainer access - ensure the token belongs to a user with Maintainer role on at least one project")
		}

		body, _ := json.Marshal(map[string]interface{}{
			"instance_url":     "http://git",
			"token":            "pat-long-enough",
			"workspace_id":     "1",
			"organization_id":  "2",
			"setup_by_user_id": "3",
		})
		req := httptest.NewRequest(http.MethodPost, "/integrations/gitlab/setup", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusBadRequest))
		var resp map[string]interface{}
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp["error"]).To(ContainSubstring("no projects found with maintainer access"))
	})
})
