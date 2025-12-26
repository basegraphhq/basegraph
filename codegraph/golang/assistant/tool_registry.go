package assistant

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared/constant"
)

type toolHandler func(ctx context.Context, args json.RawMessage) (string, error)

type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
	Strict      bool
}

func (d ToolDefinition) toToolUnionParam() responses.ToolUnionParam {
	strict := param.NewOpt(d.Strict)
	typeValue := constant.Function("").Default()
	params := make(map[string]any)
	for k, v := range d.Parameters {
		params[k] = v
	}
	if _, ok := params["type"]; !ok {
		params["type"] = "object"
	}
	params["additionalProperties"] = false
	tool := responses.FunctionToolParam{
		Name:        d.Name,
		Parameters:  params,
		Strict:      strict,
		Description: param.NewOpt(d.Description),
		Type:        typeValue,
	}
	return responses.ToolUnionParam{OfFunction: &tool}
}

// ToolRegistry stores the tool definitions and matching handlers.
type ToolRegistry struct {
	definitions []ToolDefinition
	handlers    map[string]toolHandler
	closers     []func(context.Context) error
}

// NewToolRegistry constructs an empty registry ready to accept tool entries.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		handlers: make(map[string]toolHandler),
	}
}

// Add registers a new tool definition and its handler. Tool names must be unique.
func (r *ToolRegistry) Add(def ToolDefinition, handler toolHandler) error {
	if def.Name == "" {
		return fmt.Errorf("tool definition must include function name")
	}
	if _, exists := r.handlers[def.Name]; exists {
		return fmt.Errorf("tool %s already registered", def.Name)
	}
	r.definitions = append(r.definitions, def)
	r.handlers[def.Name] = handler
	return nil
}

// AddCloser registers a shutdown hook that will be invoked when Close is called.
func (r *ToolRegistry) AddCloser(fn func(context.Context) error) {
	if fn == nil {
		return
	}
	r.closers = append(r.closers, fn)
}

// Definitions returns the list of tool definitions to send to OpenAI.
func (r *ToolRegistry) Definitions() []ToolDefinition {
	return append([]ToolDefinition(nil), r.definitions...)
}

// ResponseTools returns the tool definitions encoded for the Responses API.
func (r *ToolRegistry) ResponseTools() []responses.ToolUnionParam {
	tools := make([]responses.ToolUnionParam, 0, len(r.definitions))
	for _, def := range r.definitions {
		tools = append(tools, def.toToolUnionParam())
	}
	return tools
}

// Handle executes the handler registered for the given tool name.
func (r *ToolRegistry) Handle(ctx context.Context, name string, args json.RawMessage) (string, error) {
	handler, ok := r.handlers[name]
	if !ok {
		return "", fmt.Errorf("no handler registered for tool %s", name)
	}
	return handler(ctx, args)
}

// Close invokes registered shutdown hooks in reverse order.
func (r *ToolRegistry) Close(ctx context.Context) error {
	var firstErr error
	for i := len(r.closers) - 1; i >= 0; i-- {
		if err := r.closers[i](ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
