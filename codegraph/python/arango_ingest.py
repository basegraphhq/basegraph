#!/usr/bin/env python3
"""
ArangoDB ingestion CLI for CodeGraph.

Ingests extracted Python codebase into ArangoDB graph database.
ArangoDB stores lean graph data (no code snippets) for relationship traversal.
"""

import argparse
import hashlib
import json
import os
import sys
from typing import Any

from arango import ArangoClient
from arango.database import StandardDatabase
from arango.exceptions import CollectionCreateError, GraphCreateError


# Node collection mappings: Python kind -> ArangoDB collection
KIND_TO_COLLECTION = {
    "class": "types",
    "function": "functions",
    "async_function": "functions",
    "module": "modules",
    # These are typically edges, not nodes
    "params_of": None,
    "returns": None,
    "import": None,
    # Module-level assignments become members
    "assignment": "members",
    "augmented_assignment": "members",
}

# Edge collection mappings: Python rel_type -> ArangoDB edge collection
REL_TO_EDGE_COLLECTION = {
    "CALLS": "calls",
    "CALLED_BY": None,  # Skip inverse edge
    "INHERITS_FROM": "inherits",
    "DECORATED_BY": "decorated_by",
    "PARAM_OF": "param_of",
    "RETURNS": "returns",
    "IMPORTS": "imports",
    "CLASS_DEF": "parent",  # class defined in module -> parent edge
    "FUNCTION_DEF": "parent",  # function defined in class/module -> parent edge
}

# Node collections to create
NODE_COLLECTIONS = ["functions", "types", "members", "files", "modules"]

# Edge collections with their from/to constraints
# ArangoDB driver expects: edge_collection, from_vertex_collections, to_vertex_collections
EDGE_DEFINITIONS = [
    {"edge_collection": "calls", "from_vertex_collections": ["functions"], "to_vertex_collections": ["functions"]},
    {"edge_collection": "implements", "from_vertex_collections": ["types"], "to_vertex_collections": ["types"]},
    {"edge_collection": "inherits", "from_vertex_collections": ["types"], "to_vertex_collections": ["types"]},
    {"edge_collection": "returns", "from_vertex_collections": ["functions"], "to_vertex_collections": ["types"]},
    {"edge_collection": "param_of", "from_vertex_collections": ["types"], "to_vertex_collections": ["functions"]},
    {"edge_collection": "parent", "from_vertex_collections": ["functions", "members"], "to_vertex_collections": ["types", "files", "modules"]},
    {"edge_collection": "imports", "from_vertex_collections": ["files", "modules"], "to_vertex_collections": ["modules"]},
    {"edge_collection": "decorated_by", "from_vertex_collections": ["functions", "types"], "to_vertex_collections": ["functions"]},
]


def make_key(qname: str) -> str:
    """Generate a 16-char hex key from qname using MD5."""
    return hashlib.md5(qname.encode()).hexdigest()[:16]


def setup_database(db: StandardDatabase, graph_name: str = "codegraph") -> None:
    """Create collections and graph if they don't exist."""
    # Create node collections
    for coll_name in NODE_COLLECTIONS:
        try:
            db.create_collection(coll_name)
            print(f"  Created collection: {coll_name}")
        except CollectionCreateError:
            pass  # Already exists

    # Create edge collections
    edge_names = {e["edge_collection"] for e in EDGE_DEFINITIONS}
    for edge_name in edge_names:
        try:
            db.create_collection(edge_name, edge=True)
            print(f"  Created edge collection: {edge_name}")
        except CollectionCreateError:
            pass  # Already exists

    # Create named graph
    try:
        db.create_graph(
            graph_name,
            edge_definitions=EDGE_DEFINITIONS,
        )
        print(f"  Created graph: {graph_name}")
    except GraphCreateError:
        pass  # Already exists


def determine_collection(node: dict[str, Any]) -> str | None:
    """Determine which ArangoDB collection a node belongs to."""
    kind = node.get("kind")
    parent_qname = node.get("parent_qualified_name")

    # Direct mapping
    if kind in KIND_TO_COLLECTION:
        coll = KIND_TO_COLLECTION[kind]
        if coll:
            # For assignments, only module-level become members
            if kind in ("assignment", "augmented_assignment"):
                # Check if parent is a class (then it's a member)
                # For now, skip function-local assignments
                if parent_qname and ".assignment." not in parent_qname:
                    return "members"
                return None
            return coll

    return None


def ingest_nodes(db: StandardDatabase, data: dict[str, Any]) -> dict[str, str]:
    """Ingest nodes into appropriate collections. Returns qname -> collection mapping."""
    qname_to_collection: dict[str, str] = {}
    batch_size = 1000
    batches: dict[str, list[dict]] = {coll: [] for coll in NODE_COLLECTIONS}

    for qname, node in data.items():
        collection = determine_collection(node)
        if not collection:
            continue

        qname_to_collection[qname] = collection

        # Build lean document (no code field)
        doc = {
            "_key": make_key(qname),
            "qname": qname,
            "name": node.get("name"),
            "kind": node.get("kind"),
            "doc": node.get("docstring"),
            "filepath": node.get("filepath"),
            "namespace": node.get("parent_qualified_name"),
            "language": "python",
            "pos": node.get("pos", {}).get("start"),
            "end": node.get("pos", {}).get("end"),
        }

        # Add async flag for functions
        if node.get("kind") == "async_function":
            doc["is_async"] = True
            doc["kind"] = "function"  # Normalize kind

        # Add is_method flag
        if collection == "functions" and node.get("parent_qualified_name"):
            parent = data.get(node["parent_qualified_name"], {})
            if parent.get("kind") == "class":
                doc["is_method"] = True

        batches[collection].append(doc)

        # Flush batch if needed
        if len(batches[collection]) >= batch_size:
            db.collection(collection).import_bulk(batches[collection], on_duplicate="replace")
            batches[collection] = []

    # Flush remaining
    for collection, docs in batches.items():
        if docs:
            db.collection(collection).import_bulk(docs, on_duplicate="replace")

    return qname_to_collection


