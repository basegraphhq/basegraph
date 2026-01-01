package logger

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "relay-worker"

// SpanContext wraps an OTel span for managed lifecycle.
// Use Start() to begin a span and End() to complete it.
type SpanContext struct {
	ctx  context.Context
	span trace.Span
}

// StartSpan creates a new span as a child of the current trace context.
// Returns a SpanContext that must be ended with End().
//
// Example:
//
//	sc := logger.StartSpan(ctx, "worker.process_message", trace.WithSpanKind(trace.SpanKindConsumer))
//	defer sc.End()
//	ctx = sc.Context()
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) *SpanContext {
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, name, opts...)
	return &SpanContext{ctx: ctx, span: span}
}

// StartSpanFromTraceID creates a new span linked to a remote trace.
// Used when propagating trace context across process boundaries (e.g., Redis queue).
// If traceID is empty or invalid, returns a span with no parent trace.
//
// Example:
//
//	sc := logger.StartSpanFromTraceID(ctx, msg.TraceID, "worker.process_message")
//	defer sc.End()
//	ctx = sc.Context()
func StartSpanFromTraceID(ctx context.Context, traceIDStr string, name string, opts ...trace.SpanStartOption) *SpanContext {
	tracer := otel.Tracer(tracerName)

	if traceIDStr == "" {
		ctx, span := tracer.Start(ctx, name, opts...)
		return &SpanContext{ctx: ctx, span: span}
	}

	traceID, err := trace.TraceIDFromHex(traceIDStr)
	if err != nil {
		ctx, span := tracer.Start(ctx, name, opts...)
		return &SpanContext{ctx: ctx, span: span}
	}

	// Create span context with the propagated trace ID
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})

	// Start span as child of the remote trace
	opts = append(opts, trace.WithLinks(trace.Link{SpanContext: spanCtx}))
	ctx = trace.ContextWithRemoteSpanContext(ctx, spanCtx)
	ctx, span := tracer.Start(ctx, name, opts...)

	return &SpanContext{ctx: ctx, span: span}
}

// Context returns the context with the span attached.
// Use this context for all operations within the span's scope.
func (sc *SpanContext) Context() context.Context {
	return sc.ctx
}

// End completes the span. Must be called to ensure proper trace reporting.
// Safe to call multiple times (subsequent calls are no-ops).
func (sc *SpanContext) End() {
	if sc.span != nil {
		sc.span.End()
	}
}

// RecordError records an error on the span for observability.
func (sc *SpanContext) RecordError(err error) {
	if sc.span != nil && err != nil {
		sc.span.RecordError(err)
	}
}

// Span returns the underlying OTel span for advanced operations.
func (sc *SpanContext) Span() trace.Span {
	return sc.span
}
