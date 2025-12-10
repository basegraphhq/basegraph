package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"

	"basegraph.app/relay/core/db"
)

type Config struct {
	Features Features
	OTel     OTelConfig
	Env      string
	Port     string
	DB       db.Config
}

type OTelConfig struct {
	Endpoint       string
	Headers        string
	ServiceName    string
	ServiceVersion string
}

type Features struct{}

func Load() Config {
	if getEnv("RELAY_ENV", "development") == "development" {
		_ = godotenv.Load(".env")
	}

	return Config{
		Env:  getEnv("RELAY_ENV", "development"),
		Port: getEnv("PORT", "8080"),
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
		Features: Features{},
	}
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
