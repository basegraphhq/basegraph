# Custom Linters

This directory contains custom static analysis tools for the Relay project.

## Available Analyzers

### enumvalidator

Validates that enum fields are only assigned using defined constants, not string literals.

**What it checks:**
- Assignments to fields of type `Provider`, `Capability`, or `CredentialType`
- Ensures values are enum constants (e.g., `model.ProviderGitHub`)
- Prevents string literals (e.g., `"github"`)

**Example violations:**
```go
// BAD: String literal
integration.Provider = "gitlab"  // ❌ Will be flagged

// GOOD: Using constant
integration.Provider = model.ProviderGitLab  // ✅ Correct
```

## Running the Analyzers

### Running Tests

```bash
go test ./tools/linters/enumvalidator/...
```

### Integration with golangci-lint

The analyzer is currently **not enabled** in `make lint` because it requires building as a plugin.

**To enable it:**

1. Build the plugin (Linux/macOS only):
```bash
go build -buildmode=plugin -o tools/linters/plugin/enumvalidator.so tools/linters/plugin/
```

2. Uncomment the configuration in `.golangci.yml`:
```yaml
linters-settings:
  custom:
    enumvalidator:
      path: ./tools/linters/plugin/enumvalidator.so
      description: Validates enum field assignments
```

3. Run linter:
```bash
make lint
```

**Note:** Go plugins have limitations:
- Only work on Linux, macOS, and FreeBSD (not Windows)
- Must be built with the exact same Go version as golangci-lint
- Can be fragile across Go version updates

## Manual Code Review

Until the plugin is enabled, use the enum validation rules documented in:
- `docs/review-rules.md` - Manual review guidelines
- `docs/custom-linters.md` - Implementation guide

## Directory Structure

```
tools/linters/
├── README.md                          # This file
├── enumvalidator/
│   ├── analyzer.go                    # Analyzer implementation
│   ├── analyzer_test.go               # Tests
│   └── testdata/                      # Test fixtures
│       └── src/example/
│           └── example.go
└── plugin/
    └── plugin.go                      # Plugin registration for golangci-lint
```

## Adding New Analyzers

See `docs/custom-linters.md` for a complete guide on implementing custom analyzers.

**Quick steps:**
1. Create new directory under `tools/linters/`
2. Implement `analysis.Analyzer` interface
3. Write tests using `analysistest` package
4. Register in `plugin/plugin.go`
5. Update this README
