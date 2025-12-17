package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	gitlab "gitlab.com/gitlab-org/api/client-go"

	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/service"
	"basegraph.app/relay/internal/store"
)

type GitLabWebhookHandler struct {
	credentialStore store.IntegrationCredentialStore
	eventIngest     service.EventIngestService
}

func NewGitLabWebhookHandler(credentialStore store.IntegrationCredentialStore, eventIngest service.EventIngestService) *GitLabWebhookHandler {
	return &GitLabWebhookHandler{
		credentialStore: credentialStore,
		eventIngest:     eventIngest,
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

	// Wiki events don't require processing as they don't represent issues or discussions
	if eventType == string(gitlab.EventTypeWikiPage) || payload.ObjectKind == "wiki_page" {
		var wikiEvent gitlab.WikiPageEvent
		if err := json.Unmarshal(body, &wikiEvent); err == nil {
			slog.InfoContext(ctx, "gitlab wiki webhook received",
				"integration_id", integrationID,
				"event_type", eventType,
				"action", wikiEvent.ObjectAttributes.Action,
				"title", wikiEvent.ObjectAttributes.Title,
				"slug", wikiEvent.ObjectAttributes.Slug,
				"url", wikiEvent.ObjectAttributes.URL,
				"format", wikiEvent.ObjectAttributes.Format,
			)
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
			return
		}
	}

	result, err := h.processGitLabEvent(ctx, integrationID, eventType, body, payload)
	if err != nil {
		slog.ErrorContext(ctx, "failed to process gitlab event", "error", err, "integration_id", integrationID, "event_type", eventType)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process event"})
		return
	}

	slog.InfoContext(ctx, "gitlab webhook processed",
		"integration_id", integrationID,
		"event_type", eventType,
		"object_kind", payload.ObjectKind,
		"object_id", payload.ObjectAttributes.ID,
		"object_iid", payload.ObjectAttributes.IID,
		"title", payload.ObjectAttributes.Title,
		"note", payload.ObjectAttributes.Note,
		"event_log_id", result.EventLog.ID,
		"issue_id", result.Issue.ID,
		"enqueued", result.Enqueued,
	)

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *GitLabWebhookHandler) processGitLabEvent(ctx context.Context, integrationID int64, eventType string, body []byte, payload gitlabWebhookPayload) (*service.EventIngestResult, error) {
	canonicalEventType := h.mapGitLabEventType(eventType, payload.ObjectKind)
	if canonicalEventType == "" {
		slog.DebugContext(ctx, "ignoring unknown gitlab event type", "event_type", eventType, "object_kind", payload.ObjectKind)
		return &service.EventIngestResult{Enqueued: false}, nil
	}

	issueID, title, description := h.extractIssueData(payload, body)
	if issueID == "" {
		return nil, fmt.Errorf("no issue ID found in payload")
	}

	params := service.EventIngestParams{
		IntegrationID:   integrationID,
		ExternalIssueID: issueID,
		EventType:       canonicalEventType,
		Source:          nil,
		Payload:         body,
		Title:           title,
		Description:     description,
	}

	return h.eventIngest.Ingest(ctx, params)
}

func (h *GitLabWebhookHandler) mapGitLabEventType(headerEventType, objectKind string) string {
	switch headerEventType {
	case "Issue Hook":
		return "issue_created"
	case "Note Hook":
		return "reply"
	case "Merge Request Hook":
		return "merge_request_created"
	default:
		switch objectKind {
		case "issue":
			return "issue_created"
		case "note":
			return "reply"
		case "merge_request":
			return "merge_request_created"
		default:
			return ""
		}
	}
}

func (h *GitLabWebhookHandler) extractIssueData(payload gitlabWebhookPayload, body []byte) (issueID string, title *string, description *string) {
	if payload.ObjectAttributes.IID > 0 {
		issueID = strconv.FormatInt(payload.ObjectAttributes.IID, 10)
		title = &payload.ObjectAttributes.Title
		return issueID, title, nil
	}

	var fullPayload map[string]interface{}
	if err := json.Unmarshal(body, &fullPayload); err == nil {
		if issue, ok := fullPayload["issue"].(map[string]interface{}); ok {
			if iid, ok := issue["iid"].(float64); ok {
				issueID = strconv.FormatInt(int64(iid), 10)
			}
		}
	}

	return issueID, nil, nil
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
