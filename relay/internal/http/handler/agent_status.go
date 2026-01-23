package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type AgentStatusHandler struct {
	redis *redis.Client
}

func NewAgentStatusHandler(redisClient *redis.Client) *AgentStatusHandler {
	return &AgentStatusHandler{redis: redisClient}
}

func (h *AgentStatusHandler) Stream(c *gin.Context) {
	ctx := c.Request.Context()
	if h.redis == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "redis not configured"})
		return
	}

	orgID := c.Param("org_id")
	workspaceID := c.Param("workspace_id")
	if orgID == "" || workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing org_id or workspace_id"})
		return
	}

	stream := fmt.Sprintf("agent-status:org-%s:workspace-%s", orgID, workspaceID)
	lastID := c.Query("last_id")
	if lastID == "" {
		lastID = "$"
	}

	setSSEHeaders(c.Writer)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	sseWrite(c.Writer, "ping", "ready")
	flusher.Flush()

	clientClosed := c.Request.Context().Done()

	for {
		select {
		case <-clientClosed:
			return
		default:
		}

		res, err := h.redis.XRead(ctx, &redis.XReadArgs{
			Streams: []string{stream, lastID},
			Block:   25 * time.Second,
			Count:   100,
		}).Result()
		if err != nil {
			if err == redis.Nil {
				sseWrite(c.Writer, "ping", time.Now().UTC().Format(time.RFC3339Nano))
				flusher.Flush()
				continue
			}
			if ctx.Err() != nil {
				return
			}
			sseWrite(c.Writer, "error", map[string]string{"error": err.Error()})
			flusher.Flush()
			continue
		}

		for _, streamRes := range res {
			for _, msg := range streamRes.Messages {
				lastID = msg.ID
				sseWrite(c.Writer, "status", msg)
				flusher.Flush()
			}
		}
	}
}

func setSSEHeaders(w http.ResponseWriter) {
	headers := w.Header()
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Connection", "keep-alive")
	headers.Set("X-Accel-Buffering", "no")
}

func sseWrite(w http.ResponseWriter, event string, data any) {
	payload := marshalPayload(data)
	if event != "" {
		_, _ = fmt.Fprintf(w, "event: %s\n", event)
	}
	for _, line := range strings.Split(payload, "\n") {
		_, _ = fmt.Fprintf(w, "data: %s\n", line)
	}
	_, _ = fmt.Fprint(w, "\n")
}

func marshalPayload(data any) string {
	switch payload := data.(type) {
	case string:
		return payload
	case []byte:
		return string(payload)
	default:
		bytes, err := json.Marshal(payload)
		if err != nil {
			return fmt.Sprintf("%v", data)
		}
		return string(bytes)
	}
}
