.PHONY: build build-server build-worker run-server run-worker run-worker-testdata test test-unit test-bdd test-integration clean sqlc-generate migrate-up migrate-down format lint install-tools reindex-codegraph reset reset-redis reset-db tidy

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
	MODULE_PATH=basegraph.app/relay-without-ack \
	go run ./cmd/worker

# Testing
test: test-unit test-bdd test-integration

test-unit:
	@echo "Running unit tests..."
	@go test -short ./...

test-bdd:
	@echo "Running BDD tests with Ginkgo..."
	@ginkgo -r --skip-package=vendor

test-integration:
	@echo "Running integration tests..."
	@go test -tags=integration ./...

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
