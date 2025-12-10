package webhook

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/store"
)

type GitLabWebhookHandler struct {
	credentialStore store.IntegrationCredentialStore
	logger          *slog.Logger
}

// logger is injected so tests can capture output without relying on the global slog default.
func NewGitLabWebhookHandler(credentialStore store.IntegrationCredentialStore, logger *slog.Logger) *GitLabWebhookHandler {
	return &GitLabWebhookHandler{
		credentialStore: credentialStore,
		logger:          logger,
	}
}

func (h *GitLabWebhookHandler) HandleEvent(c *gin.Context) {
	ctx := c.Request.Context()

	integrationIDStr := c.Param("integration_id")
	integrationID, err := strconv.ParseInt(integrationIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid integration id"})
		return
	}

	secretHeader := c.GetHeader("X-Gitlab-Token")
	if secretHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing webhook token"})
		return
	}

	creds, err := h.credentialStore.ListActiveByIntegration(ctx, integrationID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "integration not found"})
		return
	}

	var webhookSecret *string
	for _, cred := range creds {
		if cred.CredentialType == model.CredentialTypeWebhookSecret {
			webhookSecret = &cred.AccessToken
			break
		}
	}

	if webhookSecret == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "webhook secret not configured"})
		return
	}

	if secretHeader != *webhookSecret {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid webhook token"})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	var payload gitlabWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	eventType := c.GetHeader("X-Gitlab-Event")
	if eventType == "" {
		eventType = payload.EventType
	}
	if eventType == "" {
		eventType = payload.ObjectKind
	}

	h.logger.InfoContext(ctx, "gitlab webhook received",
		"integration_id", integrationID,
		"event_type", eventType,
		"object_kind", payload.ObjectKind,
		"object_id", payload.ObjectAttributes.ID,
		"object_iid", payload.ObjectAttributes.IID,
		"title", payload.ObjectAttributes.Title,
		"note", payload.ObjectAttributes.Note,
	)

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type gitlabWebhookPayload struct {
	ObjectKind       string `json:"object_kind"`
	EventType        string `json:"event_type"`
	ObjectAttributes struct {
		Title  string `json:"title"`
		Note   string `json:"note"`
		Action string `json:"action"`
		ID     int64  `json:"id"`
		IID    int64  `json:"iid"`
	} `json:"object_attributes"`
}
