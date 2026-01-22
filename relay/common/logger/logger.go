package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"basegraph.co/relay/core/config"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/trace"
)

func Setup(cfg config.Config) {
	var handler slog.Handler

	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	if cfg.IsDevelopment() {
		opts.Level = slog.LevelDebug
	}

	if cfg.IsProduction() && cfg.OTel.Enabled() {
		handler = otelslog.NewHandler(
			cfg.OTel.ServiceName,
			otelslog.WithLoggerProvider(global.GetLoggerProvider()),
		)
	} else if cfg.IsProduction() {
		handler = NewTraceHandler(slog.NewJSONHandler(os.Stdout, opts))
	} else {
		// Development mode: write logs to both stdout and file
		writer := createDevWriter()
		handler = NewTraceHandler(slog.NewTextHandler(writer, opts))
	}

	slog.SetDefault(slog.New(handler))
}

func createDevWriter() io.Writer {
	// Create logs directory if it doesn't exist
	logsDir := "logs"
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create logs directory: %v\n", err)
		return os.Stdout
	}

	// Create log file with timestamp
	timestamp := time.Now().Format("2006-01-02")
	logFileName := filepath.Join(logsDir, fmt.Sprintf("relay-%s.log", timestamp))

	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to open log file: %v\n", err)
		return os.Stdout
	}

	// Write to both stdout and file
	return io.MultiWriter(os.Stdout, logFile)
}

type TraceHandler struct {
	slog.Handler
}

func NewTraceHandler(h slog.Handler) *TraceHandler {
	return &TraceHandler{Handler: h}
}

func (h *TraceHandler) Handle(ctx context.Context, r slog.Record) error {
	// Add OTel trace/span IDs from context
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		sc := span.SpanContext()
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}

	// Add structured fields from context (automatic enrichment)
	fields := GetLogFields(ctx)
	if fields.IssueID != nil {
		r.AddAttrs(slog.Int64("issue_id", *fields.IssueID))
	}
	if fields.EventLogID != nil {
		r.AddAttrs(slog.Int64("event_log_id", *fields.EventLogID))
	}
	if fields.MessageID != nil {
		r.AddAttrs(slog.String("message_id", *fields.MessageID))
	}
	if fields.IntegrationID != nil {
		r.AddAttrs(slog.Int64("integration_id", *fields.IntegrationID))
	}
	if fields.WorkspaceID != nil {
		r.AddAttrs(slog.Int64("workspace_id", *fields.WorkspaceID))
	}
	if fields.EventType != nil {
		r.AddAttrs(slog.String("event_type", *fields.EventType))
	}
	if fields.Component != "" {
		r.AddAttrs(slog.String("component", fields.Component))
	}

	return h.Handler.Handle(ctx, r)
}

func (h *TraceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &TraceHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *TraceHandler) WithGroup(name string) slog.Handler {
	return &TraceHandler{Handler: h.Handler.WithGroup(name)}
}
