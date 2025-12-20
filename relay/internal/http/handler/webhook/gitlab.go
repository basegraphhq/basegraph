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

	"basegraph.app/relay/internal/mapper"
	"basegraph.app/relay/internal/service"
)

type GitLabWebhookHandler struct {
	credentialService service.IntegrationCredentialService
	eventIngest       service.EventIngestService
	mapper            mapper.EventMapper
}

func NewGitLabWebhookHandler(credentialService service.IntegrationCredentialService, eventIngest service.EventIngestService, mapper mapper.EventMapper) *GitLabWebhookHandler {
	return &GitLabWebhookHandler{
		credentialService: credentialService,
		eventIngest:       eventIngest,
		mapper:            mapper,
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

	if err := h.credentialService.ValidateWebhookToken(ctx, integrationID, secretHeader); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid webhook token"})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	// Parse body as map for mapper and as struct for logging/extraction
	var bodyMap map[string]any
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	slog.InfoContext(ctx, "received gitlab webhook", "body", bodyMap)

	var payload gitlabWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	// Wiki events don't require processing as they don't represent issues or discussions
	// TODO: @nithinsj: Process wiki events
	if payload.ObjectKind == "wiki_page" {
		var wikiEvent gitlab.WikiPageEvent
		if err := json.Unmarshal(body, &wikiEvent); err == nil {
			slog.InfoContext(ctx, "gitlab wiki webhook received",
				"integration_id", integrationID,
				"object_kind", payload.ObjectKind,
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

	// Build headers map for mapper
	headers := make(map[string]string)
	for key, values := range c.Request.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	// Map to canonical event type
	canonicalEventType, err := h.mapper.Map(ctx, bodyMap, headers)
	if err != nil {
		slog.WarnContext(ctx, "unknown gitlab event type, ignoring",
			"error", err,
			"integration_id", integrationID,
			"object_kind", payload.ObjectKind,
		)
		c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "event type not supported"})
		return
	}

	result, err := h.processGitLabEvent(ctx, integrationID, canonicalEventType, body, payload)
	if err != nil {
		slog.ErrorContext(ctx, "failed to process gitlab event",
			"error", err,
			"integration_id", integrationID,
			"canonical_event_type", canonicalEventType,
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process event"})
		return
	}

	slog.InfoContext(ctx, "gitlab webhook processed",
		"integration_id", integrationID,
		"canonical_event_type", canonicalEventType,
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

func (h *GitLabWebhookHandler) processGitLabEvent(ctx context.Context, integrationID int64, canonicalEventType mapper.CanonicalEventType, body []byte, payload gitlabWebhookPayload) (*service.EventIngestResult, error) {
	// For issue events, IID is in object_attributes
	// For note events (comments), IID is in the nested issue object
	issueIID := payload.ObjectAttributes.IID
	if issueIID == 0 {
		issueIID = payload.Issue.IID
	}
	if issueIID == 0 { // if still 0, then it's an error
		return nil, fmt.Errorf("no issue IID found in payload")
	}

	params := service.EventIngestParams{
		IntegrationID:       integrationID,
		ExternalIssueID:     strconv.FormatInt(issueIID, 10),
		TriggeredByUsername: payload.User.Username,
		EventType:           string(canonicalEventType),
		Payload:             body,
	}

	return h.eventIngest.Ingest(ctx, params)
}

type gitlabWebhookPayload struct {
	ObjectKind string `json:"object_kind"`
	User       struct {
		Id       int    `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
		Email    string `json:"email"`
	} `json:"user"`
	EventType        string `json:"event_type"`
	ObjectAttributes struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Note        string `json:"note"`
		Action      string `json:"action"`
		ID          int64  `json:"id"`
		IID         int64  `json:"iid"`
	} `json:"object_attributes"`
	Issue struct {
		IID         int64  `json:"iid"`
		Title       string `json:"title"`
		Description string `json:"description"`
	} `json:"issue"`
}
