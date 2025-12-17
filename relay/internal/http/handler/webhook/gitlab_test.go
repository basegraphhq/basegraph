package webhook_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"log/slog"

	"basegraph.app/relay/internal/http/handler/webhook"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/service"
	"basegraph.app/relay/internal/store"
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
	var active []model.IntegrationCredential
	for _, cred := range f.creds {
		if cred.RevokedAt == nil {
			active = append(active, cred)
		}
	}
	return active, nil
}

func (f *fakeCredStore) GetWebhookSecret(ctx context.Context, integrationID int64) (string, error) {
	active, err := f.ListActiveByIntegration(ctx, integrationID)
	if err != nil {
		return "", err
	}
	for _, cred := range active {
		if cred.CredentialType == model.CredentialTypeWebhookSecret {
			return cred.AccessToken, nil
		}
	}
	return "", fmt.Errorf("webhook secret not found")
}

func (f *fakeCredStore) ValidateWebhookToken(ctx context.Context, integrationID int64, token string) error {
	webhookSecret, err := f.GetWebhookSecret(ctx, integrationID)
	if err != nil {
		return err
	}
	if webhookSecret != token {
		return fmt.Errorf("invalid webhook token")
	}
	return nil
}

type fakeEventIngestService struct{}

func (f *fakeEventIngestService) Ingest(ctx context.Context, params service.EventIngestParams) (*service.EventIngestResult, error) {
	return &service.EventIngestResult{
		EventLog: &model.EventLog{ID: 12345},
		Issue:    &model.Issue{ID: 67890},
		Enqueued: true,
	}, nil
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
		slog.SetDefault(slog.New(slog.NewJSONHandler(buf, nil)))

		store := &fakeCredStore{
			creds: []model.IntegrationCredential{
				{IntegrationID: 123, CredentialType: model.CredentialTypeWebhookSecret, AccessToken: "secret"},
			},
		}

		eventIngest := &fakeEventIngestService{}
		h := webhook.NewGitLabWebhookHandler(store, eventIngest)
		router.POST("/webhooks/gitlab/:integration_id", h.HandleEvent)
	})

	It("accepts valid token and processes issue payload", func() {
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
		Expect(logStr).To(ContainSubstring(`"object_kind":"issue"`))
		Expect(logStr).To(ContainSubstring(`"object_id":10`))
		Expect(logStr).To(ContainSubstring(`"object_iid":5`))
		Expect(logStr).To(ContainSubstring(`"title":"Bug"`))
		Expect(logStr).To(ContainSubstring(`"event_log_id":12345`))
		Expect(logStr).To(ContainSubstring(`"issue_id":67890`))
		Expect(logStr).To(ContainSubstring(`"enqueued":true`))
	})

	It("rejects invalid token", func() {
		req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab/123", bytes.NewBuffer([]byte("{}")))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Gitlab-Token", "wrong")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusUnauthorized))
	})

	It("handles wiki events without processing", func() {
		body := map[string]interface{}{
			"object_kind": "wiki_page",
			"object_attributes": map[string]interface{}{
				"title":  "Test Page",
				"slug":   "test-page",
				"action": "create",
			},
		}
		payload, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab/123", bytes.NewBuffer(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Gitlab-Token", "secret")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusOK))
		logStr := buf.String()
		Expect(logStr).To(ContainSubstring("gitlab wiki webhook received"))
	})
})
