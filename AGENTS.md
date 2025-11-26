# Repository Guidelines

## Project Structure & Module Organization
- `main.go` wires the Go extractor with the processing pipeline; update the hard-coded module/path in `process/orchestrate.go` before each run.
- `extract/` contains language-specific AST walkers. `extract/golang/` holds the Go implementation plus helpers and fixtures in `testdata.go`.
- `process/` converts extracted symbols into CSV files that land in `neo4j/import/` for downstream graph loading.
- `observations/` captures design notes—refresh or append when evolving the architecture or data schema.

## Build, Test, and Development Commands
- `make run` builds `bin/codegraph` and executes the end-to-end extractor; ensure the target repo path in `process/orchestrate.go` exists locally.
- `make build` compiles the binary without running it, useful before shipping changes.
- `make du` / `make dd` proxy `docker compose up -d` and `docker compose down` for the local Neo4j stack.
- `go test ./...` runs package tests when they are added; prefer invoking it before every PR even if no tests are currently present.

## Coding Style & Naming Conventions
- Follow standard Go formatting (`go fmt ./...`); the repo expects gofmt-style tabs and camelCase identifiers.
- Keep extractor types and CSV schema structs in `extract` and `process` named after their Neo4j node or relationship (e.g., `CSVNodeExporter`, `Function`).
- Log with `slog` and wrap exported errors using Go's `%w` pattern to preserve context.

## Testing Guidelines
- Colocate tests beside source files using the `_test.go` suffix and table-driven cases.
- Mock the extractor interfaces or feed fixtures from `extract/golang/testdata.go` to validate orchestration without hitting disk.
- Surface coverage deltas in PRs; aim for new logic to include direct unit coverage and CSV snapshot assertions.

## Commit & Pull Request Guidelines
- Follow the observed `type: summary` convention (`feat:`, `chore:`, `fix:`) using lower-case imperatives.
- Reference trackers or discussion threads in the body, and note any schema changes to keep Neo4j consumers informed.
- PRs should link to the motivating issue, describe verification steps (`make run`, `go test ./...`), and attach CSV diffs or screenshots when they explain behavioral changes.

## Neo4j Export Tips
- Ensure `neo4j/import/` stays writable; the exporters overwrite files such as `type.csv` and `function.csv` on each run.
- After generating CSVs, point Neo4j's `LOAD CSV` commands at the same directory or mount it into your container for ingestion.