def ingest_edges(db: StandardDatabase, data: dict[str, Any], qname_to_coll: dict[str, str]) -> None:
    """Ingest edges from relations into edge collections."""
    batch_size = 1000
    edge_collections = {e["edge_collection"] for e in EDGE_DEFINITIONS}
    batches: dict[str, list[dict]] = {coll: [] for coll in edge_collections}

    for qname, node in data.items():
        relations = node.get("relations", [])
        for rel in relations:
            rel_type = rel.get("rel_type")
            source = rel.get("source")
            target = rel.get("target")

            if not rel_type or not source or not target:
                continue

            edge_coll = REL_TO_EDGE_COLLECTION.get(rel_type)
            if not edge_coll:
                continue

            # Determine source and target collections
            source_coll = qname_to_coll.get(source)
            target_coll = qname_to_coll.get(target)

            # For call edges to classes (constructor calls), redirect to __init__
            if edge_coll == "calls" and target_coll == "types":
                init_qname = f"{target}.__init__"
                if init_qname in qname_to_coll:
                    target = init_qname
                    target_coll = qname_to_coll.get(target)
                else:
                    # Skip constructor calls if __init__ not found
                    continue

            # For parent edges, target might be a module not in our mapping
            if edge_coll == "parent" and not target_coll:
                # Check if target looks like a module
                if target in data and data[target].get("kind") == "module":
                    target_coll = "modules"

            if not source_coll or not target_coll:
                continue

            edge_doc = {
                "_key": make_key(f"{source}:{target}:{rel_type}"),
                "_from": f"{source_coll}/{make_key(source)}",
                "_to": f"{target_coll}/{make_key(target)}",
            }

            # Add position info for calls
            if edge_coll == "calls":
                pos = rel.get("pos", {})
                if pos.get("start"):
                    edge_doc["call_site_pos"] = pos["start"]

            batches[edge_coll].append(edge_doc)

            if len(batches[edge_coll]) >= batch_size:
                try:
                    db.collection(edge_coll).import_bulk(batches[edge_coll], on_duplicate="replace")
                except Exception as e:
                    print(f"  Warning: Failed to import {edge_coll} edges: {e}")
                batches[edge_coll] = []

    # Flush remaining
    for edge_coll, docs in batches.items():
        if docs:
            try:
                db.collection(edge_coll).import_bulk(docs, on_duplicate="replace")
            except Exception as e:
                print(f"  Warning: Failed to import {edge_coll} edges: {e}")


def main() -> None:
    parser = argparse.ArgumentParser(description="Ingest CodeGraph into ArangoDB")
    parser.add_argument(
        "--input",
        dest="input",
        default="output/output.json",
        help="Path to extractor JSON output (default: output/output.json)",
    )
    parser.add_argument(
        "--url",
        dest="url",
        default=os.environ.get("ARANGO_URL", "http://localhost:8529"),
        help="ArangoDB URL (default: http://localhost:8529)",
    )
    parser.add_argument(
        "--db",
        dest="db",
        default=os.environ.get("ARANGO_DB", "codegraph"),
        help="ArangoDB database name (default: codegraph)",
    )
    parser.add_argument(
        "--user",
        dest="user",
        default=os.environ.get("ARANGO_USER", "root"),
        help="ArangoDB username (default: root)",
    )
    parser.add_argument(
        "--password",
        dest="password",
        default=os.environ.get("ARANGO_PASSWORD", ""),
        help="ArangoDB password (default: empty or ARANGO_PASSWORD env)",
    )
    parser.add_argument(
        "--graph",
        dest="graph",
        default="codegraph",
        help="Named graph name (default: codegraph)",
    )
    args = parser.parse_args()

    # Load extracted data
    print(f"Loading {args.input}...")
    try:
        with open(args.input, "r", encoding="utf-8") as f:
            data = json.load(f)
    except FileNotFoundError:
        print(f"Error: File not found: {args.input}")
        sys.exit(1)

    print(f"  Loaded {len(data)} nodes")

    # Connect to ArangoDB
    print(f"Connecting to ArangoDB at {args.url}...")
    client = ArangoClient(hosts=args.url)

    # Get system db to create database if needed
    sys_db = client.db("_system", username=args.user, password=args.password)
    if not sys_db.has_database(args.db):
        sys_db.create_database(args.db)
        print(f"  Created database: {args.db}")

    db = client.db(args.db, username=args.user, password=args.password)
    print(f"  Connected to database: {args.db}")

    # Setup collections and graph
    print("Setting up collections and graph...")
    setup_database(db, args.graph)

    # Ingest nodes
    print("Ingesting nodes...")
    qname_to_coll = ingest_nodes(db, data)
    print(f"  Ingested {len(qname_to_coll)} nodes")

    # Ingest edges
    print("Ingesting edges...")
    ingest_edges(db, data, qname_to_coll)
    print("  Done")

    print("\nIngestion complete!")


if __name__ == "__main__":
    main()
