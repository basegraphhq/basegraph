# Custom Linting Rules in Go

This document explains how to implement custom linting rules for project-specific code standards using golangci-lint.

## How Go Linting Works

**golangci-lint** is a meta-linter that runs multiple linters in parallel. It's fast, includes 100+ built-in linters, and supports custom plugins.

```
Your Code → golangci-lint → Multiple Linters → Report Issues
```

### Built-in vs Custom Linters

**Built-in linters** (100+ available):
- `staticcheck` - Finds bugs and simplifications
- `gosec` - Security issues
- `govet` - Correctness issues
- `errcheck` - Unchecked errors
- `errorlint` - Error wrapping issues
- `unused` - Unused code
- And many more...

**Custom linters**: Code you write to enforce project-specific rules that aren't covered by built-in linters.

## Approaches for Custom Rules

### 1. Static Analysis Plugin (Complex Rules)

Write a Go plugin using the `go/analysis` framework for complex logic.

**When to use:**
- Complex pattern detection (e.g., "ensure enum values are only from defined constants")
- Type-aware checks
- Cross-function or cross-package analysis

**Pros:**
- Full AST (Abstract Syntax Tree) access
- Type information available
- Can detect complex patterns
- Integrates with golangci-lint

**Cons:**
- Requires learning AST traversal
- More code to write
- Steeper learning curve

### 2. Configuration-based (Import Restrictions)

Use built-in linters like `gomodguard` or `depguard` to block specific imports or patterns.

**When to use:**
- "Don't import X package"
- "Use Y instead of Z"
- Package-level restrictions

**Pros:**
- Zero code to write
- Just configuration
- Fast

**Cons:**
- Limited to import/package restrictions
- Can't analyze value assignments or logic

### 3. Regex-based (Simple Patterns)

Some linters support regex patterns in configuration.

**Pros:**
- Simple patterns without coding

**Cons:**
- Very limited
- No type awareness
- Easy to have false positives

## Understanding AST Analysis

### What is an AST?

Go code is parsed into a tree structure called an Abstract Syntax Tree (AST).

**Example:**
```go
integration.Provider = "gitlab"
```

**AST Structure:**
```
AssignStmt
├── Left: SelectorExpr
│   ├── X: integration (Ident)
│   └── Sel: Provider (Ident)
└── Right: "gitlab" (BasicLit, STRING)
```

### Basic Analyzer Structure

```go
package enumvalidator

import (
    "go/ast"
    "golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
    Name: "enumvalidator",
    Doc:  "checks that enum fields only use defined constants",
    Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
    // Walk through all files
    for _, file := range pass.Files {
        ast.Inspect(file, func(n ast.Node) bool {
            // Find assignment statements
            assign, ok := n.(*ast.AssignStmt)
            if !ok {
                return true
            }

            // Check if left side is a field we care about
            // Check if right side is a string literal
            // If both true → report error

            return true
        })
    }
    return nil, nil
}
```

## Implementation Guide: Custom Analyzer

### Project Structure

```
relay/
├── tools/
│   └── linters/
│       ├── enumvalidator/
│       │   ├── analyzer.go       # Analyzer logic
│       │   ├── analyzer_test.go  # Tests
│       │   └── testdata/         # Test fixtures
│       │       └── src/
│       │           └── example.go
│       └── plugin.go              # Plugin registration
```

### Step 1: Create Analyzer

**File: `tools/linters/enumvalidator/analyzer.go`**

```go
package enumvalidator

import (
    "go/ast"
    "go/types"

    "golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
    Name: "enumvalidator",
    Doc:  "checks that enum fields only use defined constants",
    Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
    // Define enum types to check
    enumTypes := map[string]bool{
        "Provider":       true,
        "Capability":     true,
        "CredentialType": true,
    }

    for _, file := range pass.Files {
        ast.Inspect(file, func(n ast.Node) bool {
            assign, ok := n.(*ast.AssignStmt)
            if !ok {
                return true
            }

            // Check each assignment
            for i, lhs := range assign.Lhs {
                if i >= len(assign.Rhs) {
                    continue
                }

                // Check if left side is an enum field
                if sel, ok := lhs.(*ast.SelectorExpr); ok {
                    if isEnumField(pass, sel, enumTypes) {
                        // Check if right side is a string literal
                        if isStringLiteral(assign.Rhs[i]) {
                            pass.Reportf(assign.Pos(),
                                "enum field %s assigned string literal; use defined constant",
                                sel.Sel.Name)
                        }
                    }
                }
            }

            return true
        })
    }
    return nil, nil
}

func isEnumField(pass *analysis.Pass, sel *ast.SelectorExpr, enumTypes map[string]bool) bool {
    // Get type info for the selector
    if t := pass.TypesInfo.TypeOf(sel); t != nil {
        if named, ok := t.(*types.Named); ok {
            return enumTypes[named.Obj().Name()]
        }
    }
    return false
}

func isStringLiteral(expr ast.Expr) bool {
    lit, ok := expr.(*ast.BasicLit)
    return ok && lit.Kind == token.STRING
}
```

