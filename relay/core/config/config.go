package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"

	"basegraph.co/relay/core/db"
)

type Config struct {
	Features         Features
	OTel             OTelConfig
	WorkOS           WorkOSConfig
	EventWebhook     EventWebhookConfig
	Pipeline         PipelineConfig
	OpenAI           OpenAIConfig
	PlannerLLM       LLMConfig
	ExploreLLM       LLMConfig
	SpecGeneratorLLM LLMConfig
	ArangoDB         ArangoDBConfig
	Env              string
	Port             string
	DashboardURL     string
	AdminAPIKey      string
	DB               db.Config
}

type WorkOSConfig struct {
	APIKey      string
	ClientID    string
	RedirectURI string
}

type OTelConfig struct {
	Endpoint       string
	Headers        string
	ServiceName    string
	ServiceVersion string
}

type EventWebhookConfig struct {
	BaseURL string
}

type PipelineConfig struct {
	RedisURL        string
	RedisStream     string
	RedisGroup      string
	RedisDLQStream  string
	RedisConsumer   string
	TraceHeaderName string
}

type OpenAIConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

type LLMConfig struct {
	Provider        string // "openai" or "anthropic"
	APIKey          string
	BaseURL         string // Optional: for custom endpoints
	Model           string
	MaxTokens       int
	ReasoningEffort string // Optional: "low", "medium", "high" for reasoning models (gpt-5.1, o1, o3)
}

type ArangoDBConfig struct {
	URL      string
	Username string
	Password string
	Database string
}

type Features struct{}

type ServiceType string

const (
	ServiceTypeServer ServiceType = "server"
	ServiceTypeWorker ServiceType = "worker"
)

