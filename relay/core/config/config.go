package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"

	"basegraph.app/relay/core/db"
)

type Config struct {
	Env      string
	Port     string
	DB       db.Config
	OTel     OTelConfig
	Features Features
}

type OTelConfig struct {
	Endpoint       string
	Headers        string
	ServiceName    string
	ServiceVersion string
}

type Features struct{}

func Load() Config {
	// if getEnv("RELAY_ENV", "development") == "development" {
	_ = godotenv.Load(".env")
	// }

	return Config{
		Env:  getEnv("RELAY_ENV", "development"),
		Port: getEnv("PORT", "8080"),
		DB: db.Config{
			DSN:      buildDSN(),
			MaxConns: int32(getEnvInt("DB_MAX_CONNS", 10)),
			MinConns: int32(getEnvInt("DB_MIN_CONNS", 2)),
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

func buildDSN() string {
	host := getEnv("DATABASE_HOST", "localhost")
	port := getEnv("DATABASE_PORT", "5432")
	user := getEnv("DATABASE_USER", "postgres")
	password := getEnv("DATABASE_PASSWORD", "postgres")
	name := getEnv("DATABASE_NAME", "basegraph")
	sslMode := getEnv("DATABASE_SSLMODE", "disable")

	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		user, password, host, port, name, sslMode,
	)
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

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		return value == "true" || value == "1"
	}
	return fallback
}
