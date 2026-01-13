package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"basegraph.app/relay/common/arangodb"
	"basegraph.app/relay/common/llm"
	"basegraph.app/relay/internal/brain"
	"github.com/joho/godotenv"
)

func main() {
	ctx := context.Background()

	// Load .env file (ignore error if not found)
	_ = godotenv.Load()

	// Repo config - defaults to relay codebase for easy testing
	repoRoot := getEnv("REPO_ROOT", "/Users/nithin/basegraph/relay")
	modulePath := getEnv("MODULE_PATH", "basegraph.app/relay")

	// LLM client - uses same env vars as relay server
	provider := getEnv("LLM_PROVIDER", "openai")
	model := getEnv("LLM_MODEL", "gpt-4o")
	apiKey := os.Getenv("LLM_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "LLM_API_KEY is required")
		os.Exit(1)
	}

	agentClient, err := llm.NewAgentClient(llm.Config{
		Provider: provider,
		APIKey:   apiKey,
		Model:    model,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create LLM client: %v\n", err)
		os.Exit(1)
	}

	// ArangoDB client (optional - uses defaults matching config.go)
	var arangoClient arangodb.Client
	arangoURL := getEnv("ARANGO_URL", "http://localhost:8529")
	arangoClient, err = arangodb.New(ctx, arangodb.Config{
		URL:      arangoURL,
		Username: getEnv("ARANGO_USERNAME", "root"),
		Password: getEnv("ARANGO_PASSWORD", ""),
		Database: getEnv("ARANGO_DATABASE", "codegraph"),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Codegraph: disabled (%v)\n", err)
		arangoClient = nil
	} else {
		if err := arangoClient.EnsureDatabase(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Codegraph: disabled (%v)\n", err)
			arangoClient = nil
		} else {
			fmt.Fprintf(os.Stderr, "Codegraph: connected (%s)\n", arangoURL)
		}
	}

	// Debug dir (optional)
	debugDir := os.Getenv("BRAIN_DEBUG_DIR")
	if debugDir != "" {
		debugDir = brain.SetupDebugRunDir(debugDir)
		fmt.Fprintf(os.Stderr, "Debug logs: %s\n", debugDir)
	}

	// Create explore agent
	tools := brain.NewExploreTools(repoRoot, arangoClient)
	explorer := brain.NewExploreAgent(agentClient, tools, modulePath, debugDir)

	// Mock mode support for A/B testing
	mockFixtureFile := os.Getenv("MOCK_EXPLORE_FIXTURES")
	if mockFixtureFile != "" {
		// Create a cheap LLM for fixture selection (OpenAI gpt-4o-mini)
		mockAPIKey := os.Getenv("OPENAI_API_KEY")
		if mockAPIKey == "" {
			mockAPIKey = apiKey // Fall back to LLM_API_KEY
		}
		mockModel := getEnv("MOCK_EXPLORE_MODEL", "gpt-4o-mini")

		selectorClient, err := llm.NewAgentClient(llm.Config{
			Provider: "openai",
			APIKey:   mockAPIKey,
			Model:    mockModel,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create mock selector LLM: %v\n", err)
			os.Exit(1)
		}

		explorer = explorer.WithMockMode(selectorClient, mockFixtureFile)
		fmt.Fprintf(os.Stderr, "Mock mode: enabled (fixtures=%s, model=%s)\n", mockFixtureFile, mockModel)
	}

	fmt.Fprintf(os.Stderr, "\nExplore CLI ready (repo=%s)\n", repoRoot)
	fmt.Fprintln(os.Stderr, "Enter your question (or 'quit' to exit):")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		query := strings.TrimSpace(scanner.Text())
		if query == "" {
			continue
		}
		if query == "quit" || query == "exit" || query == "q" {
			break
		}

		fmt.Fprintf(os.Stderr, "\nExploring: %s\n", query)
		fmt.Fprintln(os.Stderr, "---")

		report, err := explorer.Explore(ctx, 0, query) // 0 = no issue context (CLI mode, no caching)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			continue
		}

		fmt.Println(report)
		fmt.Println()
	}

	fmt.Fprintln(os.Stderr, "Goodbye!")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
