#!/usr/bin/env python3
"""
Typesense ingestion CLI for CodeGraph.

Ingests extracted Python codebase into Typesense search index.
Typesense stores code snippets and metadata for full-text search.
"""

import argparse
import json
import os
import re
import sys
from typing import Any

import typesense
from typesense.exceptions import ObjectAlreadyExists


# Schema for code_nodes collection
CODE_NODES_SCHEMA = {
    "name": "code_nodes",
    "fields": [
        {"name": "qname", "type": "string"},
        {"name": "name", "type": "string"},
        {"name": "name_variants", "type": "string[]"},
        {"name": "code", "type": "string", "optional": True},
        {"name": "doc", "type": "string", "optional": True},
        {"name": "kind", "type": "string", "facet": True},
        {"name": "language", "type": "string", "facet": True},
        {"name": "filepath", "type": "string", "optional": True},
        {"name": "namespace", "type": "string", "optional": True},
        {"name": "pos", "type": "int32", "optional": True},
        {"name": "end", "type": "int32", "optional": True},
    ],
}

# Kinds to index (skip synthetic nodes like params_of, returns, etc.)
INDEXABLE_KINDS = {
    "class",
    "function",
    "async_function",
    "module",
    "assignment",  # Module-level assignments
}


def make_name_variants(name: str | None) -> list[str]:
    """Generate name variants for fuzzy matching."""
    if not name:
        return []

    variants = [name]

    # Lowercase
    lower = name.lower()
    if lower not in variants:
        variants.append(lower)

    # snake_case from CamelCase
    snake = re.sub(r"(?<!^)(?=[A-Z])", "_", name).lower()
    if snake not in variants:
        variants.append(snake)

    # No underscores
    no_underscore = name.replace("_", "")
    if no_underscore not in variants:
        variants.append(no_underscore)

    return variants


def setup_collection(client: typesense.Client, collection_name: str = "code_nodes") -> None:
    """Create or update the Typesense collection."""
    try:
        # Try to create collection
        schema = CODE_NODES_SCHEMA.copy()
        schema["name"] = collection_name
        client.collections.create(schema)
        print(f"  Created collection: {collection_name}")
    except ObjectAlreadyExists:
        print(f"  Collection exists: {collection_name}")


def should_index(node: dict[str, Any]) -> bool:
    """Determine if a node should be indexed in Typesense."""
    kind = node.get("kind")
    if kind not in INDEXABLE_KINDS:
        return False

    # Skip synthetic/internal nodes
    qname = node.get("qualified_name", "")
    if ".param." in qname or ".return" in qname or ".yields." in qname:
        return False

    return True


def ingest_documents(
    client: typesense.Client,
    data: dict[str, Any],
    collection_name: str = "code_nodes",
) -> int:
    """Ingest documents into Typesense."""
    batch_size = 100
    batch: list[dict] = []
    total_indexed = 0

    for qname, node in data.items():
        if not should_index(node):
            continue

        # Build document
        name = node.get("name") or qname.split(".")[-1]
        doc = {
            "id": qname.replace("/", "_").replace(".", "_"),  # Typesense-safe ID
            "qname": qname,
            "name": name,
            "name_variants": make_name_variants(name),
            "kind": node.get("kind", "unknown"),
            "language": "python",
        }

        # Optional fields
        if node.get("code"):
            doc["code"] = node["code"]
        if node.get("docstring"):
            doc["doc"] = node["docstring"]
        if node.get("filepath"):
            doc["filepath"] = node["filepath"]
        if node.get("parent_qualified_name"):
            doc["namespace"] = node["parent_qualified_name"]

        pos = node.get("pos", {})
        if pos.get("start"):
            doc["pos"] = pos["start"]
        if pos.get("end"):
            doc["end"] = pos["end"]

        batch.append(doc)

        if len(batch) >= batch_size:
            try:
                client.collections[collection_name].documents.import_(
                    batch, {"action": "upsert"}
                )
                total_indexed += len(batch)
            except Exception as e:
                print(f"  Warning: Failed to import batch: {e}")
            batch = []

    # Flush remaining
    if batch:
        try:
            client.collections[collection_name].documents.import_(
                batch, {"action": "upsert"}
            )
            total_indexed += len(batch)
        except Exception as e:
            print(f"  Warning: Failed to import final batch: {e}")

    return total_indexed


def main() -> None:
    parser = argparse.ArgumentParser(description="Ingest CodeGraph into Typesense")
    parser.add_argument(
        "--input",
        dest="input",
        default="output/output.json",
        help="Path to extractor JSON output (default: output/output.json)",
    )
    parser.add_argument(
        "--url",
        dest="url",
        default=os.environ.get("TYPESENSE_URL", "http://localhost:8108"),
        help="Typesense URL (default: http://localhost:8108)",
    )
    parser.add_argument(
        "--api-key",
        dest="api_key",
        default=os.environ.get("TYPESENSE_API_KEY", "xyz"),
        help="Typesense API key (default: xyz or TYPESENSE_API_KEY env)",
    )
    parser.add_argument(
        "--collection",
        dest="collection",
        default="code_nodes",
        help="Typesense collection name (default: code_nodes)",
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

    # Parse URL for host/port/protocol
    url = args.url
    if url.startswith("https://"):
        protocol = "https"
        host_port = url[8:]
    elif url.startswith("http://"):
        protocol = "http"
        host_port = url[7:]
    else:
        protocol = "http"
        host_port = url

    if ":" in host_port:
        host, port = host_port.split(":", 1)
        port = int(port)
    else:
        host = host_port
        port = 8108

    # Connect to Typesense
    print(f"Connecting to Typesense at {args.url}...")
    client = typesense.Client({
        "nodes": [{"host": host, "port": port, "protocol": protocol}],
        "api_key": args.api_key,
        "connection_timeout_seconds": 10,
    })

    # Verify connection
    try:
        client.collections.retrieve()
        print("  Connected successfully")
    except Exception as e:
        print(f"Error: Failed to connect to Typesense: {e}")
        sys.exit(1)

    # Setup collection
    print("Setting up collection...")
    setup_collection(client, args.collection)

    # Ingest documents
    print("Ingesting documents...")
    count = ingest_documents(client, data, args.collection)
    print(f"  Indexed {count} documents")

    print("\nIngestion complete!")


if __name__ == "__main__":
    main()
