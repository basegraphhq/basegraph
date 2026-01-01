# Execution Path Trace: main → Orchestrate → Go Extractor → ExportCalls

This document traces the directed execution path from `cmd/codegraph/main.go:8` through `process.Orchestrate` into the Go extractor and its visitors, down to `process/export_relations.go:56`'s `CSVRelationshipExporter.ExportCalls`.

## Execution Flow

### 1. Entry Point
**Location:** `cmd/codegraph/main.go:8-10`
```go
func main() {
    process.Orchestrate(golang.NewGoExtractor())
}
```
- **Function:** `main()`
- **Action:** Creates a `*GoExtractor` instance and passes it to `process.Orchestrate`
- **Interface Edge:** Passes `*GoExtractor` as `extract.Extractor` interface

---

### 2. Orchestration
**Location:** `process/orchestrate.go:15-86`
```go
func Orchestrate(e extract.Extractor) {
    // ... setup code ...
    for _, mod := range mods {
        moduleRes, extractErr := e.Extract(mod.ModulePath, mod.Dir)
        mergeExtractResults(&acc, moduleRes)
    }
    // ... ingestion code ...
}
```
- **Function:** `process.Orchestrate(e extract.Extractor)`
- **Interface Edge:** Calls `e.Extract()` on the `extract.Extractor` interface
- **Action:** 
  - Iterates through discovered Go modules
  - Calls `e.Extract()` for each module (line 54)
  - Accumulates results via `mergeExtractResults()` (line 59)
  - The accumulated `extractRes.Functions` map contains all extracted functions with their `Calls` slices populated

**Note:** The current `Orchestrate` function does NOT call `ExportCalls()` directly. Instead, it uses Neo4j ingestion via `ingestor.Ingest()` which internally calls `ingestCalls()` (similar logic but writes to Neo4j instead of CSV). However, `ExportToCSV()` exists (line 132) and would call `ExportCalls()` at line 172 if invoked. The trace below shows the path to `ExportCalls()` as requested.

---

### 3. Go Extractor
**Location:** `extract/golang/extract.go:23-164`
```go
func (g *GoExtractor) Extract(pkgstr string, dir string) (extract.ExtractNodesResult, error) {
    // ... package loading ...
    
    fv := &FunctionVisitor{
        Fset:      fset,
        Info:      pkg.TypesInfo,
        Functions: extractRes.Functions,
    }
    
    for _, file := range pkg.Syntax {
        ast.Walk(fv, file)  // Line 152
    }
    
    return extractRes, nil
}
```
- **Function:** `GoExtractor.Extract()`
- **Struct Edge:** Implements `extract.Extractor` interface
- **Action:**
  - Loads Go packages using `packages.Load()` (line 49)
  - Creates a `FunctionVisitor` instance (lines 127-131)
  - Shares the `extractRes.Functions` map with the visitor
  - Walks each AST file using `ast.Walk(fv, file)` (line 152)
  - Returns `ExtractNodesResult` containing the populated `Functions` map

---

### 4. Function Visitor
**Location:** `extract/golang/functions.go:18-128`
```go
func (v *FunctionVisitor) Visit(node ast.Node) ast.Visitor {
    switch n := node.(type) {
    case *ast.FuncDecl:
        // ... extract function metadata ...
        
        bv := &BodyVisitor{
            CallerQName: qname,
            Fset:        v.Fset,
            Info:        v.Info,
        }
        if n.Body != nil {
            ast.Walk(bv, n.Body)  // Line 116
        }
        f := v.Functions[qname]
        f.Calls = bv.Calls  // Line 119: CRITICAL - Assigns Calls slice
        v.Functions[qname] = f  // Line 120: Updates function in map
    }
}
```
- **Function:** `FunctionVisitor.Visit()`
- **Struct Edge:** Implements `ast.Visitor` interface
- **Action:**
  - Detects `*ast.FuncDecl` nodes (function declarations)
  - Extracts function metadata (name, position, parameters, returns)
  - Creates and stores the function in `v.Functions[qname]` (lines 65, 86, or 107 depending on function type)
  - Creates a `BodyVisitor` instance (lines 110-114) with:
    - `CallerQName`: The qualified name of the function being analyzed
    - `Fset` and `Info`: Shared type information
  - **Line 116:** Walks the function body AST using `ast.Walk(bv, n.Body)`
  - **Line 118:** Retrieves the function from the map
  - **Line 119:** **CRITICAL** - Assigns `bv.Calls` to `f.Calls` (this is where the Calls slice is populated)
  - **Line 120:** Updates the function in the `Functions` map with the populated `Calls` slice

