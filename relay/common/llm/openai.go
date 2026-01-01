package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

type openaiClient struct {
	client openai.Client
	model  string
}

// newOpenAIClient creates an AgentClient using the OpenAI API.
func newOpenAIClient(cfg Config) (AgentClient, error) {
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	model := cfg.Model
	if model == "" {
		model = "gpt-4o"
	}

	return &openaiClient{
		client: openai.NewClient(opts...),
		model:  model,
	}, nil
}

func (c *openaiClient) ChatWithTools(ctx context.Context, req AgentRequest) (*AgentResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}

	messages := c.convertMessages(req.Messages)
	tools := c.convertTools(req.Tools)

	params := openai.ChatCompletionNewParams{
		Model:               c.model,
		Messages:            messages,
		MaxCompletionTokens: openai.Int(int64(maxTokens)),
	}

	if len(tools) > 0 {
		params.Tools = tools
	}

	if req.Temperature != nil {
		params.Temperature = openai.Float(*req.Temperature)
	}

	start := time.Now()
	resp, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai chat with tools: %w", err)
	}

	slog.DebugContext(ctx, "agent chat completed",
		"model", c.model,
		"duration_ms", time.Since(start).Milliseconds(),
		"prompt_tokens", resp.Usage.PromptTokens,
		"completion_tokens", resp.Usage.CompletionTokens,
		"finish_reason", resp.Choices[0].FinishReason)

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := resp.Choices[0]
	result := &AgentResponse{
		Content:          choice.Message.Content,
		FinishReason:     string(choice.FinishReason),
		PromptTokens:     int(resp.Usage.PromptTokens),
		CompletionTokens: int(resp.Usage.CompletionTokens),
	}

	for _, tc := range choice.Message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	return result, nil
}

func (c *openaiClient) Model() string {
	return c.model
}

func (c *openaiClient) convertMessages(msgs []Message) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(msgs))

	for _, msg := range msgs {
		switch msg.Role {
		case "system":
			result = append(result, openai.SystemMessage(msg.Content))

		case "user":
			if msg.Name != "" {
				result = append(result, openai.ChatCompletionMessageParamUnion{
					OfUser: &openai.ChatCompletionUserMessageParam{
						Name: openai.String(msg.Name),
						Content: openai.ChatCompletionUserMessageParamContentUnion{
							OfString: openai.String(msg.Content),
						},
					},
				})
			} else {
				result = append(result, openai.UserMessage(msg.Content))
			}

		case "assistant":
			if len(msg.ToolCalls) > 0 {
				toolCalls := make([]openai.ChatCompletionMessageToolCallParam, len(msg.ToolCalls))
				for i, tc := range msg.ToolCalls {
					toolCalls[i] = openai.ChatCompletionMessageToolCallParam{
						ID:   tc.ID,
						Type: "function",
						Function: openai.ChatCompletionMessageToolCallFunctionParam{
							Name:      tc.Name,
							Arguments: tc.Arguments,
						},
					}
				}
				result = append(result, openai.ChatCompletionMessageParamUnion{
					OfAssistant: &openai.ChatCompletionAssistantMessageParam{
						Content:   openai.ChatCompletionAssistantMessageParamContentUnion{OfString: openai.String(msg.Content)},
						ToolCalls: toolCalls,
					},
				})
			} else {
				result = append(result, openai.AssistantMessage(msg.Content))
			}

		case "tool":
			result = append(result, openai.ToolMessage(msg.Content, msg.ToolCallID))
		}
	}

	return result
}

func (c *openaiClient) convertTools(tools []Tool) []openai.ChatCompletionToolParam {
	result := make([]openai.ChatCompletionToolParam, len(tools))

	for i, t := range tools {
		var params shared.FunctionParameters
		if t.Parameters != nil {
			data, _ := json.Marshal(t.Parameters)
			_ = json.Unmarshal(data, &params)
		}

		result[i] = openai.ChatCompletionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        t.Name,
				Description: openai.String(t.Description),
				Parameters:  params,
			},
		}
	}

	return result
}
