Codegraph - Index your codebase into Code property Graph

Codegraph takes in path and package name of a codebase(Go is supported as of now), and emits a series of csv indices. This indices can be imported onto Neo4J and can be used to perform complex graph queries.
Example: Structs that implement x interface that are defined in y package.


Can go further with introduction of vector indexing of Graph nodes.

## Codegraph Assistant

A CLI coding assistant that uses OpenAI's Responses API with function calling to query the Neo4j code graph and interact with the workspace filesystem.

### Prerequisites

1. **Neo4j instance** running and populated with code graph data (see extraction/ingestion below)
2. **OpenAI API key** with access to models supporting the Responses API (e.g., `gpt-5-codex`, `gpt-4o`)
3. **Go 1.24+** toolchain

### Setup

1. Extract and ingest your codebase into Neo4j:

```go
// See demo/demo.go for a complete example
extractor := golang.NewGoExtractor()
result, err := extractor.Extract("github.com/your/repo", "/path/to/repo")

ingestor, err := process.NewNeo4jIngestor(process.Neo4jConfig{
    URI:      "neo4j://localhost:7687",
    Username: "neo4j",
    Password: "password",
    Database: "neo4j",
})
err = ingestor.Ingest(context.Background(), result)
```

2. Set required environment variables:

```bash
export OPENAI_API_KEY=sk-...
export OPENAI_MODEL=gpt-5-codex          # or gpt-4o, gpt-4o-mini
export NEO4J_URI=neo4j://localhost:7687
export NEO4J_USERNAME=neo4j
export NEO4J_PASSWORD=password
export NEO4J_DATABASE=neo4j
export WORKSPACE_ROOT=/path/to/your/workspace  # defaults to current directory
```

Optional:
```bash
export OPENAI_BASE_URL=https://api.openai.com/v1  # custom endpoint
export OPENAI_ORG_ID=org-...                       # organization ID
```

3. Run the assistant:

```bash
go run ./...
# or
go build -o codegraph-assistant && ./codegraph-assistant
```

### Available Tools

The assistant has access to the following tools:

**Neo4j Code Graph Tools:**
- `search_code_symbols`: Find symbols by name/qualified name/namespace/kind
- `get_symbol_details`: Retrieve full details including code, docs, and relationships (CALLS, IMPLEMENTS, RETURNS, PARAMS)
- `grep_code_nodes`: Search within node code/doc/name fields with context snippets

**Filesystem Tools:**
- `read_entire_file`: Read full file contents (up to 200kB)
- `read_partial_file`: Read specific line ranges with line numbers
- `list_directory`: Breadth-first directory listing with depth control
- `apply_patch`: Replace/create/delete file content with targeted edits

### Example Queries

```
» where is the NewNeo4jIngestor function defined?
» show me all functions that call Extract
» grep for "VerifyConnectivity" in the codebase
» list the assistant directory
» read the config.go file in the assistant package
```

### Architecture

- **Main loop** (`assistant/runner.go`): reads user input, calls OpenAI Responses API, executes tools, and maintains conversation state
- **Tool registry** (`assistant/tool_registry.go`): manages function definitions and handlers
- **Code graph tools** (`assistant/codegraph.go`): Neo4j Cypher queries for symbol search/detail/grep
- **Filesystem tools** (`assistant/filesystem.go`): safe file operations within workspace boundaries 
