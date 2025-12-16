package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"

	"basegraph.app/relay/core/db"
)

type Config struct {
	Features     Features
	OTel         OTelConfig
	WorkOS       WorkOSConfig
	EventWebhook EventWebhookConfig
	Pipeline     PipelineConfig
	OpenAI       OpenAIConfig
	Env          string
	Port         string
	DashboardURL string
	DB           db.Config
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
	LLMEnabled      bool
}

type Features struct{}

type OpenAIConfig struct {
	APIKey       string
	Model        string
	MaxTokens    int32
	Temperature  float32
	BaseURL      string
	Organization string
}

func Load() (Config, error) {
	if getEnv("RELAY_ENV", "development") == "development" {
		_ = godotenv.Load(".env")
	}

	cfg := Config{
		Env:          getEnv("RELAY_ENV", "development"),
		Port:         getEnv("PORT", "8080"),
		DashboardURL: getEnv("DASHBOARD_URL", "http://localhost:3000"),
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
			LLMEnabled:      getEnvBool("LLM_ENABLED", true),
		},
		OpenAI: OpenAIConfig{
			APIKey:       getEnv("OPENAI_API_KEY", ""),
			Model:        getEnv("OPENAI_MODEL", "gpt-4"),
			MaxTokens:    getEnvInt32("OPENAI_MAX_TOKENS", 2000),
			Temperature:  getEnvFloat32("OPENAI_TEMPERATURE", 0.7),
			BaseURL:      getEnv("OPENAI_BASE_URL", ""),
			Organization: getEnv("OPENAI_ORGANIZATION", ""),
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

func getEnvBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		if value == "" {
			return fallback
		}
		switch value {
		case "1", "true", "TRUE", "True", "yes", "YES", "on", "ON":
			return true
		case "0", "false", "FALSE", "False", "no", "NO", "off", "OFF":
			return false
		}
	}
	return fallback
}

func getEnvFloat32(key string, fallback float32) float32 {
	if value, ok := os.LookupEnv(key); ok {
		if f, err := strconv.ParseFloat(value, 32); err == nil {
			return float32(f)
		}
	}
	return fallback
}
