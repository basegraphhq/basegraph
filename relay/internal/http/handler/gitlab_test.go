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

	"basegraph.co/relay/internal/http/handler"
	"basegraph.co/relay/internal/service"
	"basegraph.co/relay/internal/service/integration"
)

type mockGitLabService struct {
	listProjectsFn     func(ctx context.Context, instanceURL, token string) ([]integration.GitLabProject, error)
	setupIntegrationFn func(ctx context.Context, params integration.SetupIntegrationParams) (*integration.SetupResult, error)
	enableReposFn      func(ctx context.Context, params integration.EnableRepositoriesParams) (*integration.EnableRepositoriesResult, error)
	listEnabledFn      func(ctx context.Context, workspaceID int64) ([]int64, error)
	statusFn           func(ctx context.Context, workspaceID int64) (*integration.StatusResult, error)
	refreshFn          func(ctx context.Context, workspaceID int64, webhookBaseURL string) (*integration.SetupResult, error)
}

type mockWorkspaceSetupService struct {
	enqueueFn func(ctx context.Context, workspaceID int64) (*service.WorkspaceSetupResult, error)
}

func (m *mockWorkspaceSetupService) Enqueue(ctx context.Context, workspaceID int64) (*service.WorkspaceSetupResult, error) {
	if m.enqueueFn != nil {
		return m.enqueueFn(ctx, workspaceID)
	}
	return &service.WorkspaceSetupResult{RunID: 1}, nil
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

func (m *mockGitLabService) EnableRepositories(ctx context.Context, params integration.EnableRepositoriesParams) (*integration.EnableRepositoriesResult, error) {
	if m.enableReposFn != nil {
		return m.enableReposFn(ctx, params)
	}
	return nil, nil
}

func (m *mockGitLabService) ListEnabledProjectIDs(ctx context.Context, workspaceID int64) ([]int64, error) {
	if m.listEnabledFn != nil {
		return m.listEnabledFn(ctx, workspaceID)
	}
	return []int64{}, nil
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
		setup  *mockWorkspaceSetupService
	)

	BeforeEach(func() {
		gin.SetMode(gin.TestMode)
		router = gin.New()
		svc = &mockGitLabService{}
		setup = &mockWorkspaceSetupService{}
		h := handler.NewGitLabHandler(svc, setup, "https://relay")

		router.POST("/integrations/gitlab/projects", h.ListProjects)
		router.POST("/integrations/gitlab/setup", h.SetupIntegration)
		router.POST("/integrations/gitlab/repos/enable", h.EnableRepositories)
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
		var resp []map[string]any
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
		var resp map[string]any
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
		var resp map[string]any
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

		body, _ := json.Marshal(map[string]any{
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
		var resp map[string]any
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp["integration_id"]).To(Equal("42"))
		Expect(resp["webhooks_created"]).To(Equal(float64(1)))
	})

	It("enables repositories and enqueues workspace setup", func() {
		svc.enableReposFn = func(_ context.Context, params integration.EnableRepositoriesParams) (*integration.EnableRepositoriesResult, error) {
			return &integration.EnableRepositoriesResult{IntegrationID: 10}, nil
		}
		called := false
		setup.enqueueFn = func(_ context.Context, workspaceID int64) (*service.WorkspaceSetupResult, error) {
			called = true
			Expect(workspaceID).To(Equal(int64(42)))
			return &service.WorkspaceSetupResult{RunID: 99}, nil
		}

		body, _ := json.Marshal(map[string]any{
			"workspace_id": "42",
			"project_ids":  []int64{1, 2},
		})
		req := httptest.NewRequest(http.MethodPost, "/integrations/gitlab/repos/enable", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusOK))
		Expect(called).To(BeTrue())
	})

	It("returns 502 when setup fails with generic error", func() {
		svc.setupIntegrationFn = func(_ context.Context, _ integration.SetupIntegrationParams) (*integration.SetupResult, error) {
			return nil, errors.New("fail")
		}

		body, _ := json.Marshal(map[string]any{
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
		var resp map[string]any
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp["error"]).To(Equal("fail"))
	})

	It("returns 400 with error message when no projects found during setup", func() {
		svc.setupIntegrationFn = func(_ context.Context, _ integration.SetupIntegrationParams) (*integration.SetupResult, error) {
			return nil, errors.New("no projects found with maintainer access - ensure the token belongs to a user with Maintainer role on at least one project")
		}

		body, _ := json.Marshal(map[string]any{
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
		var resp map[string]any
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp["error"]).To(ContainSubstring("no projects found with maintainer access"))
	})
})
