# PyDexer: Python Code Graph Indexer

PyDexer is an AST-based Python code indexer that extracts structural metadata from Python codebases, building a graph of nodes (classes, functions, etc.) and relationships (calls, imports, etc.). It outputs JSON for analysis or ingestion into Neo4j for querying and visualization. Ideal for code intelligence, refactoring, and AI-driven insights.

## Features

- **AST-Powered Extraction**: Precise parsing of classes, functions, control flow, imports, and more.
- **Qualified Naming**: Tracks scoped names (e.g., `mypackage.MyClass.method`) and hierarchies.
- **Bidirectional Relations**: Captures calls, definitions, and their inverses (e.g., CALLS/CALLED_BY).
- **Import Resolution**: Maps local imports to fully qualified names for accurate call graphs.
- **Position Tracking**: Line/column positions for IDE-like navigation.
- **Docstring Extraction**: Captures module, class, and function documentation.
- **Type Annotations**: Preserves parameter and return type annotations.
- **Flat JSON Schema**: Normalized output with embedded relations for easy graph building.
- **CLI & API**: Command-line tool and Python imports for integration.
- **Neo4j Ingestion**: Bulk loading with batching and indexes for large graphs.
- **Git Support**: Clone and index remote repositories.

## Installation

### Prerequisites
- **Python 3.8+**: For AST parsing.
- **Neo4j (Optional)**: For graph storage. Install Neo4j Desktop/Server and set env vars:
  ```bash
  export NEO4J_URI="neo4j://localhost:7687"
  export NEO4J_USER="neo4j"
  export NEO4J_PASSWORD="your_password"
  ```
- **Git (Optional)**: For repo cloning.

### Setup
1. Clone/download pydexer.
2. Core extraction uses only Python stdlib (`ast`, `json`, etc.).
3. For Neo4j: `pip install neo4j`.
4. (Optional) `.env` file for credentials.

## Usage

### CLI
Index directories or Git repos to JSON.

```bash
python extract.py --pkg myproject --dir ./src --out graph.json
python extract.py --repo https://github.com/user/repo.git --out repo_graph.json
```

Options: `--pkg` (prefix), `--dir` (local path), `--repo` (Git URL), `--ref` (branch), `--out` (output file).

### Programmatic
```python
from extract import extract, extract_ast_nodes

result, time = extract("myproject", "./src")
nodes = extract_ast_nodes("file.py", "module", "./src")
```

### Neo4j Ingestion
1. Create indexes in Neo4j:
   ```cypher
   CREATE INDEX FOR (n:CodeNode) ON (n.id);
   CREATE INDEX FOR (n:CodeNode) ON (n.qualified_name);
   CREATE INDEX FOR (n:CodeNode) ON (n.parent_qualified_name);
   ```
2. Ingest: `python ingest.py` (uses `output/output.json`).
3. Query: `MATCH (a)-[:CALLS]->(b) RETURN a.name, b.name`.

## Output Schema

PyDexer outputs a flat JSON structure where each key is a qualified name and each value contains the node's metadata and relationships.

### Node Structure (JSON Schema)

Each node has the following fields:

```json
{
  "<qualified_name>": {
    "name": "string",
    "kind": "module|class|function|async_function|params_of|returns|yields|assignment|augmented_assignment|try|except|import",
    "ast_type": "string",
    "pos": {
      "start": "integer",
      "end": "integer"
    },
    "qualified_name": "string",
    "parent_qualified_name": "string|null",
    "annotation": "string|null",
    "modifier": "string|null",
    "docstring": "string|null",
    "relations": [
      {
        "source": "string",
        "rel_type": "CLASS_DEF|FUNCTION_DEF|PARAM_OF|RETURNS|IMPORTS|CALLS|CALLED_BY|INHERITS_FROM|DECORATED_BY|YIELDS|ASSIGNS|TRY|EXCEPT|FINALLY",
        "target": "string",
        "pos": {
          "start": "integer",
          "end": "integer"
        }
      }
    ]
  }
}
```

### Node Fields

