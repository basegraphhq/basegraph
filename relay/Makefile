.PHONY: build run install-tools migrate-up migrate-down migrate-status migrate-create

# Install tools globally (optional - tools work without this via 'go tool')
install-tools:
	@echo "Installing tools from go.mod..."
	@go install tool
	@echo "Tools installed successfully"

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

# Database migration commands (uses Go 1.24+ 'go tool' - no global install needed)
# Usage: make migrate-up DB_DRIVER=postgres DB_STRING="postgres://user:pass@localhost/dbname"
migrate-up:
	@if [ -z "$(DB_DRIVER)" ] || [ -z "$(DB_STRING)" ]; then \
		echo "Usage: make migrate-up DB_DRIVER=<driver> DB_STRING=<connection_string>"; \
		echo "Example: make migrate-up DB_DRIVER=postgres DB_STRING='postgres://user:pass@localhost/dbname'"; \
		exit 1; \
	fi
	go tool goose -dir migrations $(DB_DRIVER) $(DB_STRING) up

migrate-down:
	@if [ -z "$(DB_DRIVER)" ] || [ -z "$(DB_STRING)" ]; then \
		echo "Usage: make migrate-down DB_DRIVER=<driver> DB_STRING=<connection_string>"; \
		exit 1; \
	fi
	go tool goose -dir migrations $(DB_DRIVER) $(DB_STRING) down

migrate-status:
	@if [ -z "$(DB_DRIVER)" ] || [ -z "$(DB_STRING)" ]; then \
		echo "Usage: make migrate-status DB_DRIVER=<driver> DB_STRING=<connection_string>"; \
		exit 1; \
	fi
	go tool goose -dir migrations $(DB_DRIVER) $(DB_STRING) status

migrate-create:
	@if [ -z "$(NAME)" ]; then \
		echo "Usage: make migrate-create NAME=<migration_name>"; \
		echo "Example: make migrate-create NAME=create_users_table"; \
		exit 1; \
	fi
	@mkdir -p migrations
	go tool goose -dir migrations create $(NAME) sql
