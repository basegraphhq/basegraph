# OpenAI Integration for Relay

This document explains how to use the OpenAI Go SDK integration in Relay.

## Setup

1. **Install the OpenAI SDK dependency** (already done):
   ```bash
   go get github.com/openai/openai-go/v3
   ```

2. **Configure your OpenAI API key** by setting the environment variable:
   ```bash
   export OPENAI_API_KEY=your-api-key-here
   ```

3. **Enable LLM functionality** by ensuring `LLM_ENABLED=true` (default is `true`):
   ```bash
   export LLM_ENABLED=true
   ```

## Configuration Options

The following environment variables control the OpenAI integration:

- `OPENAI_API_KEY`: Your OpenAI API key (required for real LLM calls)
- `OPENAI_MODEL`: The model to use (default: `gpt-4`)
- `OPENAI_MAX_TOKENS`: Maximum tokens per request (default: `2000`)
- `OPENAI_TEMPERATURE`: Temperature for generation (default: `0.7`)
- `LLM_ENABLED`: Enable/disable LLM functionality (default: `true`)

## Usage

### Creating an LLM Client

```go
import "basegraph.app/relay/internal/llm"

// Create client with API key (uses OpenAI if key provided, otherwise mock)
client := llm.NewClient(os.Getenv("OPENAI_API_KEY"))

// Or use configuration from your config
cfg, _ := config.Load()
client := llm.NewClient(cfg.OpenAI.APIKey)
```

### Using the LLM Client

The LLM client provides three main capabilities:

#### 1. Extract Keywords
```go
req := llm.KeywordRequest{
    Issue: issue,           // *domain.Issue
    Text: "issue text",    // string
    Context: domain.ContextSnapshot{
        Keywords: []domain.Keyword{
            {Value: "existing", Weight: 0.8},
        },
    },
}

keywords, err := client.ExtractKeywords(ctx, req)
if err != nil {
    return err
}

for _, kw := range keywords {
    fmt.Printf("Keyword: %s (weight: %.2f)\n", kw.Value, kw.Weight)
}
```

#### 2. Detect Gaps
```go
req := llm.GapRequest{
    Issue: issue,  // *domain.Issue
    Event: event,  // domain.Event
    Context: domain.ContextSnapshot{
        Keywords:     issue.Keywords,
        CodeFindings: issue.CodeFindings,
        Learnings:    issue.Learnings,
        Discussions:  issue.Discussions,
    },
}

analysis, err := client.DetectGaps(ctx, req)
if err != nil {
    return err
}

fmt.Printf("Confidence: %.2f\n", analysis.Confidence)
fmt.Printf("Ready for spec: %v\n", analysis.ReadyForSpec)

for _, gap := range analysis.Gaps {
    fmt.Printf("Gap: %s - %s\n", gap.Category, gap.Summary)
}

for _, question := range analysis.Questions {
    fmt.Printf("Question: %s\n", question.Body)
}
```

#### 3. Generate Spec
```go
req := llm.SpecRequest{
    Issue:   issue,  // *domain.Issue
    Context: context, // domain.ContextSnapshot
    Gaps:    gaps,    // []domain.Gap
}

spec, err := client.GenerateSpec(ctx, req)
if err != nil {
    return err
}

fmt.Println("Generated Specification:")
fmt.Println(spec)
```

## Mock Mode

If no OpenAI API key is provided, the client automatically falls back to a mock implementation that returns sensible defaults for testing:

```go
// Mock client (no API key)
client := llm.NewClient("")

// This will use the mock client and return test data
keywords, _ := client.ExtractKeywords(ctx, req)
```

## Integration with Relay Services

The LLM client is automatically integrated into Relay's service layer:

```go
// In your service or handler
services := service.NewServices(stores, txRunner, workOSCfg, dashboardURL, webhookCfg, eventProducer, llmClient)

// Use gap detector
gapDetector := services.GapDetector()
analysis, err := gapDetector.Detect(ctx, event, issue)

// Use spec generator  
specGen := services.SpecGenerator()
spec, err := specGen.Generate(ctx, issue, context, gaps)
```

## Testing

Run tests with the mock client:
```bash
go test ./internal/llm/...
```

Run integration tests with real OpenAI API:
```bash
export OPENAI_API_KEY=your-api-key
# This will skip if no API key is provided
go test -tags=integration ./internal/llm/...
```

## Error Handling

The LLM client handles various error scenarios:

1. **API Key Missing**: Falls back to mock client
2. **API Errors**: Returns wrapped errors with context
3. **JSON Parsing Failures**: Falls back to simple keyword extraction
4. **Rate Limiting**: Relies on OpenAI SDK's built-in retry logic

## Performance Considerations

- **Token Limits**: Configurable via `OPENAI_MAX_TOKENS`
- **Temperature**: Lower values (0.3-0.5) for more deterministic results
- **Timeout**: Uses context timeout for API calls
- **Caching**: Consider caching responses for similar requests

## Security

- **API Key Storage**: Use environment variables or secure key management
- **Request Filtering**: Sanitize input before sending to OpenAI
- **Response Validation**: Parse and validate JSON responses
- **Rate Limiting**: Monitor API usage and implement client-side rate limiting if needed