# Logging

## The One Rule

```go
slog.InfoContext(ctx, "message", "key", value)   // correct
slog.Info("message", "key", value)                // wrong
```

Always use `*Context` variants. This ensures trace correlation.

## Quick Reference

```go
slog.DebugContext(ctx, "detailed info for debugging", "user_id", userID)
slog.InfoContext(ctx, "normal operation", "action", "user_created")
slog.WarnContext(ctx, "unexpected but handled", "retry_count", 3)
slog.ErrorContext(ctx, "operation failed", "error", err)
```

## How It Works

| Environment | Output | Destination |
|-------------|--------|-------------|
| Development | Human-readable text | stdout (your terminal) |
| Production  | JSON with trace_id/span_id | OTLP → SigNoz |

**Development:**
```
time=2025-12-08T10:30:45Z level=INFO msg="user created" user_id=123 trace_id=abc...
```

**Production:**
```json
{"time":"2025-12-08T10:30:45Z","level":"INFO","msg":"user created","user_id":123,"trace_id":"abc...","span_id":"xyz..."}
```

## Environment Variables

```bash
# Required for production
OTEL_EXPORTER_OTLP_ENDPOINT=https://ingest.signoz.cloud:443
OTEL_EXPORTER_OTLP_HEADERS=signoz-ingestion-key=YOUR_KEY

# Optional
OTEL_SERVICE_NAME=relay        # default: relay
OTEL_SERVICE_VERSION=v1.2.3    # default: dev
```

No endpoint = no OTel export. Safe for local dev.

## What Gets Logged Automatically

The request logging middleware logs every HTTP request:

```
method=POST path=/api/v1/users status=201 latency_ms=45 client_ip=192.168.1.1 trace_id=abc...
```

You don't need to log request start/end in handlers.

## When to Log

**Log:**
- Business events: user created, payment processed, spec generated
- Errors with context: what failed, relevant IDs
- Decisions: why a code path was taken (if non-obvious)

**Don't log:**
- Request/response bodies (handled by middleware, security risk)
- Passwords, tokens, PII
- Every function entry/exit (noise)

## Tracing in SigNoz

1. Find a log entry in SigNoz
2. Click the `trace_id`
3. See the full request trace with all spans

Logs and traces are automatically correlated. No extra work needed.

