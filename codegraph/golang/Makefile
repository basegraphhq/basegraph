build-relay:
	@VERSION=$$(date +"%Y-%m-%dT%H:%M:%S"); \
	go build -ldflags "-X main.version=$$VERSION" -o ~/go/bin/relay cmd/relay/main.go; \
	echo "Built relay version: $$VERSION"

run-relay: build-relay
	bin/relay

build-codegraph:
	go build -o bin/codegraph cmd/codegraph/main.go

run-codegraph: build-codegraph
	bin/codegraph

# Index a fixture for agent testing
# Usage: FIXTURE=relay-without-ack make index-fixture
# Fixtures must be complete Go modules located in: ../relay/testdata/fixtures/<FIXTURE>/
FIXTURE ?=
FIXTURES_DIR ?= $(CURDIR)/../../relay/testdata/fixtures

index-fixture: build-codegraph
ifndef FIXTURE
	$(error FIXTURE is not set. Usage: FIXTURE=relay-without-ack make index-fixture)
endif
	@if [ ! -d "$(FIXTURES_DIR)/$(FIXTURE)" ]; then \
		echo "Error: Fixture '$(FIXTURE)' not found at $(FIXTURES_DIR)/$(FIXTURE)"; \
		exit 1; \
	fi
	@if [ ! -f "$(FIXTURES_DIR)/$(FIXTURE)/go.mod" ]; then \
		echo "Error: Fixture '$(FIXTURE)' is missing go.mod - not a valid Go module"; \
		exit 1; \
	fi
	@echo "Indexing fixture: $(FIXTURE)"
	@echo "Path: $(FIXTURES_DIR)/$(FIXTURE)"
	TARGET_REPO_PATH=$(FIXTURES_DIR)/$(FIXTURE) TARGET_MODULE= bin/codegraph

test:
	go test -v ./...


tidy:
	go mod tidy
	go mod vendor