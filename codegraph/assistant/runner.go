package assistant

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

type conversationItemKind int

const (
	conversationItemKindMessage conversationItemKind = iota
	conversationItemKindFunctionCall
	conversationItemKindFunctionOutput
)

type conversationItem struct {
	kind         conversationItemKind
	role         string
	content      string
	functionName string
	arguments    string
	callID       string
}

type toolCall struct {
	CallID    string
	Name      string
	Arguments string
}

// Run starts the interactive assistant loop and blocks until the context is
// cancelled or the user exits.
func Run(ctx context.Context, cfg Config) error {
	registry := NewToolRegistry()
	if _, err := newFilesystemTools(registry, cfg.WorkspaceRoot); err != nil {
		return fmt.Errorf("initialise filesystem tools: %w", err)
	}
	if _, err := newCodeGraphTools(ctx, registry, cfg.Neo4j); err != nil {
		return fmt.Errorf("initialise code graph tools: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := registry.Close(closeCtx); err != nil {
			slog.Error("failed to close tool registry", "err", err)
		}
	}()

	opts := []option.RequestOption{option.WithAPIKey(cfg.OpenAI.APIKey)}
	if cfg.OpenAI.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.OpenAI.BaseURL))
	}
	if cfg.OpenAI.Organization != "" {
		opts = append(opts, option.WithOrganization(cfg.OpenAI.Organization))
	}
	client := openai.NewClient(opts...)

	conversation := []conversationItem{
		{kind: conversationItemKindMessage, role: "system", content: systemPrompt},
		{kind: conversationItemKindMessage, role: "developer", content: developerPrompt},
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	fmt.Println("Relay is ready. Codegraph is online. Type 'exit' to quit.")
	fmt.Println("Using OpenAI Model: ", cfg.OpenAI.Model)
	fmt.Println("Using Neo4j Database: ", cfg.Neo4j.Database)

	for {
		fmt.Print("» ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("read input: %w", err)
			}
			return nil
		}
		userInput := strings.TrimSpace(scanner.Text())
		if userInput == "" {
			continue
		}
		if strings.EqualFold(userInput, "exit") || strings.EqualFold(userInput, "quit") {
			fmt.Println("Exiting.")
			return nil
		}

		conversation = append(conversation, conversationItem{
			kind:    conversationItemKindMessage,
			role:    "user",
			content: userInput,
		})

		start := time.Now()
		if err := runConversation(ctx, client, registry, &conversation, cfg.OpenAI.Model); err != nil {
			return err
		}
		duration := time.Since(start)
		fmt.Printf("⏱️  Response time: %v\n", duration)
	}
}

func runConversation(ctx context.Context, client openai.Client, registry *ToolRegistry, conversation *[]conversationItem, model string) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		resp, err := createResponse(ctx, client, registry, *conversation, model)
		if err != nil {
			return fmt.Errorf("create response: %w", err)
		}

		assistantText, toolCalls, err := parseResponse(resp)
		if err != nil {
			return err
		}

		if assistantText != "" {
			fmt.Println(strings.TrimSpace(assistantText))
			*conversation = append(*conversation, conversationItem{
				kind:    conversationItemKindMessage,
				role:    "assistant",
				content: assistantText,
			})
		}

		if len(toolCalls) == 0 {
			return nil
		}

		var toolErr error
		for _, call := range toolCalls {
			fmt.Printf("🔧 Calling tool: %s\n", call.Name)
			fmt.Printf("   Input: %s\n", call.Arguments)

			*conversation = append(*conversation, conversationItem{
				kind:         conversationItemKindFunctionCall,
				functionName: call.Name,
				arguments:    call.Arguments,
				callID:       call.CallID,
			})

			output, err := registry.Handle(ctx, call.Name, json.RawMessage(call.Arguments))
			if err != nil {
				slog.Error("tool execution failed", "tool", call.Name, "err", err)
				if toolErr == nil {
					toolErr = err
				}
				output = fmt.Sprintf(`{"error": %q}`, err.Error())
			}

			*conversation = append(*conversation, conversationItem{
				kind:    conversationItemKindFunctionOutput,
				callID:  call.CallID,
				content: output,
			})
		}

		if toolErr != nil {
			fmt.Println("⚠️ Tool call failed, but continuing the conversation.")
			continue
		}
	}
}

func createResponse(ctx context.Context, client openai.Client, registry *ToolRegistry, conversation []conversationItem, model string) (*responses.Response, error) {
	inputItems := buildInputItems(conversation)
	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(model),
		Input: responses.ResponseNewParamsInputUnion{OfInputItemList: inputItems},
		Tools: registry.ResponseTools(),
	}
	params.ToolChoice.OfToolChoiceMode = param.NewOpt(responses.ToolChoiceOptionsAuto)
	resp, err := client.Responses.New(ctx, params)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("nil response from OpenAI")
	}
	return resp, nil
}

func buildInputItems(conversation []conversationItem) responses.ResponseInputParam {
	items := make(responses.ResponseInputParam, 0, len(conversation))
	for _, item := range conversation {
		switch item.kind {
		case conversationItemKindMessage:
			role := responses.EasyInputMessageRole(item.role)
			items = append(items, responses.ResponseInputItemParamOfMessage(item.content, role))
		case conversationItemKindFunctionCall:
			items = append(items, responses.ResponseInputItemParamOfFunctionCall(item.arguments, item.callID, item.functionName))
		case conversationItemKindFunctionOutput:
			items = append(items, responses.ResponseInputItemParamOfFunctionCallOutput(item.callID, item.content))
		}
	}
	return items
}

func parseResponse(resp *responses.Response) (string, []toolCall, error) {
	if resp.Error.Message != "" {
		return "", nil, fmt.Errorf("openai response error: %s (code=%s)", resp.Error.Message, resp.Error.Code)
	}

	var textBuilder strings.Builder
	toolCalls := make([]toolCall, 0)

	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			msg := item.AsMessage()
			segment := extractMessageText(msg)
			if segment != "" {
				if textBuilder.Len() > 0 {
					textBuilder.WriteString("\n")
				}
				textBuilder.WriteString(segment)
			}
		case "function_call":
			call := item.AsFunctionCall()
			toolCalls = append(toolCalls, toolCall{
				CallID:    call.CallID,
				Name:      call.Name,
				Arguments: call.Arguments,
			})
		}
	}

	return strings.TrimSpace(textBuilder.String()), toolCalls, nil
}

func extractMessageText(msg responses.ResponseOutputMessage) string {
	var builder strings.Builder
	for _, content := range msg.Content {
		switch content.Type {
		case "output_text":
			builder.WriteString(content.AsOutputText().Text)
		case "refusal":
			builder.WriteString(content.AsRefusal().Refusal)
		}
	}
	return strings.TrimSpace(builder.String())
}
