package config

import (
	"fmt"
	"os"
	"strconv"

	"basegraph.app/relay/core/db"
)

// Config holds all application configuration.
type Config struct {
	// Env is the environment name (development, staging, production)
	Env string

	// Port is the HTTP server port
	Port string

	// DB holds database configuration
	DB db.Config
}

// Load loads configuration from environment variables.
// It provides sensible defaults for development.
func Load() Config {
	return Config{
		Env:  getEnv("RELAY_ENV", "development"),
		Port: getEnv("PORT", "8080"),
		DB: db.Config{
			DSN:      buildDSN(),
			MaxConns: int32(getEnvInt("DB_MAX_CONNS", 10)),
			MinConns: int32(getEnvInt("DB_MIN_CONNS", 2)),
		},
	}
}

// buildDSN constructs the database connection string from individual env vars.
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

// IsProduction returns true if running in production environment.
func (c Config) IsProduction() bool {
	return c.Env == "production"
}

// IsDevelopment returns true if running in development environment.
func (c Config) IsDevelopment() bool {
	return c.Env == "development"
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