### Step 2: Register Plugin

**File: `tools/linters/plugin.go`**

```go
package main

import (
    "golang.org/x/tools/go/analysis"
    "github.com/golangci/plugin-module-register/register"
    "basegraph.app/api-server/tools/linters/enumvalidator"
)

func init() {
    register.Plugin("enumvalidator", New)
}

func New(conf any) ([]*analysis.Analyzer, error) {
    return []*analysis.Analyzer{enumvalidator.Analyzer}, nil
}
```

### Step 3: Build Plugin

```bash
go build -buildmode=plugin -o tools/linters/enumvalidator.so tools/linters/
```

### Step 4: Configure golangci-lint

**File: `.golangci.yml`**

```yaml
linters-settings:
  custom:
    enumvalidator:
      path: ./tools/linters/enumvalidator.so
      description: Validates enum field assignments
```

## Testing Custom Analyzers

### Test Structure

**File: `tools/linters/enumvalidator/analyzer_test.go`**

```go
package enumvalidator_test

import (
    "testing"

    "golang.org/x/tools/go/analysis/analysistest"
    "basegraph.app/api-server/tools/linters/enumvalidator"
)

func Test(t *testing.T) {
    testdata := analysistest.TestData()
    analysistest.Run(t, testdata, enumvalidator.Analyzer, "example")
}
```

**File: `tools/linters/enumvalidator/testdata/src/example/example.go`**

```go
package example

type Provider string

const (
    ProviderGitHub Provider = "github"
    ProviderGitLab Provider = "gitlab"
)

type Integration struct {
    Provider Provider
}

func bad() {
    i := &Integration{}
    i.Provider = "bitbucket" // want "enum field Provider assigned string literal"
}

func good() {
    i := &Integration{}
    i.Provider = ProviderGitHub // OK: using constant
}
```

Run tests:
```bash
go test ./tools/linters/enumvalidator/...
```

## Suppressing False Positives

Use `//nolint` directives to suppress false positives:

```go
const (
    CredentialTypeUserOAuth CredentialType = "user_oauth" //nolint:gosec // False positive: enum constant, not hardcoded credential
)
```

## Learning Resources

- [Go AST Viewer](https://yuroyoro.github.io/goast-viewer/) - Paste code, see AST
- [Writing Go Analyzers](https://disaev.me/p/writing-useful-go-analysis-linter/) - Tutorial
- [go/analysis package](https://pkg.go.dev/golang.org/x/tools/go/analysis) - Official docs
- [golangci-lint analyzers](https://github.com/golangci/golangci-lint/tree/master/pkg/golinters) - Example implementations

## Common Patterns

### Checking Field Types

```go
func getFieldType(pass *analysis.Pass, sel *ast.SelectorExpr) string {
    if t := pass.TypesInfo.TypeOf(sel); t != nil {
        return t.String()
    }
    return ""
}
```

### Checking Function Calls

```go
func isFunctionCall(expr ast.Expr, funcName string) bool {
    call, ok := expr.(*ast.CallExpr)
    if !ok {
        return false
    }

    if ident, ok := call.Fun.(*ast.Ident); ok {
        return ident.Name == funcName
    }
    return false
}
```

### Reporting with Context

```go
pass.Reportf(node.Pos(),
    "found %s at line %d: %s",
    issue,
    pass.Fset.Position(node.Pos()).Line,
    detail)
```

## Best Practices

1. **Start Simple** - Begin with basic pattern matching, add complexity as needed
2. **Test Thoroughly** - Write tests with both positive and negative cases
3. **Document Well** - Explain why the rule exists and how to fix violations
4. **Minimize False Positives** - Better to miss some issues than flag correct code
5. **Provide Examples** - Show good and bad examples in error messages
6. **Make it Fast** - Analyzers run on every lint, keep them performant
7. **Version Control** - Check in your analyzer code and tests