---

### 5. Body Visitor - Call Detection
**Location:** `extract/golang/body.go:17-57`
```go
func (v *BodyVisitor) Visit(node ast.Node) ast.Visitor {
    switch n := node.(type) {
    case *ast.CallExpr:
        v.handleCallExpr(n)  // Line 25
        return v
    }
}

func (v *BodyVisitor) handleCallExpr(ce *ast.CallExpr) {
    if id, ok := ce.Fun.(*ast.Ident); ok {
        // Direct function call: funcName()
        ceObj := v.Info.Uses[id]
        if ceObj != nil && ceObj.Pkg() != nil {
            callee := ceObj.Pkg().Path() + "." + ceObj.Name()
            v.Calls = append(v.Calls, callee)  // Line 44: Populates Calls slice
        }
    } else if se, ok := ce.Fun.(*ast.SelectorExpr); ok {
        // Method call: obj.Method()
        seObj := v.Info.Uses[se.Sel]
        if seObj != nil && seObj.Pkg() != nil {
            callee := seObj.Pkg().Path() + "." + seObj.Name()
            v.Calls = append(v.Calls, callee)  // Line 54: Populates Calls slice
        }
    }
}
```
- **Function:** `BodyVisitor.Visit()` and `BodyVisitor.handleCallExpr()`
- **Struct Edge:** Implements `ast.Visitor` interface
- **Action:**
  - Detects `*ast.CallExpr` nodes (function/method calls)
  - **Line 25:** Delegates to `handleCallExpr()` for processing
  - **Line 33-57:** `handleCallExpr()` processes two types of calls:
    1. **Direct calls** (`*ast.Ident`): `funcName()` - Line 37-45
       - Uses `v.Info.Uses[id]` to get type information
       - Constructs qualified name: `package.Path + "." + functionName`
       - **Line 44:** Appends to `v.Calls` slice
    2. **Method calls** (`*ast.SelectorExpr`): `obj.Method()` - Line 46-56
       - Uses `v.Info.Uses[se.Sel]` to get method information
       - Constructs qualified name: `package.Path + "." + methodName`
       - **Line 54:** Appends to `v.Calls` slice
  - The `v.Calls` slice accumulates all function/method calls found in the function body

---

### 6. Data Flow Back to Function
**Location:** `extract/golang/functions.go:118-120`
```go
f := v.Functions[qname]
f.Calls = bv.Calls  // Transfers Calls from BodyVisitor to Function
v.Functions[qname] = f
```
- After `ast.Walk(bv, n.Body)` completes, `bv.Calls` contains all detected calls
- The `Calls` slice is assigned to the `Function` struct
- The function is stored back in the `Functions` map

---

### 7. Result Accumulation
**Location:** `process/orchestrate.go:50-62`
```go
acc := newExtractAccumulator()  // Creates empty Functions map

for _, mod := range mods {
    moduleRes, extractErr := e.Extract(mod.ModulePath, mod.Dir)
    mergeExtractResults(&acc, moduleRes)  // Line 59
}

extractRes := acc  // Contains all functions with populated Calls slices
```
- **Function:** `mergeExtractResults()`
- **Action:** Merges function maps from each module extraction
- **Result:** `extractRes.Functions` contains all functions with their `Calls` slices populated

---

