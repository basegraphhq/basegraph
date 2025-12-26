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

test:
	go test -v ./...


tidy:
	go mod tidy
	go mod vendor