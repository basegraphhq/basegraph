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
