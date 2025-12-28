# CodeGraph Schema

Schema definition for the ArangoDB graph database used by the CodeGraph Retriever.

## Architecture Overview

CodeGraph uses **ArangoDB** for storing code entities and their relationships as a graph database. Code search is performed using filesystem tools (ripgrep, fd) rather than a separate search index.

| Component | Purpose | Technology |
|----------|---------|------------|
| **ArangoDB Graph** | Store code relationships | Nodes (functions, types, etc.) + edges (calls, implements, etc.) |
| **Filesystem Tools** | Code search | ripgrep (grep), fd (glob), direct file reading |

---

## ArangoDB Schema

### Node Collections

#### `functions`
Functions, methods, and async functions.

```json
{
  "_key": "hash(qname)",
  "qname": "myproject.services.user.UserService.create",
  "name": "create",
  "kind": "function",
  "doc": "Create a new user.",
  "filepath": "services/user.py",
  "namespace": "myproject.services.user",
  "language": "python",
  "pos": 42,
  "end": 58,
  "is_method": true,
  "is_async": false
}
```

#### `types`
Classes, interfaces (ABC/Protocol), type aliases.

```json
{
  "_key": "hash(qname)",
  "qname": "myproject.services.user.UserService",
  "name": "UserService",
  "kind": "class",
  "doc": "Handles user operations.",
  "filepath": "services/user.py",
  "namespace": "myproject.services.user",
  "language": "python",
  "pos": 10,
  "end": 100
}
```

**kind values:** `class`, `interface`, `alias`

#### `members`
Class attributes, module-level variables/constants.

```json
{
  "_key": "hash(qname)",
  "qname": "myproject.services.user.UserService.db",
  "name": "db",
  "type_qname": "sqlalchemy.orm.Session",
  "filepath": "services/user.py",
  "namespace": "myproject.services.user",
  "language": "python",
  "pos": 12,
  "end": 12
}
```

#### `files`
Source files.

```json
{
  "_key": "hash(qname)",
  "qname": "myproject.services.user",
  "name": "user.py",
  "filepath": "services/user.py",
  "namespace": "myproject.services",
  "language": "python"
}
```

#### `modules`
Packages/modules (namespace containers).

```json
{
  "_key": "hash(qname)",
  "qname": "myproject.services",
  "name": "services",
  "language": "python"
}
```

---

### Edge Collections

#### `calls`
Function A calls function B.

```json
{
  "_from": "functions/abc123",
  "_to": "functions/def456",
  "call_site_pos": 55
}
```

**Direction:** Outbound = "what does this call?", Inbound = "who calls this?"

#### `implements`
Type implements interface (Python: ABC, Protocol).

```json
{
  "_from": "types/abc123",
  "_to": "types/def456"
}
```

#### `inherits`
Class inherits from base class.

```json
{
  "_from": "types/abc123",
  "_to": "types/def456"
}
```

#### `returns`
Function returns type.

```json
{
  "_from": "functions/abc123",
  "_to": "types/def456"
}
```

#### `param_of`
Type is parameter of function.

```json
{
  "_from": "types/abc123",
  "_to": "functions/def456",
  "position": 1,
  "name": "user"
}
```

#### `parent`
Child belongs to parent (methods→class, members→class, functions→file).

```json
{
  "_from": "functions/abc123",
  "_to": "types/def456"
}
```

#### `imports`
File imports module.

```json
{
  "_from": "files/abc123",
  "_to": "modules/def456",
  "alias": "db"
}
```

#### `decorated_by`
Function/class is decorated (Python-specific).

```json
{
  "_from": "functions/abc123",
  "_to": "functions/def456"
}
```

**Use case:** "Find all functions decorated with @app.route"

---

### Named Graph Definition

```javascript
{
  name: "codegraph",
  edgeDefinitions: [
    { collection: "calls", from: ["functions"], to: ["functions"] },
    { collection: "implements", from: ["types"], to: ["types"] },
    { collection: "inherits", from: ["types"], to: ["types"] },
    { collection: "returns", from: ["functions"], to: ["types"] },
    { collection: "param_of", from: ["types"], to: ["functions"] },
    { collection: "parent", from: ["functions", "members"], to: ["types", "files"] },
    { collection: "imports", from: ["files"], to: ["modules"] },
    { collection: "decorated_by", from: ["functions", "types"], to: ["functions"] }
  ]
}
```

---

## Key Query Patterns (AQL)

### Find callers of a function
```aql
FOR caller IN 1..3 INBOUND @qname GRAPH "codegraph"
  OPTIONS { edgeCollections: ["calls"] }
  RETURN caller
```

### Find implementations of an interface
```aql
FOR impl IN 1..1 INBOUND @interface_qname GRAPH "codegraph"
  OPTIONS { edgeCollections: ["implements"] }
  RETURN impl
```

### Find all methods of a class
```aql
FOR method IN 1..1 INBOUND @type_qname GRAPH "codegraph"
  OPTIONS { edgeCollections: ["parent"] }
  FILTER IS_SAME_COLLECTION("functions", method)
  RETURN method
```

### Find usages of a type (as param or return)
```aql
LET as_param = (
  FOR func IN 1..1 OUTBOUND @type_qname GRAPH "codegraph"
    OPTIONS { edgeCollections: ["param_of"] }
    RETURN func
)
LET as_return = (
  FOR func IN 1..1 INBOUND @type_qname GRAPH "codegraph"
    OPTIONS { edgeCollections: ["returns"] }
    RETURN func
)
RETURN UNION_DISTINCT(as_param, as_return)
```

### Find decorated functions
```aql
FOR func IN 1..1 INBOUND @decorator_qname GRAPH "codegraph"
  OPTIONS { edgeCollections: ["decorated_by"] }
  RETURN func
```

---

## Qualified Name (qname) Convention

Format: `{module_path}.{type}.{member}`

| Language | Example |
|----------|---------|
| Python | `myapp.services.user_service.UserService.create` |
| Go | `github.com/org/repo/pkg.UserService.Create` |
| TypeScript | `@org/package/src/services/UserService.create` |

The qname is the **unique identifier** across the system. `_key = hash(qname)`.

---

## Language-Specific Notes

### Python
- Classes use `kind: "class"` (not "struct")
- `is_async: true` for async functions
- `decorated_by` edges for decorators
- `inherits` edges for class inheritance

### Go
- Implicit interface implementation via `implements` edges
- No `inherits` edges (composition over inheritance)
- `is_method: true` for receiver functions

---

## Key Generation

```python
import hashlib

def make_key(qname: str) -> str:
    return hashlib.md5(qname.encode()).hexdigest()[:16]
```

This produces a 16-character hex string suitable for ArangoDB `_key`.