### 8. Export Path (Alternative: CSV Export)
**Location:** `process/orchestrate.go:132-199`
```go
func ExportToCSV(extractRes extract.ExtractNodesResult) error {
    // ... other exports ...
    
    csvr := CSVRelationshipExporter{}
    err = csvr.ExportCalls(extractRes.Functions)  // Line 172
    // ...
}
```
- **Function:** `process.ExportToCSV()`
- **Action:** Creates `CSVRelationshipExporter` and calls `ExportCalls()`
- **Note:** This function exists but is **not currently called** in the main `Orchestrate` path. The current path uses Neo4j ingestion instead (see `neo4j_ingest.go:484` `ingestCalls()` which performs similar iteration over `fn.Calls` but writes to Neo4j)

---

### 9. CSV Export
**Location:** `process/export_relations.go:56-96`
```go
func (c *CSVRelationshipExporter) ExportCalls(functions map[string]extract.Function) error {
    // ... CSV setup ...
    
    for qname, f := range functions {  // Line 78
        for _, call := range f.Calls {  // Line 79: Iterates over Calls slice
            row := []string{qname, call}  // Line 80: Creates CSV row
            err := csvwriter.Write(row)   // Line 81: Writes to calls.csv
        }
    }
    
    csvwriter.Flush()  // Line 88
}
```
- **Function:** `CSVRelationshipExporter.ExportCalls()`
- **Action:**
  - Iterates through all functions in the map (line 78)
  - For each function, iterates through its `Calls` slice (line 79)
  - Creates CSV rows with format: `[caller_qname, callee_qname]` (line 80)
  - Writes rows to `neo4j/import/calls.csv` (line 81)
  - Flushes the CSV writer (line 88)

---

## Summary: How `extract.Function.Calls` is Populated

1. **Initialization:** `BodyVisitor.Calls` is initialized as an empty slice (line 12 in `body.go`)

2. **AST Traversal:** `ast.Walk(bv, n.Body)` traverses the function body AST

3. **Call Detection:** For each `*ast.CallExpr` node:
   - `BodyVisitor.Visit()` detects the call expression
   - `handleCallExpr()` processes it:
     - For direct calls (`funcName()`): Uses `v.Info.Uses[id]` to resolve the function
     - For method calls (`obj.Method()`): Uses `v.Info.Uses[se.Sel]` to resolve the method
     - Constructs qualified name: `package.Path + "." + functionName`
     - **Appends to `v.Calls` slice** (lines 44 and 54 in `body.go`)

4. **Transfer:** After AST walk completes:
   - `f.Calls = bv.Calls` (line 119 in `functions.go`) transfers the Calls slice from `BodyVisitor` to `Function`
   - The function is stored in `v.Functions[qname]` map

5. **Accumulation:** Functions are merged across modules via `mergeExtractResults()`

6. **Consumption:** `ExportCalls()` iterates through `f.Calls` and writes CSV rows

---

## Key Interface/Struct Edges

1. **`extract.Extractor` interface** → `*GoExtractor` struct
   - `Orchestrate()` calls `e.Extract()` on the interface

2. **`ast.Visitor` interface** → `*FunctionVisitor` struct
   - `ast.Walk()` uses the visitor pattern to traverse AST

3. **`ast.Visitor` interface** → `*BodyVisitor` struct
   - `ast.Walk(bv, n.Body)` uses the visitor pattern to traverse function body

4. **Data Flow:**
   - `BodyVisitor.Calls []string` → `Function.Calls []string` (via assignment)
   - `Function.Calls []string` → CSV rows (via iteration in `ExportCalls`)

---

## File References

- `cmd/codegraph/main.go:8` - Entry point
- `process/orchestrate.go:15` - Orchestration
- `extract/golang/extract.go:23` - Go extractor
- `extract/golang/functions.go:18` - Function visitor
- `extract/golang/body.go:17` - Body visitor (call detection)
- `process/export_relations.go:56` - CSV export

