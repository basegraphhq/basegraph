.PHONY: build run tidy format install-tools install-goose install-sqlc install-gofumpt sqlc-generate migrate-up migrate-down migrate-status migrate-create

# Tool versions
GOOSE_VERSION := v3.24.3
SQLC_VERSION := v1.30.0
GOFUMPT_VERSION := latest

# Build the relay service
build:
	go build -o bin/relay cmd/relay/main.go

# Run the relay service
run: build
	bin/relay

# Tidy up go.mod and go.sum
tidy:
	go mod tidy
	go mod vendor

# Format all Go files (excluding generated code) using gofumpt
format:
	@command -v gofumpt >/dev/null 2>&1 || { \
		echo "Error: gofumpt not found. Run 'make install-gofumpt' first."; \
		exit 1; \
	}
	@echo "Formatting Go files (excluding generated code)..."
	@find . -name '*.go' -not -path './core/db/sqlc/*' -not -path './vendor/*' -exec gofumpt -w {} +
	@echo "Formatting complete"

# ============================================================================
# Tool Installation
# ============================================================================

# Install all CLI tools
install-tools: install-goose install-sqlc install-gofumpt
	@echo "All tools installed successfully"

# Install goose CLI with PostgreSQL-only support (excludes other DB drivers)
install-goose:
	@echo "Installing goose $(GOOSE_VERSION) with PostgreSQL-only support..."
	@go install -tags='no_clickhouse no_libsql no_mssql no_mysql no_sqlite3 no_vertica no_ydb' \
		github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
	@echo "Goose installed to $$(go env GOPATH)/bin/goose"

# Install sqlc for generating type-safe Go code from SQL
install-sqlc:
	@echo "Installing sqlc $(SQLC_VERSION)..."
	@go install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)
	@echo "sqlc installed to $$(go env GOPATH)/bin/sqlc"

# Install gofumpt for stricter Go formatting
install-gofumpt:
	@echo "Installing gofumpt $(GOFUMPT_VERSION)..."
	@go install mvdan.cc/gofumpt@$(GOFUMPT_VERSION)
	@echo "gofumpt installed to $$(go env GOPATH)/bin/gofumpt"

# ============================================================================
# Code Generation
# ============================================================================

# Generate Go code from SQL queries using sqlc
sqlc-generate:
	@command -v sqlc >/dev/null 2>&1 || { \
		echo "Error: sqlc not found. Run 'make install-sqlc' first."; \
		exit 1; \
	}
	sqlc generate

# Verify sqlc queries without generating (useful for CI)
sqlc-verify:
	@command -v sqlc >/dev/null 2>&1 || { \
		echo "Error: sqlc not found. Run 'make install-sqlc' first."; \
		exit 1; \
	}
	sqlc compile

# ============================================================================
# Database Migrations
# ============================================================================

# Check if goose is installed
check-goose:
	@command -v goose >/dev/null 2>&1 || { \
		echo "Error: goose not found. Run 'make install-goose' first."; \
		exit 1; \
	}

# Usage: make migrate-up DB_STRING="postgres://user:pass@localhost/dbname?sslmode=disable"
migrate-up: check-goose
	@if [ -z "$(DB_STRING)" ]; then \
		echo "Usage: make migrate-up DB_STRING=<connection_string>"; \
		echo "Example: make migrate-up DB_STRING='postgres://user:pass@localhost/dbname?sslmode=disable'"; \
		exit 1; \
	fi
	goose -dir migrations postgres "$(DB_STRING)" up

migrate-down: check-goose
	@if [ -z "$(DB_STRING)" ]; then \
		echo "Usage: make migrate-down DB_STRING=<connection_string>"; \
		exit 1; \
	fi
	goose -dir migrations postgres "$(DB_STRING)" down

migrate-status: check-goose
	@if [ -z "$(DB_STRING)" ]; then \
		echo "Usage: make migrate-status DB_STRING=<connection_string>"; \
		exit 1; \
	fi
	goose -dir migrations postgres "$(DB_STRING)" status

migrate-create: check-goose
	@if [ -z "$(NAME)" ]; then \
		echo "Usage: make migrate-create NAME=<migration_name>"; \
		echo "Example: make migrate-create NAME=create_users_table"; \
		exit 1; \
	fi
	@mkdir -p migrations
	goose -dir migrations create $(NAME) sql
