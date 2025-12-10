# Linting and Code Quality Guide for Relay

## Overview

This guide covers the complete linting setup for the Relay project, including standard Go linters, custom analyzers, and pre-commit hooks.

## Quick Start

For new developers, run this single command to set up everything:

```bash
./scripts/setup-linters.sh
```

This will:
1. Install all required tools (golangci-lint, gofumpt, etc.)
2. Configure pre-commit hooks
3. Build custom analyzers (if supported on your platform)

## Architecture

### 1. Standard Linters (via golangci-lint)

We use `golangci-lint` as our main linting framework. It runs multiple linters in parallel:

**Enabled Linters:**
- `errcheck` - Checks for unchecked errors
- `gosimple` - Simplifies code
- `govet` - Reports suspicious constructs
- `ineffassign` - Detects ineffectual assignments
- `staticcheck` - Advanced static analysis
- `unused` - Finds unused code
- `bodyclose` - Checks HTTP response body closure
- `gofmt` - Checks formatting
- `gosec` - Security checks
- `misspell` - Finds misspelled words
- `unconvert` - Removes unnecessary type conversions
- `unparam` - Finds unused function parameters
- `errorlint` - Error wrapping checks
- `errname` - Checks error variable naming
- `gocognit` - Cognitive complexity checker (max: 20)
- `gocyclo` - Cyclomatic complexity checker (max: 15)
- `revive` - Fast, configurable linter
- `testifylint` - Testify-specific checks
- `usetesting` - Ensures proper testing package usage

**Configuration:** `.golangci.yml`

### 2. Custom Analyzers

#### enumvalidator

Ensures enum fields are assigned using constants, not string literals.

**What it checks:**
- `Provider` field assignments
- `Capability` field assignments  
- `CredentialType` field assignments

**Example:**
```go
// ❌ BAD - String literal
integration.Provider = "gitlab"

// ✅ GOOD - Using constant
integration.Provider = model.ProviderGitLab
```

**Location:** `tools/linters/enumvalidator/`

### 3. Code Formatting

We use `gofumpt` (stricter than `gofmt`) for consistent formatting:

```bash
# Format all Go files
make format

# Format specific files
gofumpt -w path/to/file.go
```

**What gofumpt does:**
- Removes unnecessary blank lines
- Standardizes import grouping
- Consistent spacing around operators
- Sorts struct fields (in some cases)

## Available Commands

### Makefile Targets

```bash
# Install all tools
make install-tools

# Run standard linters
make lint

# Auto-fix linting issues (where possible)
make lint-fix

# Format code with gofumpt
make format

# Run custom analyzers
make lint-custom

# Run ALL linters (standard + custom)
make lint-all
```

### Manual Commands

```bash
# Run golangci-lint directly
golangci-lint run ./...

# Run specific linter
golangci-lint run --disable-all --enable errcheck ./...

# Run on specific files
golangci-lint run path/to/file.go

# Check new code only (useful in PRs)
golangci-lint run --new-from-rev=main
```

## Pre-commit Hook

The pre-commit hook automatically runs on every commit to ensure code quality.

### What it does:

1. **Formats Go code** - Runs gofumpt on staged files
2. **Runs linters** - Checks only changed files
3. **Verifies SQL** - If SQL files changed, validates syntax

### Bypass (use sparingly):

```bash
git commit --no-verify -m "Emergency fix"
```

### Manual Installation:

If the setup script didn't work:

```bash
# Copy the hook
cp scripts/pre-commit-hook .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
```

## Troubleshooting

### Plugin Compatibility Issues

If you see errors about plugin version mismatches:

```
Error: plugin was built with a different version of package
```

**Solution:** Custom analyzers run separately via `make lint-custom` to avoid plugin issues.

### Linter Not Found

```
Error: golangci-lint not found
```

**Solution:**
```bash
make install-tools
# or specifically:
make install-golangci-lint
```

### Too Many Linting Errors

For existing code with many issues:

```bash
# Auto-fix what's possible
make lint-fix

# Focus on new code only
golangci-lint run --new-from-rev=main

# Disable specific linters temporarily
golangci-lint run --disable gosec,gocyclo
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Lint

on: [push, pull_request]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.24'
      
      - name: Install tools
        run: |
          cd relay
          make install-tools
      
      - name: Run linters
        run: |
          cd relay
          make lint
      
      - name: Check formatting
        run: |
          cd relay
          make format
          git diff --exit-code
```

### GitLab CI Example

```yaml
lint:
  stage: test
  script:
    - cd relay
    - make install-tools
    - make lint
    - make format && git diff --exit-code
```

## Best Practices

### 1. Fix Issues Immediately

Don't let linting issues accumulate. Fix them as part of your development:

```bash
# After making changes
make lint-fix  # Auto-fix
make lint      # Check remaining issues
```

### 2. Configure Your Editor

**VS Code:**
```json
{
  "go.lintTool": "golangci-lint",
  "go.lintOnSave": "file",
  "go.formatTool": "gofumpt",
  "editor.formatOnSave": true
}
```

**GoLand/IntelliJ:**
- File → Settings → Tools → File Watchers
- Add golangci-lint and gofumpt watchers

### 3. Review Linter Suggestions

Not all linter suggestions should be blindly accepted:

```go
// Sometimes explicit is better than implicit
var result error = nil  // gosimple might complain, but it's clear

// Document why you're disabling a linter
//nolint:errcheck // Error is intentionally ignored for cleanup
defer file.Close()
```

### 4. Custom Analyzer Development

When adding new patterns to enforce:

1. Create analyzer in `tools/linters/your-analyzer/`
2. Write comprehensive tests
3. Document the rule clearly
4. Add to `make lint-custom` target

## Gradual Adoption

For existing codebases:

### Phase 1: Format Only
```bash
make format
git commit -m "Format code with gofumpt"
```

### Phase 2: Critical Linters
```yaml
# Start with essentials in .golangci.yml
linters:
  enable:
    - errcheck
    - govet
    - staticcheck
```

### Phase 3: Style Linters
```yaml
# Add style linters
linters:
  enable:
    - gofmt
    - misspell
    - unconvert
```

### Phase 4: Complexity Linters
```yaml
# Add complexity checks
linters:
  enable:
    - gocognit
    - gocyclo
```

### Phase 5: Custom Rules
```bash
# Enable project-specific analyzers
make lint-custom
```

## Questions?

- Check `tools/linters/README.md` for custom analyzer details
- Run `golangci-lint linters` to see all available linters
- See `.golangci.yml` for current configuration

For issues or suggestions, create a GitHub issue or contact the platform team.