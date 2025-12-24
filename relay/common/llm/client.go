package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/invopop/jsonschema"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type Client interface {
	Chat(ctx context.Context, req Request, result any) (*Response, error)
	Model() string
}

type Request struct {
	SystemPrompt string
	UserPrompt   string
	SchemaName   string
	Schema       any
	MaxTokens    int
	Temperature  *float64 // nil = model default, explicit 0 = deterministic
}

type Response struct {
	PromptTokens     int
	CompletionTokens int
}

type Config struct {
	APIKey  string
	BaseURL string
	Model   string
}

type client struct {
	openai openai.Client
	model  string
}

func New(cfg Config) (Client, error) {
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
		model = "gpt-4o-mini"
	}

	return &client{
		openai: openai.NewClient(opts...),
		model:  model,
	}, nil
}

func (c *client) Chat(ctx context.Context, req Request, result any) (*Response, error) {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 1000
	}

	schemaParam := openai.ResponseFormatJSONSchemaJSONSchemaParam{
		Name:        req.SchemaName,
		Description: openai.String("Structured response schema"),
		Schema:      req.Schema,
		Strict:      openai.Bool(true),
	}

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(req.SystemPrompt),
		openai.UserMessage(req.UserPrompt),
	}

	params := openai.ChatCompletionNewParams{
		Model:     c.model,
		Messages:  messages,
		MaxTokens: openai.Int(int64(maxTokens)),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: schemaParam,
			},
		},
	}
	if req.Temperature != nil {
		params.Temperature = openai.Float(*req.Temperature)
	}

	start := time.Now()
	resp, err := c.openai.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai chat: %w", err)
	}

	slog.DebugContext(ctx, "llm chat completed",
		"model", c.model,
		"duration_ms", time.Since(start).Milliseconds(),
		"prompt_tokens", resp.Usage.PromptTokens,
		"completion_tokens", resp.Usage.CompletionTokens)

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	content := resp.Choices[0].Message.Content
	if err := json.Unmarshal([]byte(content), result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &Response{
		PromptTokens:     int(resp.Usage.PromptTokens),
		CompletionTokens: int(resp.Usage.CompletionTokens),
	}, nil
}

func (c *client) Model() string {
	return c.model
}

func GenerateSchema[T any]() any {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T
	return reflector.Reflect(v)
}

func Temp(t float64) *float64 {
	return &t
}

func IsRetryable(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		slog.DebugContext(ctx, "llm error not retryable: context cancelled or deadline exceeded")
		return false
	}

	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.StatusCode == 429:
			slog.WarnContext(ctx, "llm rate limited, will retry",
				"status_code", apiErr.StatusCode)
			return true
		case apiErr.StatusCode >= 500:
			slog.WarnContext(ctx, "llm server error, will retry",
				"status_code", apiErr.StatusCode)
			return true
		default:
			slog.ErrorContext(ctx, "llm client error, not retryable",
				"status_code", apiErr.StatusCode,
				"error_type", apiErr.Type,
				"error_code", apiErr.Code)
			return false
		}
	}

	// Network errors (no API response) are generally retryable
	slog.WarnContext(ctx, "llm network error, will retry", "error", err)
	return true
}
