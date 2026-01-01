package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/invopop/jsonschema"
)

var nameInvalidChars = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// Provider constants for LLM provider selection.
const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"
)

// ReasoningEffort controls the amount of reasoning for supported models.
type ReasoningEffort string

const (
	ReasoningEffortLow    ReasoningEffort = "low"
	ReasoningEffortMedium ReasoningEffort = "medium"
	ReasoningEffortHigh   ReasoningEffort = "high"
)

// Config holds LLM client configuration.
type Config struct {
	Provider        string          // "openai" or "anthropic"
	APIKey          string          // Required: API key for the provider
	BaseURL         string          // Optional: custom API endpoint
	Model           string          // Model name (e.g., "gpt-5.1", "claude-sonnet-4-5-20250514")
	ReasoningEffort ReasoningEffort // Optional: for models that support reasoning (gpt-5.1, o1, o3)
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

// NewAgentClient creates an AgentClient for tool-calling conversations.
// It selects the appropriate provider based on cfg.Provider ("openai" or "anthropic").
// Defaults to Anthropic if no provider is specified.
func NewAgentClient(cfg Config) (AgentClient, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	provider := cfg.Provider
	if provider == "" {
		provider = ProviderAnthropic
	}

	switch provider {
	case ProviderAnthropic:
		return NewAnthropicClient(cfg)
	case ProviderOpenAI:
		return newOpenAIClient(cfg)
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", provider)
	}
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
