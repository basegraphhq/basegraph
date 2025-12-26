package assistant

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	// if _, err := newFilesystemTools(registry, cfg.WorkspaceRoot); err != nil {
	// 	return fmt.Errorf("initialise filesystem tools: %w", err)
	// }
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
				writeConversationToFile(conversation, cfg.WorkspaceRoot)
				return fmt.Errorf("read input: %w", err)
			}
			writeConversationToFile(conversation, cfg.WorkspaceRoot)
			return nil
		}
		userInput := strings.TrimSpace(scanner.Text())
		if userInput == "" {
			continue
		}
		if strings.EqualFold(userInput, "exit") || strings.EqualFold(userInput, "quit") {
			fmt.Println("Exiting.")
			writeConversationToFile(conversation, cfg.WorkspaceRoot)
			return nil
		}

		conversation = append(conversation, conversationItem{
			kind:    conversationItemKindMessage,
			role:    "user",
			content: userInput,
		})

		start := time.Now()
		if err := runConversation(ctx, client, registry, &conversation, cfg.OpenAI.Model); err != nil {
			writeConversationToFile(conversation, cfg.WorkspaceRoot)
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

		if len(toolCalls) > 1 {
			fmt.Printf("🚀 Parallel tool calling detected: %d tools will execute concurrently\n", len(toolCalls))
		}

		type toolCallResult struct {
			output   string
			duration time.Duration
			err      error
		}

		results := make([]toolCallResult, len(toolCalls))
		var wg sync.WaitGroup
		var mu sync.Mutex

		for i, call := range toolCalls {
			fmt.Printf("🔧 Calling tool: %s\n", call.Name)
			fmt.Printf("   Input: %s\n", call.Arguments)

			*conversation = append(*conversation, conversationItem{
				kind:         conversationItemKindFunctionCall,
				functionName: call.Name,
				arguments:    call.Arguments,
				callID:       call.CallID,
			})

			wg.Add(1)
			go func(idx int, call toolCall) {
				defer wg.Done()

				toolStart := time.Now()
				output, err := registry.Handle(ctx, call.Name, json.RawMessage(call.Arguments))
				duration := time.Since(toolStart)
				if err != nil {
					slog.Error("tool execution failed", "tool", call.Name, "err", err)
					output = fmt.Sprintf(`{"error": %q}`, err.Error())
				}

				mu.Lock()
				results[idx] = toolCallResult{
					output:   output,
					duration: duration,
					err:      err,
				}
				mu.Unlock()
			}(i, call)
		}

		wg.Wait()

		var toolErr error
		for i, call := range toolCalls {
			result := results[i]
			fmt.Printf("⏱️  Tool execution time (%s): %v\n", call.Name, result.duration)
			if result.err != nil && toolErr == nil {
				toolErr = result.err
			}

			*conversation = append(*conversation, conversationItem{
				kind:    conversationItemKindFunctionOutput,
				callID:  call.CallID,
				content: result.output,
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
		Model:             shared.ResponsesModel(model),
		Input:             responses.ResponseNewParamsInputUnion{OfInputItemList: inputItems},
		Tools:             registry.ResponseTools(),
		ParallelToolCalls: param.NewOpt(true),
		Reasoning: shared.ReasoningParam{
			Effort: shared.ReasoningEffortMedium,
		},
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

// conversationItemJSON is a JSON-serializable representation of conversationItem
type conversationItemJSON struct {
	Kind         string `json:"kind"`
	Role         string `json:"role,omitempty"`
	Content      string `json:"content,omitempty"`
	FunctionName string `json:"function_name,omitempty"`
	Arguments    string `json:"arguments,omitempty"`
	CallID       string `json:"call_id,omitempty"`
}

// writeConversationToFile writes the conversation thread to a JSON file with a timestamp.
func writeConversationToFile(conversation []conversationItem, workspaceRoot string) {
	if len(conversation) <= 1 {
		// Only system message, no actual conversation
		return
	}

	// Create conversations directory in workspace root
	conversationsDir := filepath.Join(workspaceRoot, ".conversations")
	if err := os.MkdirAll(conversationsDir, 0o755); err != nil {
		slog.Error("failed to create conversations directory", "dir", conversationsDir, "err", err)
		return
	}

	// Generate filename with timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("conversation_%s.json", timestamp)
	filePath := filepath.Join(conversationsDir, filename)

	// Convert conversation items to JSON-serializable format
	items := make([]conversationItemJSON, 0, len(conversation))
	for _, item := range conversation {
		jsonItem := conversationItemJSON{
			Role:    item.role,
			Content: item.content,
		}

		switch item.kind {
		case conversationItemKindMessage:
			jsonItem.Kind = "message"
		case conversationItemKindFunctionCall:
			jsonItem.Kind = "function_call"
			jsonItem.FunctionName = item.functionName
			jsonItem.Arguments = item.arguments
			jsonItem.CallID = item.callID
		case conversationItemKindFunctionOutput:
			jsonItem.Kind = "function_output"
			jsonItem.CallID = item.callID
		}

		items = append(items, jsonItem)
	}

	// Create output structure with metadata
	output := map[string]any{
		"timestamp":    time.Now().Format(time.RFC3339),
		"total_items":  len(items),
		"conversation": items,
	}

	// Marshal to JSON with indentation
	jsonData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		slog.Error("failed to marshal conversation to JSON", "err", err)
		return
	}

	// Write to file
	if err := os.WriteFile(filePath, jsonData, 0o644); err != nil {
		slog.Error("failed to write conversation to file", "file", filePath, "err", err)
		return
	}

	slog.Info("conversation saved", "file", filePath, "items", len(items))
}
