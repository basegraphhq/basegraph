package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"

	"basegraph.app/relay/internal/http/dto"
	"basegraph.app/relay/internal/service"
)

type EventIngestHandler struct {
	service     service.EventIngestService
	traceHeader string
}

func NewEventIngestHandler(service service.EventIngestService, traceHeader string) *EventIngestHandler {
	return &EventIngestHandler{
		service:     service,
		traceHeader: traceHeader,
	}
}

func (h *EventIngestHandler) Ingest(c *gin.Context) {
	ctx := c.Request.Context()

	var req dto.IngestEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.WarnContext(ctx, "invalid ingest request", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	traceID := c.GetHeader(h.traceHeader)
	if traceID == "" {
		if spanCtx := trace.SpanContextFromContext(ctx); spanCtx.IsValid() {
			traceID = spanCtx.TraceID().String()
		}
	}
	params := service.EventIngestParams{
		IntegrationID:       req.IntegrationID,
		ExternalIssueID:     req.ExternalIssueID,
		TriggeredByUsername: req.TriggeredByUsername,
		EventType:           req.EventType,
		Payload:             req.Payload,
	}
	if traceID != "" {
		params.TraceID = &traceID
	}

	result, err := h.service.Ingest(ctx, params)
	if err != nil {
		if errors.Is(err, service.ErrIntegrationNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "integration not found"})
			return
		}
		slog.ErrorContext(ctx, "failed to ingest event", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to ingest event"})
		return
	}

	c.JSON(http.StatusAccepted, dto.IngestEventResponse{
		EventLogID: result.EventLog.ID,
		IssueID:    result.Issue.ID,
		DedupeKey:  result.DedupeKey,
		Enqueued:   result.Enqueued,
		Duplicated: result.Duplicated,
	})
}
