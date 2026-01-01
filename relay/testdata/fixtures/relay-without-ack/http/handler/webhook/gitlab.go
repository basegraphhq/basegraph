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
	"go.opentelemetry.io/otel/trace"

	"basegraph.app/relay/common/logger"
	"basegraph.app/relay/internal/mapper"
	"basegraph.app/relay/internal/model"
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

	// Enrich context with component for all logs in this handler
	ctx = logger.WithLogFields(ctx, logger.LogFields{
		Component: "relay.http.webhook.gitlab",
	})

	integrationIDStr := c.Param("integration_id")
	integrationID, err := strconv.ParseInt(integrationIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid integration id"})
		return
	}

	// Add integration_id to context for subsequent logs
	ctx = logger.WithLogFields(ctx, logger.LogFields{
		IntegrationID: &integrationID,
	})

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

	slog.DebugContext(ctx, "received gitlab webhook", "body", bodyMap)

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
			"object_kind", payload.ObjectKind,
		)
		c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "event type not supported"})
		return
	}

	result, err := h.processEvent(ctx, integrationID, canonicalEventType, body, payload)
	if err != nil {
		slog.ErrorContext(ctx, "failed to process gitlab event",
			"error", err,
			"canonical_event_type", canonicalEventType,
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process event"})
		return
	}

	if !result.Engaged {
		slog.InfoContext(ctx, "gitlab webhook received but not engaged",
			"canonical_event_type", canonicalEventType,
			"object_kind", payload.ObjectKind,
		)
		c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "event not engaged"})
		return
	}

	slog.InfoContext(ctx, "gitlab webhook processed",
		"canonical_event_type", canonicalEventType,
		"object_kind", payload.ObjectKind,
		"object_id", payload.ObjectAttributes.ID,
		"object_iid", payload.ObjectAttributes.IID,
		"title", payload.ObjectAttributes.Title,
		"note", payload.ObjectAttributes.Note,
		"event_log_id", result.EventLog.ID,
		"issue_id", result.Issue.ID,
		"event_published", result.EventPublished,
	)

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *GitLabWebhookHandler) processEvent(ctx context.Context, integrationID int64, canonicalEventType mapper.CanonicalEventType, body []byte, payload gitlabWebhookPayload) (*service.EventIngestResult, error) {
	// For issue events, IID is in object_attributes
	// For note events (comments), IID is in the nested issue object. IID -> External Issue Id
	externalIssueID := payload.ObjectAttributes.IID
	if externalIssueID == 0 {
		externalIssueID = payload.Issue.IID
	}
	if externalIssueID == 0 {
		return nil, fmt.Errorf("no external issue id found in payload")
	}

	// For issue events, body is in ObjectAttributes.Description
	// For note events on issues, body is in Issue.Description
	issueBody := payload.ObjectAttributes.Description
	if issueBody == "" {
		issueBody = payload.Issue.Description
	}

	// Extract trace ID from OTel span (set by otelgin middleware) for propagation to worker
	var traceID *string
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		tid := span.SpanContext().TraceID().String()
		traceID = &tid
	}

	params := service.EventIngestParams{
		IntegrationID:       integrationID,
		ExternalIssueID:     strconv.FormatInt(externalIssueID, 10),
		ExternalProjectID:   payload.Project.ID,
		Provider:            model.ProviderGitLab,
		IssueBody:           issueBody,
		CommentBody:         payload.ObjectAttributes.Note,
		DiscussionID:        payload.ObjectAttributes.DiscussionID,
		CommentID:           strconv.FormatInt(payload.ObjectAttributes.ID, 10),
		TriggeredByUsername: payload.User.Username,
		EventType:           string(canonicalEventType),
		Payload:             body,
		TraceID:             traceID,
	}

	return h.eventIngest.Ingest(ctx, params)
}

type gitlabWebhookPayload struct {
	ObjectKind string `json:"object_kind"`
	Project    struct {
		ID     int64  `json:"id"`
		WebURL string `json:"web_url"`
	} `json:"project"`
	User struct {
		Id       int    `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
		Email    string `json:"email"`
	} `json:"user"`
	EventType        string `json:"event_type"`
	ObjectAttributes struct {
		Title        string `json:"title"`
		Description  string `json:"description"`
		Note         string `json:"note"`
		Action       string `json:"action"`
		ID           int64  `json:"id"`
		IID          int64  `json:"iid"`
		DiscussionID string `json:"discussion_id"`
	} `json:"object_attributes"`
	Issue struct {
		IID         int64  `json:"iid"`
		Title       string `json:"title"`
		Description string `json:"description"`
	} `json:"issue"`
}
