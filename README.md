# Basegraph

A monorepo containing code analysis tools and a web dashboard for exploring codebases as property graphs.

## Project Structure

```
basegraph/
├── codegraph/          # Go service for code extraction and analysis
├── dashboard/          # Next.js web dashboard
└── relay/              # CLI AI coding assistant
```

## Quick Start

### Prerequisites

- **Go 1.24+** (for codegraph service)
- **Node.js 18+** and **Bun** (for dashboard)
- **Git**

### Codegraph Service

Codegraph extracts code structures from Go codebases and builds a property graph representation.

#### Setup

```bash
cd codegraph

# Install dependencies
go mod download
go mod vendor

# Build the service
make build-codegraph

# Run the service
make run-codegraph
```

#### Usage

```bash
# Build and run
make run-codegraph

# Run tests
make test

# Tidy dependencies
make tidy
```

For more details, see [codegraph/Readme.md](./codegraph/Readme.md).

### Relay Service

CLI AI coding assistant service.

#### Setup

```bash
cd relay

# Install dependencies
go mod download

# Build the service
make build

# Run the service
make run
```

#### Database Migrations

The relay service uses [goose](https://github.com/pressly/goose) for database migrations, managed via Go 1.24+'s `tool` directive in `go.mod`.

**Create a new migration:**
```bash
make migrate-create NAME=create_users_table
```

**Run migrations:**
```bash
# Apply all pending migrations
make migrate-up DB_DRIVER=postgres DB_STRING="postgres://user:pass@localhost/dbname"

# Rollback last migration
make migrate-down DB_DRIVER=postgres DB_STRING="postgres://user:pass@localhost/dbname"

# Check migration status
make migrate-status DB_DRIVER=postgres DB_STRING="postgres://user:pass@localhost/dbname"
```

Supported database drivers: `postgres`, `mysql`, `sqlite3`, `mssql`, `clickhouse`, `vertica`, `ydb`, `turso`.

**Note:** Goose is declared in `go.mod` using Go 1.24+'s `tool` directive. No global installation needed — `go tool goose` runs it directly using the version from `go.mod`.

### Dashboard

A Next.js web application for visualizing and interacting with code graphs.

#### Setup

```bash
cd dashboard

# Install dependencies
bun install

# Run development server
bun run dev
```

The dashboard will be available at `http://localhost:3000`.

#### Available Scripts

- `bun run dev` - Start development server
- `bun run build` - Build for production
- `bun run start` - Start production server
- `bun run lint` - Run ESLint

## Development

### Monorepo Structure

This is a monorepo containing multiple services:

- **codegraph/**: Go-based code extraction and graph building service
- **dashboard/**: Next.js frontend for code graph visualization
- **relay/**: CLI AI coding assistant (see [Relay_v1.md](./Relay_v1.md))

### Building Everything

```bash
# Build codegraph service
cd codegraph && make build-codegraph

# Build relay service
cd relay && make build

# Build dashboard (production)
cd dashboard && bun run build
```

## Codegraph Assistant

The codegraph service includes a CLI assistant that uses OpenAI's API with function calling to query code graphs and interact with workspace filesystem.

### Prerequisites

- OpenAI API key with access to models supporting function calling
- Workspace with code graph data (if using graph queries)

### Setup

See [codegraph/Readme.md](./codegraph/Readme.md) for detailed setup instructions.

### Available Tools

**Code Graph Tools:**
- `search_code_symbols`: Find symbols by name/qualified name/namespace/kind
- `get_symbol_details`: Retrieve full details including code, docs, and relationships
- `grep_code_nodes`: Search within node code/doc/name fields

**Filesystem Tools:**
- `read_entire_file`: Read full file contents
- `read_partial_file`: Read specific line ranges
- `list_directory`: Directory listing with depth control
- `apply_patch`: Replace/create/delete file content

## Testing

### Codegraph Tests

```bash
cd codegraph
make test
```

### Dashboard Tests

```bash
cd dashboard
bun run lint
```

## Dependencies

### Go (codegraph)
- See [codegraph/go.mod](./codegraph/go.mod)

### Node.js (dashboard)
- See [dashboard/package.json](./dashboard/package.json)

## Configuration

### Environment Variables

#### Codegraph Assistant

```bash
export OPENAI_API_KEY=sk-...
export OPENAI_MODEL=gpt-4o
export WORKSPACE_ROOT=/path/to/workspace
```

See [codegraph/Readme.md](./codegraph/Readme.md) for full configuration options.

#### Dashboard

Create a `.env.local` file in the `dashboard/` directory for local development.

## Documentation

- [Codegraph README](./codegraph/Readme.md) - Detailed codegraph service documentation
- [Relay v1 Spec](./Relay_v1.md) - Planned CLI AI assistant architecture
- [Dashboard Auth Setup](./dashboard/docs/AUTH_SETUP.md) - Authentication configuration
- [Design System](./dashboard/docs/DESIGN_SYSTEM.md) - UI component guidelines

## Contributing

1. Create a feature branch from `main`
2. Make your changes
3. Ensure tests pass
4. Submit a pull request

## License

[Add your license here]

## Related Projects

- [Codegraph Assistant](./codegraph/assistant/) - CLI coding assistant
- [Dashboard](./dashboard/) - Web interface for code graphs


