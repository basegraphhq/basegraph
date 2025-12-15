package webhook_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"basegraph.app/api-server/internal/http/handler/webhook"
	"basegraph.app/api-server/internal/model"
	"basegraph.app/api-server/internal/store"
)

type fakeCredStore struct {
	creds []model.IntegrationCredential
}

func (f *fakeCredStore) GetByID(ctx context.Context, id int64) (*model.IntegrationCredential, error) {
	return nil, store.ErrNotFound
}

func (f *fakeCredStore) GetPrimaryByIntegration(ctx context.Context, integrationID int64) (*model.IntegrationCredential, error) {
	return nil, store.ErrNotFound
}

func (f *fakeCredStore) GetByIntegrationAndUser(ctx context.Context, integrationID int64, userID int64) (*model.IntegrationCredential, error) {
	return nil, store.ErrNotFound
}

func (f *fakeCredStore) Create(ctx context.Context, cred *model.IntegrationCredential) error {
	return nil
}

func (f *fakeCredStore) UpdateTokens(ctx context.Context, id int64, accessToken string, refreshToken *string, expiresAt *time.Time) error {
	return nil
}

func (f *fakeCredStore) SetAsPrimary(ctx context.Context, integrationID int64, credentialID int64) error {
	return nil
}

func (f *fakeCredStore) Revoke(ctx context.Context, id int64) error {
	return nil
}

func (f *fakeCredStore) RevokeAllByIntegration(ctx context.Context, integrationID int64) error {
	return nil
}

func (f *fakeCredStore) Delete(ctx context.Context, id int64) error {
	return nil
}

func (f *fakeCredStore) ListByIntegration(ctx context.Context, integrationID int64) ([]model.IntegrationCredential, error) {
	return f.creds, nil
}

func (f *fakeCredStore) ListActiveByIntegration(ctx context.Context, integrationID int64) ([]model.IntegrationCredential, error) {
	return f.creds, nil
}

var _ = Describe("GitLabWebhookHandler", func() {
	var (
		router *gin.Engine
		buf    *bytes.Buffer
	)

	BeforeEach(func() {
		gin.SetMode(gin.TestMode)
		router = gin.New()
		buf = &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{AddSource: false}))

		store := &fakeCredStore{
			creds: []model.IntegrationCredential{
				{IntegrationID: 123, CredentialType: model.CredentialTypeWebhookSecret, AccessToken: "secret"},
			},
		}

		h := webhook.NewGitLabWebhookHandler(store, logger)
		router.POST("/webhooks/gitlab/:integration_id", h.HandleEvent)
	})

	It("accepts valid token and logs parsed issue payload", func() {
		body := map[string]interface{}{
			"object_kind": "issue",
			"object_attributes": map[string]interface{}{
				"id":     10,
				"iid":    5,
				"title":  "Bug",
				"note":   "",
				"action": "open",
			},
		}
		payload, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab/123", bytes.NewBuffer(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Gitlab-Token", "secret")
		req.Header.Set("X-Gitlab-Event", "Issue Hook")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusOK))
		logStr := buf.String()
		Expect(logStr).To(ContainSubstring("Issue Hook"))
		Expect(logStr).To(ContainSubstring("object_kind=issue"))
		Expect(logStr).To(ContainSubstring("object_id=10"))
		Expect(logStr).To(ContainSubstring("object_iid=5"))
		Expect(logStr).To(ContainSubstring("title=Bug"))
	})

	It("accepts valid token and logs parsed note payload", func() {
		body := map[string]interface{}{
			"object_kind": "note",
			"object_attributes": map[string]interface{}{
				"id":     20,
				"iid":    0,
				"title":  "",
				"note":   "hello",
				"action": "create",
			},
		}
		payload, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab/123", bytes.NewBuffer(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Gitlab-Token", "secret")
		req.Header.Set("X-Gitlab-Event", "Note Hook")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusOK))
		logStr := buf.String()
		Expect(logStr).To(ContainSubstring("Note Hook"))
		Expect(logStr).To(ContainSubstring("object_kind=note"))
		Expect(logStr).To(ContainSubstring("object_id=20"))
		Expect(logStr).To(ContainSubstring("note=hello"))
	})

	It("accepts valid token and logs parsed wiki page payload", func() {
		body := map[string]interface{}{
			"object_kind": "wiki_page",
			"user": map[string]interface{}{
				"id":       1,
				"name":     "Admin",
				"username": "admin",
			},
			"project": map[string]interface{}{
				"id":                  10,
				"name":                "my-project",
				"path_with_namespace": "group/my-project",
			},
			"wiki": map[string]interface{}{
				"web_url":             "http://example.com/group/my-project/-/wikis",
				"path_with_namespace": "group/my-project.wiki",
			},
			"object_attributes": map[string]interface{}{
				"title":   "Getting Started",
				"content": "# Welcome\n\nThis is the getting started guide.",
				"format":  "markdown",
				"message": "Add getting started page",
				"slug":    "getting-started",
				"url":     "http://example.com/group/my-project/-/wikis/getting-started",
				"action":  "create",
			},
		}
		payload, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab/123", bytes.NewBuffer(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Gitlab-Token", "secret")
		req.Header.Set("X-Gitlab-Event", "Wiki Page Hook")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusOK))
		logStr := buf.String()
		Expect(logStr).To(ContainSubstring("gitlab wiki webhook received"))
		Expect(logStr).To(ContainSubstring("Wiki Page Hook"))
		Expect(logStr).To(ContainSubstring("action=create"))
		Expect(logStr).To(ContainSubstring("title=\"Getting Started\""))
		Expect(logStr).To(ContainSubstring("slug=getting-started"))
		Expect(logStr).To(ContainSubstring("format=markdown"))
	})

	It("rejects missing token", func() {
		req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab/123", bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		Expect(w.Code).To(Equal(http.StatusUnauthorized))
	})

	It("rejects invalid token", func() {
		req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab/123", bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Gitlab-Token", "wrong")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		Expect(w.Code).To(Equal(http.StatusUnauthorized))
	})

	It("rejects malformed payload", func() {
		req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab/123", bytes.NewBufferString(`{`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Gitlab-Token", "secret")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		Expect(w.Code).To(Equal(http.StatusBadRequest))
	})

	It("rejects when no webhook secret configured", func() {
		// override router with empty credential store
		router = gin.New()
		logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{AddSource: false}))
		h := webhook.NewGitLabWebhookHandler(&fakeCredStore{}, logger)
		router.POST("/webhooks/gitlab/:integration_id", h.HandleEvent)

		req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab/123", bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Gitlab-Token", "secret")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		Expect(w.Code).To(Equal(http.StatusUnauthorized))
	})
})
