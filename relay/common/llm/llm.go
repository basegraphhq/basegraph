package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/invopop/jsonschema"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

var nameInvalidChars = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// Config holds LLM client configuration.
type Config struct {
	APIKey  string
	BaseURL string
	Model   string
}

// AgentClient supports tool-calling conversations for agent loops.
type AgentClient interface {
	ChatWithTools(ctx context.Context, req AgentRequest) (*AgentResponse, error)
	Model() string
}

// AgentRequest contains the messages and tools for an agent turn.
type AgentRequest struct {
	Messages    []Message
	Tools       []Tool
	MaxTokens   int
	Temperature *float64
}

// Message represents a conversation message.
type Message struct {
	Role       string     // "system", "user", "assistant", "tool"
	Name       string     // Optional: participant name for multi-user conversations (user messages only)
	Content    string     // Text content
	ToolCalls  []ToolCall // For assistant messages that request tool calls
	ToolCallID string     // For tool result messages (references the tool call)
}

// Tool defines a function the LLM can call.
type Tool struct {
	Name        string
	Description string
	Parameters  any // JSON Schema for parameters
}

// ToolCall represents a tool invocation requested by the LLM.
type ToolCall struct {
	ID        string // Unique ID for this call
	Name      string // Tool name
	Arguments string // JSON-encoded arguments
}

// AgentResponse contains the LLM's response.
type AgentResponse struct {
	Content          string     // Text response (when no tool calls)
	ToolCalls        []ToolCall // Tool calls to execute
	FinishReason     string     // "stop", "tool_calls", "length"
	PromptTokens     int
	CompletionTokens int
}

type agentClient struct {
	openai openai.Client
	model  string
}

// NewAgentClient creates an AgentClient for tool-calling conversations.
func NewAgentClient(cfg Config) (AgentClient, error) {
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
		model = "gpt-5-codex"
	}

	return &agentClient{
		openai: openai.NewClient(opts...),
		model:  model,
	}, nil
}

func (c *agentClient) ChatWithTools(ctx context.Context, req AgentRequest) (*AgentResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}

	messages := convertMessages(req.Messages)
	tools := convertTools(req.Tools)

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
	resp, err := c.openai.Chat.Completions.New(ctx, params)
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

	// Extract tool calls if present
	for _, tc := range choice.Message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	return result, nil
}

func (c *agentClient) Model() string {
	return c.model
}

func convertMessages(msgs []Message) []openai.ChatCompletionMessageParamUnion {
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
				// Assistant message with tool calls
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
			// Note: ToolMessage signature is (content, toolCallID), not (id, content)
			result = append(result, openai.ToolMessage(msg.Content, msg.ToolCallID))
		}
	}

	return result
}

func convertTools(tools []Tool) []openai.ChatCompletionToolParam {
	result := make([]openai.ChatCompletionToolParam, len(tools))

	for i, t := range tools {
		// Convert parameters to shared.FunctionParameters (map[string]any)
		var params shared.FunctionParameters
		if t.Parameters != nil {
			// Marshal and unmarshal to convert schema to map
			data, _ := json.Marshal(t.Parameters)
			_ = json.Unmarshal(data, &params)
		}

		result[i] = openai.ChatCompletionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        t.Name,
				Description: openai.String(t.Description),
				Parameters:  params,
				// Note: Strict mode requires ALL properties to be in 'required' array,
				// which doesn't work well with optional parameters. Keep it off for flexibility.
			},
		}
	}

	return result
}

// ParseToolArguments unmarshals tool arguments into the target struct.
func ParseToolArguments[T any](arguments string) (T, error) {
	var result T
	if err := json.Unmarshal([]byte(arguments), &result); err != nil {
		return result, fmt.Errorf("parse tool arguments: %w", err)
	}
	return result, nil
}

// GenerateSchemaFrom generates a JSON schema from an instance value.
// Useful when the type is not known at compile time.
func GenerateSchemaFrom(v any) any {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	return reflector.Reflect(v)
}

// SanitizeName converts a username to a valid OpenAI name parameter.
// The name must match ^[a-zA-Z0-9_-]{1,64}$.
// Invalid characters are replaced with underscores, and the result is truncated to 64 characters.
func SanitizeName(username string) string {
	sanitized := nameInvalidChars.ReplaceAllString(username, "_")
	if len(sanitized) > 64 {
		sanitized = sanitized[:64]
	}
	return sanitized
}
