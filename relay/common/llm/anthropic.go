package llm

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type anthropicClient struct {
	client anthropic.Client
	model  string
}

// NewAnthropicClient creates an AgentClient using the Anthropic API.
func NewAnthropicClient(cfg Config) (AgentClient, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	model := cfg.Model
	if model == "" {
		model = "claude-sonnet-4-5-20250514"
	}

	return &anthropicClient{
		client: anthropic.NewClient(opts...),
		model:  model,
	}, nil
}

func (c *anthropicClient) ChatWithTools(ctx context.Context, req AgentRequest) (*AgentResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}

	// Extract system message and convert remaining messages
	systemContent, messages := c.convertMessages(req.Messages)
	tools := c.convertTools(req.Tools)

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: int64(maxTokens),
		Messages:  messages,
	}

	if len(systemContent) > 0 {
		params.System = systemContent
	}

	if len(tools) > 0 {
		params.Tools = tools
	}

	if req.Temperature != nil {
		params.Temperature = anthropic.Float(*req.Temperature)
	}

	start := time.Now()
	resp, err := c.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic chat with tools: %w", err)
	}

	slog.DebugContext(ctx, "agent chat completed",
		"model", c.model,
		"duration_ms", time.Since(start).Milliseconds(),
		"input_tokens", resp.Usage.InputTokens,
		"output_tokens", resp.Usage.OutputTokens,
		"stop_reason", resp.StopReason)

	result := &AgentResponse{
		FinishReason:     c.mapStopReason(resp.StopReason),
		PromptTokens:     int(resp.Usage.InputTokens),
		CompletionTokens: int(resp.Usage.OutputTokens),
	}

	// Extract content and tool calls from response
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			result.Content += block.Text
		case "tool_use":
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: string(block.Input),
			})
		}
	}

	return result, nil
}

func (c *anthropicClient) Model() string {
	return c.model
}

// convertMessages extracts system content and converts messages to Anthropic format.
// Anthropic requires system messages to be passed separately, not in the messages array.
func (c *anthropicClient) convertMessages(msgs []Message) ([]anthropic.TextBlockParam, []anthropic.MessageParam) {
	var systemContent []anthropic.TextBlockParam
	messages := make([]anthropic.MessageParam, 0, len(msgs))

	for _, msg := range msgs {
		switch msg.Role {
		case "system":
			systemContent = append(systemContent, anthropic.TextBlockParam{
				Type: "text",
				Text: msg.Content,
			})

		case "user":
			content := []anthropic.ContentBlockParamUnion{
				anthropic.NewTextBlock(msg.Content),
			}
			messages = append(messages, anthropic.MessageParam{
				Role:    anthropic.MessageParamRoleUser,
				Content: content,
			})

		case "assistant":
			var content []anthropic.ContentBlockParamUnion

			// Add text content if present
			if msg.Content != "" {
				content = append(content, anthropic.NewTextBlock(msg.Content))
			}

			// Add tool use blocks for any tool calls
			for _, tc := range msg.ToolCalls {
				content = append(content, anthropic.ContentBlockParamUnion{
					OfToolUse: &anthropic.ToolUseBlockParam{
						Type:  "tool_use",
						ID:    tc.ID,
						Name:  tc.Name,
						Input: []byte(tc.Arguments),
					},
				})
			}

			messages = append(messages, anthropic.MessageParam{
				Role:    anthropic.MessageParamRoleAssistant,
				Content: content,
			})

		case "tool":
			// Tool results in Anthropic are user messages with tool_result content blocks
			content := []anthropic.ContentBlockParamUnion{
				anthropic.NewToolResultBlock(msg.ToolCallID, msg.Content, false),
			}
			messages = append(messages, anthropic.MessageParam{
				Role:    anthropic.MessageParamRoleUser,
				Content: content,
			})
		}
	}

	return systemContent, messages
}

func (c *anthropicClient) convertTools(tools []Tool) []anthropic.ToolUnionParam {
	result := make([]anthropic.ToolUnionParam, len(tools))

	for i, t := range tools {
		// Convert parameters to InputSchema
		inputSchema := anthropic.ToolInputSchemaParam{
			Type: "object",
		}

		if t.Parameters != nil {
			inputSchema.Properties = t.Parameters
		}

		result[i] = anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name,
				Description: anthropic.String(t.Description),
				InputSchema: inputSchema,
			},
		}
	}

	return result
}

func (c *anthropicClient) mapStopReason(reason anthropic.StopReason) string {
	switch reason {
	case anthropic.StopReasonEndTurn:
		return "stop"
	case anthropic.StopReasonToolUse:
		return "tool_calls"
	case anthropic.StopReasonMaxTokens:
		return "length"
	case anthropic.StopReasonStopSequence:
		return "stop"
	default:
		return string(reason)
	}
}