// Load loads configuration from environment variables.
// In development, it loads from service-specific .env files:
//   - .env.server for the API server
//   - .env.worker for the background worker
//
// Falls back to .env if service-specific file doesn't exist.
func Load(serviceType ServiceType) (Config, error) {
	if getEnv("RELAY_ENV", "development") == "development" {
		// Try service-specific env file first, fall back to .env
		envFile := fmt.Sprintf(".env.%s", serviceType)
		if err := godotenv.Load(envFile); err != nil {
			_ = godotenv.Load(".env")
		}
	}

	cfg := Config{
		Env:          getEnv("RELAY_ENV", "development"),
		Port:         getEnv("PORT", "8080"),
		DashboardURL: getEnv("DASHBOARD_URL", "http://localhost:3000"),
		AdminAPIKey:  getEnv("ADMIN_API_KEY", ""),
		DB: db.Config{
			DSN:      getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/basegraph?sslmode=disable"),
			MaxConns: getEnvInt32("DB_MAX_CONNS", 10),
			MinConns: getEnvInt32("DB_MIN_CONNS", 2),
		},
		OTel: OTelConfig{
			Endpoint:       getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
			Headers:        getEnv("OTEL_EXPORTER_OTLP_HEADERS", ""),
			ServiceName:    getEnv("OTEL_SERVICE_NAME", "relay"),
			ServiceVersion: getEnv("OTEL_SERVICE_VERSION", "dev"),
		},
		WorkOS: WorkOSConfig{
			APIKey:      getEnv("WORKOS_API_KEY", ""),
			ClientID:    getEnv("WORKOS_CLIENT_ID", ""),
			RedirectURI: getEnv("WORKOS_REDIRECT_URI", "http://localhost:8080/auth/callback"),
		},
		EventWebhook: EventWebhookConfig{
			BaseURL: getEnv("WEBHOOK_BASE_URL", ""),
		},
		Pipeline: PipelineConfig{
			RedisURL:        getEnv("REDIS_URL", "redis://localhost:6379/0"),
			RedisStream:     getEnv("REDIS_STREAM", "relay_events"),
			RedisGroup:      getEnv("REDIS_CONSUMER_GROUP", "relay_group"),
			RedisDLQStream:  getEnv("REDIS_DLQ_STREAM", "relay_events_dlq"),
			RedisConsumer:   getEnv("REDIS_CONSUMER_NAME", "api-server"),
			TraceHeaderName: getEnv("TRACE_HEADER_NAME", "X-Trace-Id"),
		},
		OpenAI: OpenAIConfig{
			APIKey:  getEnv("OPENAI_API_KEY", ""),
			BaseURL: getEnv("OPENAI_BASE_URL", ""),
			Model:   getEnv("OPENAI_MODEL", "gpt-4o-mini"),
		},
		PlannerLLM: LLMConfig{
			Provider:        getEnv("PLANNER_LLM_PROVIDER", "openai"),
			APIKey:          getEnv("PLANNER_LLM_API_KEY", ""),
			BaseURL:         getEnv("PLANNER_LLM_BASE_URL", ""),
			Model:           getEnv("PLANNER_LLM_MODEL", "gpt-5.2"),
			MaxTokens:       getEnvInt("PLANNER_LLM_MAX_TOKENS", 16384),
			ReasoningEffort: getEnv("PLANNER_LLM_REASONING_EFFORT", "medium"),
		},
		ExploreLLM: LLMConfig{
			Provider:        getEnv("EXPLORE_LLM_PROVIDER", "openai"),
			APIKey:          getEnv("EXPLORE_LLM_API_KEY", ""),
			BaseURL:         getEnv("EXPLORE_BASE_URL", ""),
			Model:           getEnv("EXPLORE_LLM_MODEL", "grok-4-1-fast-reasoning"),
			MaxTokens:       getEnvInt("EXPLORE_LLM_MAX_TOKENS", 16384),
			ReasoningEffort: getEnv("EXPLORE_LLM_REASONING_EFFORT", ""),
		},
		// Note: Reusing the planner's config because I'm lazy asf
		SpecGeneratorLLM: LLMConfig{
			Provider:        getEnv("PLANNER_LLM_PROVIDER", "openai"),
			APIKey:          getEnv("PLANNER_LLM_API_KEY", ""),
			BaseURL:         getEnv("PLANNER_LLM_BASE_URL", ""),
			Model:           getEnv("PLANNER_LLM_MODEL", "gpt-5.2"),
			MaxTokens:       getEnvInt("PLANNER_LLM_MAX_TOKENS", 16384),
			ReasoningEffort: getEnv("PLANNER_LLM_REASONING_EFFORT", "medium"),
		},
		ArangoDB: ArangoDBConfig{
			URL:      getEnv("ARANGO_URL", ""),
			Username: getEnv("ARANGO_USERNAME", ""),
			Password: getEnv("ARANGO_PASSWORD", ""),
			Database: getEnv("ARANGO_DATABASE", ""),
		},
		Features: Features{},
	}

	if cfg.EventWebhook.BaseURL == "" {
		return Config{}, fmt.Errorf("WEBHOOK_BASE_URL is required")
	}

	if cfg.WorkOS.APIKey == "" || cfg.WorkOS.ClientID == "" {
		return Config{}, fmt.Errorf("WORKOS_API_KEY and WORKOS_CLIENT_ID are required")
	}

	return cfg, nil
}

func (c Config) IsProduction() bool {
	return c.Env == "production"
}

func (c Config) IsDevelopment() bool {
	return c.Env == "development"
}

func (c OTelConfig) Enabled() bool {
	return c.Endpoint != ""
}

func (c WorkOSConfig) Enabled() bool {
	return c.APIKey != "" && c.ClientID != ""
}

func (c OpenAIConfig) Enabled() bool {
	return c.APIKey != ""
}

func (c LLMConfig) Enabled() bool {
	return c.APIKey != "" && (c.Provider == "openai" || c.Provider == "anthropic")
}

func (c ArangoDBConfig) Enabled() bool {
	return c.URL != "" && c.Username != "" && c.Database != ""
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvInt32(key string, fallback int32) int32 {
	if value, ok := os.LookupEnv(key); ok {
		if i, err := strconv.ParseInt(value, 10, 32); err == nil {
			return int32(i)
		}
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return fallback
}
