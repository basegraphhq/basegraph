.PHONY: build build-server build-worker build-explore run-server run-worker run-worker-testdata run-worker-mock run-explore test test-unit test-bdd test-integration clean sqlc-generate migrate-up migrate-down format lint install-tools reindex-codegraph reset reset-redis reset-db tidy create-fixture

PKGS := $(shell go list ./... | grep -v '/testdata/')

tidy:
	go mod tidy
	go mod vendor

# Build binaries
build: build-server build-worker

build-server:
	go build -o bin/server ./cmd/server

build-worker:
	go build -o bin/worker ./cmd/worker

# Run binaries
run-server:
	go run ./cmd/server

run-worker:
	go run ./cmd/worker

run-worker-testdata:
	REPO_ROOT=$(CURDIR)/testdata/fixtures/relay-without-ack \
	MODULE_PATH=basegraph.co/relay-without-ack \
	go run ./cmd/worker

# Run worker with mock explore for A/B testing planner prompts
# Uses pre-written fixtures instead of real codebase exploration
run-worker-mock:
	REPO_ROOT=$(CURDIR)/testdata/fixtures/relay-without-ack \
	MODULE_PATH=basegraph.co/relay-without-ack \
	MOCK_EXPLORE_FIXTURES=$(CURDIR)/evals/fixtures/explore.json \
	BRAIN_DEBUG_DIR=$(CURDIR)/debug_logs \
	go run ./cmd/worker

# Explore CLI - interactive code exploration
# Reads from .env file automatically
build-explore:
	go build -o bin/explore ./cmd/explore

# Usage: make run-explore
# Loads .env file automatically. Required vars:
#   REPO_ROOT, MODULE_PATH, ANTHROPIC_API_KEY (or OPENAI_API_KEY)
# Optional:
#   ARANGO_URL, ARANGO_USERNAME, ARANGO_PASSWORD, ARANGO_DATABASE (codegraph)
#   BRAIN_DEBUG_DIR (debug logs), THOROUGHNESS (quick/medium/thorough)
run-explore:
	go run ./cmd/explore

# Testing
test: test-unit test-bdd test-integration

test-unit:
	@echo "Running unit tests..."
	@go test -short $(PKGS)

test-bdd:
	@echo "Running BDD tests with Ginkgo..."
	@ginkgo -r --skip-package=vendor --skip-package=testdata

test-integration:
	@echo "Running integration tests..."
	@go test -tags=integration $(PKGS)

# Database operations
sqlc-generate:
	sqlc generate

migrate-up:
	goose -dir migrations postgres "$(DATABASE_URL)" up

migrate-down:
	goose -dir migrations postgres "$(DATABASE_URL)" down

migrate-create:
	goose -dir migrations create $(NAME) sql

# Code quality
format:
	gofumpt -w .

lint:
	golangci-lint run

# Install required tools
install-tools:
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	go install github.com/pressly/goose/v3/cmd/goose@latest
	go install github.com/onsi/ginkgo/v2/ginkgo@latest
	go install mvdan.cc/gofumpt@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Clean
clean:
	rm -rf bin/

# Development helpers
dev-db:
	docker compose -f dev/docker-compose.yaml up -d

dev-down:
	docker compose -f dev/docker-compose.yaml down

# Codegraph operations
reindex-codegraph:
	@./scripts/reindex-codegraph.sh

# Fixture operations
# Usage: NAME=my-fixture make create-fixture
NAME ?=
create-fixture:
ifndef NAME
	$(error NAME is not set. Usage: NAME=my-fixture make create-fixture)
endif
	@./scripts/create-fixture.sh $(NAME)

# Testing reset operations
reset:
	@./scripts/reset.sh

reset-redis:
	@./scripts/reset.sh --redis-only

reset-codegraph:
	@./scripts/reset.sh --codegraph-only

# Environment setup
.env:
	@echo "Creating .env file..."
	@echo "DATABASE_URL=postgres://postgres:postgres@localhost:5432/relay?sslmode=disable" > .env
	@echo "REDIS_URL=redis://localhost:6379/0" >> .env
	@echo "REDIS_STREAM=relay_events" >> .env
	@echo "REDIS_CONSUMER_GROUP=relay_group" >> .env
	@echo "REDIS_CONSUMER=relay_worker_1" >> .env
	@echo "REDIS_DLQ_STREAM=relay_events_dlq" >> .env
	@echo "PORT=8080" >> .env
	@echo "ENV=development" >> .env
