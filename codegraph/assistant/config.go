package assistant

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"

	"github.com/humanbeeng/lepo/prototypes/codegraph/process"
)

const (
	envOpenAIAPIKey  = "OPENAI_API_KEY"
	envOpenAIModel   = "OPENAI_MODEL"
	envOpenAIBaseURL = "OPENAI_BASE_URL"
	envOpenAIIOrg    = "OPENAI_ORG_ID"

	envNeo4jURI      = "NEO4J_URI"
	envNeo4jUser     = "NEO4J_USERNAME"
	envNeo4jPassword = "NEO4J_PASSWORD"
	envNeo4jDatabase = "NEO4J_DATABASE"

	envWorkspaceRoot = "WORKSPACE_ROOT"

	defaultOpenAIModel   = "gpt-4o-mini"
	defaultNeo4jDatabase = "neo4j"
)

// Config captures shared configuration for the assistant runtime.
type Config struct {
	OpenAI        OpenAIConfig
	Neo4j         process.Neo4jConfig
	WorkspaceRoot string
}

// OpenAIConfig stores client configuration for the Chat Completions API.
type OpenAIConfig struct {
	APIKey       string
	Model        string
	BaseURL      string
	Organization string
}

// LoadConfig reads configuration from environment variables, applying
// reasonable defaults and validation to ensure required values are present.
func LoadConfig() (Config, error) {
	err := godotenv.Load("/Users/nithin/.env")
	if err != nil {
		return Config{}, fmt.Errorf("load environment variables: %w", err)
	}
	cfg := Config{}

	cfg.OpenAI = OpenAIConfig{
		APIKey:       strings.TrimSpace(os.Getenv(envOpenAIAPIKey)),
		Model:        strings.TrimSpace(os.Getenv(envOpenAIModel)),
		BaseURL:      strings.TrimSpace(os.Getenv(envOpenAIBaseURL)),
		Organization: strings.TrimSpace(os.Getenv(envOpenAIIOrg)),
	}

	if cfg.OpenAI.APIKey == "" {
		return cfg, fmt.Errorf("%s must be set", envOpenAIAPIKey)
	}
	if cfg.OpenAI.Model == "" {
		cfg.OpenAI.Model = defaultOpenAIModel
	}

	cfg.Neo4j = process.Neo4jConfig{
		URI:      strings.TrimSpace(os.Getenv(envNeo4jURI)),
		Username: strings.TrimSpace(os.Getenv(envNeo4jUser)),
		Password: strings.TrimSpace(os.Getenv(envNeo4jPassword)),
		Database: strings.TrimSpace(os.Getenv(envNeo4jDatabase)),
	}

	if cfg.Neo4j.Database == "" {
		cfg.Neo4j.Database = defaultNeo4jDatabase
	}

	if err := cfg.Neo4j.Validate(); err != nil {
		return cfg, err
	}

	root := strings.TrimSpace(os.Getenv(envWorkspaceRoot))
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return cfg, fmt.Errorf("determine working directory: %w", err)
		}
		root = wd
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return cfg, fmt.Errorf("resolve workspace root: %w", err)
	}

	info, statErr := os.Stat(absRoot)
	switch {
	case statErr != nil && errors.Is(statErr, os.ErrNotExist):
		return cfg, fmt.Errorf("workspace root does not exist: %s", absRoot)
	case statErr != nil:
		return cfg, fmt.Errorf("stat workspace root: %w", statErr)
	case !info.IsDir():
		return cfg, fmt.Errorf("workspace root must be a directory: %s", absRoot)
	}

	cfg.WorkspaceRoot = absRoot
	return cfg, nil
}