- **name**: The simple name of the node (e.g., `MyClass`, `my_function`)
- **kind**: The semantic type of the node (see Node Kinds below)
- **ast_type**: The Python AST node type (e.g., `ClassDef`, `FunctionDef`, `arg`)
- **pos**: Position information with `start` and `end` line numbers (1-based)
- **qualified_name**: Fully qualified name including module and parent scopes
- **parent_qualified_name**: Qualified name of the parent scope (null for top-level module)
- **annotation**: Type annotation string for parameters and return types (null if not annotated)
- **modifier**: Parameter modifiers like `*` (for *args) or `**` (for **kwargs), null otherwise
- **docstring**: Extracted documentation string from modules, classes, and functions (null if absent)
- **relations**: Array of relationship objects connecting this node to others

### Node Kinds

- **module**: Python file/module with optional docstring
- **class**: Class definition
- **function**: Standard function or method definition
- **async_function**: Async function/method definition
- **params_of**: Function parameter with optional type annotation
- **returns**: Function return type annotation
- **yields**: Yield expression in generator functions
- **assignment**: Variable assignment statement
- **augmented_assignment**: Augmented assignment (e.g., `+=`, `-=`)
- **try**: Try block in exception handling
- **except**: Exception handler block
- **import**: Import statement

### Relation Types

PyDexer captures bidirectional relationships with explicit relation types:

- **CLASS_DEF**: Class defined in parent scope
- **FUNCTION_DEF**: Function defined in parent scope
- **PARAM_OF**: Parameter belongs to function
- **RETURNS**: Return type annotation of function
- **IMPORTS**: Module imports target
- **CALLS**: Function calls target function
- **CALLED_BY**: Function is called by source (inverse of CALLS)
- **INHERITS_FROM**: Class inherits from base class
- **DECORATED_BY**: Function/class decorated by decorator
- **YIELDS**: Generator yields value
- **ASSIGNS**: Assignment to variable
- **TRY**: Try block relationship
- **EXCEPT**: Exception handler relationship
- **FINALLY**: Finally block relationship

### Complete Example

```json
{
  "mymodule": {
    "name": "mymodule",
    "kind": "module",
    "ast_type": "Module",
    "pos": {"start": 1, "end": 1},
    "qualified_name": "mymodule",
    "parent_qualified_name": null,
    "annotation": null,
    "modifier": null,
    "docstring": "Module documentation.",
    "relations": []
  },
  "mymodule.MyClass": {
    "name": "MyClass",
    "kind": "class",
    "ast_type": "ClassDef",
    "pos": {"start": 10, "end": 25},
    "qualified_name": "mymodule.MyClass",
    "parent_qualified_name": "mymodule",
    "annotation": null,
    "modifier": null,
    "docstring": "A sample class.",
    "relations": [
      {
        "source": "mymodule.MyClass",
        "rel_type": "CLASS_DEF",
        "target": "mymodule",
        "pos": {"start": 10, "end": 10}
      }
    ]
  },
  "mymodule.MyClass.my_method": {
    "name": "my_method",
    "kind": "function",
    "ast_type": "FunctionDef",
    "pos": {"start": 15, "end": 20},
    "qualified_name": "mymodule.MyClass.my_method",
    "parent_qualified_name": "mymodule.MyClass",
    "annotation": null,
    "modifier": null,
    "docstring": "Method documentation.",
    "relations": [
      {
        "source": "mymodule.MyClass.my_method",
        "rel_type": "FUNCTION_DEF",
        "target": "mymodule.MyClass",
        "pos": {"start": 15, "end": 15}
      },
      {
        "source": "mymodule.MyClass.my_method",
        "rel_type": "CALLS",
        "target": "print",
        "pos": {"start": 18, "end": 18}
      }
    ]
  }
}
```

## Architecture

- **extract.py**: CLI and file processing orchestration.
- **NodeCollector.py**: AST visitor for node/relation extraction with scope tracking.
- **rel_types.py**: Relation type definitions and mappings.
- **ast_utils.py**: Utility functions for docstring extraction.
- **ingest.py**: Neo4j bulk loader with batching.
- **git_support.py**: Git repository cloning utilities.

## Performance & Limitations

- Fast extraction; ingestion optimized with batching/indexes.
- AST-accurate but static (no runtime).
- Scales to large repos with Neo4j tuning.

## Contributing

PRs welcome for new features or fixes. License: MIT.