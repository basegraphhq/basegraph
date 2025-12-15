.PHONY: build run tidy format sqlc-generate sqlc-verify migrate-up migrate-down migrate-status migrate-create test test-unit test-verbose test-pkg test-focus lint lint-fix lint-custom lint-all check-goose check-ginkgo check-golangci-lint

ENUMVALIDATOR_BIN := $(abspath bin/enumvalidator)

build:
	go build -o bin/server cmd/server/main.go

$(ENUMVALIDATOR_BIN):
	@mkdir -p $(dir $(ENUMVALIDATOR_BIN))
	go build -o $(ENUMVALIDATOR_BIN) ./tools/linters/enumvalidator/cmd

run: build
	bin/server

# Tidy up go.mod and go.sum
tidy:
	go mod tidy
	go mod vendor

# Format all Go files (excluding generated code) using gofumpt
format:
	@echo "Formatting Go files (excluding generated code)..."
	@find . -name '*.go' -not -path './core/db/sqlc/*' -not -path './vendor/*' -exec go tool gofumpt -w {} +
	@echo "Formatting complete"

# ============================================================================
# Code Generation
# ============================================================================

# Generate Go code from SQL queries using sqlc
sqlc-generate:
	go tool sqlc generate

# Verify sqlc queries without generating (useful for CI)
sqlc-verify:
	go tool sqlc compile

# ============================================================================
# Database Migrations
# ============================================================================

# Check if goose is available via go tool
check-goose:
	@go tool goose --version >/dev/null 2>&1

# Usage: make migrate-up DB_STRING="postgres://user:pass@localhost/dbname?sslmode=disable"
migrate-up: check-goose
	@if [ -z "$(DB_STRING)" ]; then \
		echo "Usage: make migrate-up DB_STRING=<connection_string>"; \
		echo "Example: make migrate-up DB_STRING='postgres://user:pass@localhost/dbname?sslmode=disable'"; \
		exit 1; \
	fi
	go tool goose -dir migrations postgres "$(DB_STRING)" up

migrate-down: check-goose
	@if [ -z "$(DB_STRING)" ]; then \
		echo "Usage: make migrate-down DB_STRING=<connection_string>"; \
		exit 1; \
	fi
	go tool goose -dir migrations postgres "$(DB_STRING)" down

migrate-status: check-goose
	@if [ -z "$(DB_STRING)" ]; then \
		echo "Usage: make migrate-status DB_STRING=<connection_string>"; \
		exit 1; \
	fi
	go tool goose -dir migrations postgres "$(DB_STRING)" status

migrate-create: check-goose
	@if [ -z "$(NAME)" ]; then \
		echo "Usage: make migrate-create NAME=<migration_name>"; \
		echo "Example: make migrate-create NAME=create_users_table"; \
		exit 1; \
	fi
	@mkdir -p migrations
	go tool goose -dir migrations create $(NAME) sql

# ============================================================================
# Testing
# ============================================================================

# Check if ginkgo is available via go tool
check-ginkgo:
	@go tool ginkgo version >/dev/null 2>&1

# Run all tests (unit tests + BDD tests with Ginkgo)
test: check-ginkgo
	@echo "Running unit tests..."
	@go test ./common/...
	@echo "Running BDD tests..."
	@go tool ginkgo ./...

# Run tests with verbose output
test-verbose: check-ginkgo
	go tool ginkgo -v ./...

# Run only utility/common tests (standard Go tests, no Ginkgo needed)
test-unit:
	go test ./common/...

# Run tests for a specific package
# Usage: make test-pkg PKG=./internal/service/...
test-pkg: check-ginkgo
	@if [ -z "$(PKG)" ]; then \
		echo "Usage: make test-pkg PKG=<package_path>"; \
		echo "Example: make test-pkg PKG=./internal/service/..."; \
		exit 1; \
	fi
	go tool ginkgo -v $(PKG)

# Run tests matching a specific pattern
# Usage: make test-focus FOCUS="UserService"
test-focus: check-ginkgo
	@if [ -z "$(FOCUS)" ]; then \
		echo "Usage: make test-focus FOCUS=<pattern>"; \
		echo "Example: make test-focus FOCUS='UserService'"; \
		exit 1; \
	fi
	go tool ginkgo --focus "$(FOCUS)" ./...

# ============================================================================
# Linting
# ============================================================================

# Check if golangci-lint is available via go tool
check-golangci-lint:
	@go tool golangci-lint --version >/dev/null 2>&1

# Run golangci-lint
lint: check-golangci-lint
	@echo "Running golangci-lint..."
	go tool golangci-lint run ./...
	@$(MAKE) lint-custom

# Run golangci-lint with auto-fix
lint-fix: check-golangci-lint
	@echo "Running golangci-lint with auto-fix..."
	go tool golangci-lint run --fix ./...

# Run custom analyzers separately (avoids plugin compatibility issues)
lint-custom: $(ENUMVALIDATOR_BIN)
	@echo "Running custom enum validator..."
	@go vet -vettool=$(ENUMVALIDATOR_BIN) ./...

# Run all linters including custom ones
lint-all: lint
