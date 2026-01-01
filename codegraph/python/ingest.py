import json
import os
import time
from neo4j import GraphDatabase
from neo4j.exceptions import Neo4jError
from rel_types import RelType
from dotenv import load_dotenv
load_dotenv()

# Configuration (set these env vars before running)
NEO4J_URI = os.environ.get("NEO4J_URI", "neo4j://127.0.0.1:7687")
NEO4J_USER = os.environ.get("NEO4J_USER", "neo4j")
NEO4J_PASSWORD = os.environ["NEO4J_PASSWORD"]

INPUT_JSON = os.environ.get("INPUT_JSON", "output/output.json")  # path to the pydexer output
ALLOWED_REL_TYPES = {r.value for r in RelType}

def ingest(driver, data):
    batch_size = 1000  # Commit every 1000 operations
    
    with driver.session() as session:
        # Step 1: Ingest nodes in batches
        print("Ingesting nodes...")
        tx = session.begin_transaction()
        for i, (qual, meta) in enumerate(data.items()):
            _create_code_node(tx, qual, meta)
            if (i + 1) % batch_size == 0:
                tx.commit()
                tx = session.begin_transaction()
        tx.commit()  # Commit any remaining
        
        # Step 2: Ingest relationships in batches
        print("Ingesting relationships...")
        tx = session.begin_transaction()
        rel_count = 0
        for qual, meta in data.items():
            for rel in meta.get("relations", []):
                _create_relationship(tx, rel)
                rel_count += 1
                if rel_count % batch_size == 0:
                    tx.commit()
                    tx = session.begin_transaction()
        tx.commit()  # Commit any remaining

def _create_code_node(tx, qual, meta):
    tx.run(
        """
        MERGE (n:CodeNode {id: $id})
          ON CREATE SET
            n.name = $name,
            n.kind = $kind,
            n.ast_type = $ast_type,
            n.qualified_name = $qualified_name,
            n.parent_qualified_name = $parent_qualified_name,
            n.start = $pos_start,
            n.end = $pos_end,
            n.meta = $meta_json
          ON MATCH SET
            n.name = $name,
            n.kind = $kind,
            n.ast_type = $ast_type,
            n.qualified_name = $qualified_name,
            n.parent_qualified_name = $parent_qualified_name,
            n.start = $pos_start,
            n.end = $pos_end,
            n.meta = $meta_json
        FOREACH (parent IN (CASE WHEN $parent_qualified_name IS NULL THEN [] ELSE [$parent_qualified_name] END) |
            MERGE (p:CodeNode {id: parent})
            MERGE (p)-[:PARENT_OF]->(n)
        )
        """,
        id=qual,  # stable identifier; here we reuse the dict key
        name=meta.get("name"),
        kind=meta.get("kind"),
        ast_type=meta.get("ast_type"),
        qualified_name=meta.get("qualified_name"),
        parent_qualified_name=meta.get("parent_qualified_name"),
        pos_start=(meta.get("pos") or {}).get("start"),
        pos_end=(meta.get("pos") or {}).get("end"),
        meta_json=json.dumps(meta),
    )


def _create_relationship(tx, rel):
    rel_type = rel.get("rel_type")
    if rel_type not in ALLOWED_REL_TYPES:
        return
    cypher = f"""
        MERGE (src:CodeNode {{id: $src_id}})
        MERGE (dst:CodeNode {{id: $dst_id}})
        MERGE (src)-[r:{rel_type}]->(dst)
        """
    tx.run(
        cypher,
        src_id=rel.get("source"),
        dst_id=rel.get("target"),
    )

def main():
    with open(INPUT_JSON, "r", encoding="utf-8") as f:
        data = json.load(f)

    print(NEO4J_PASSWORD)
    print(NEO4J_URI)
    driver = GraphDatabase.driver(NEO4J_URI, auth=(NEO4J_USER, NEO4J_PASSWORD))
    try:
        try:
            driver.verify_connectivity()
            print("✅ Connected to Neo4j")
        except Neo4jError as e:
            print("❌ Failed to connect to Neo4j:", e)
            return
        start = time.time()
        ingest(driver, data)
        elapsed = time.time() - start
        print("Ingestion complete")
        print(f"Total time elapsed: {elapsed}")
    finally:
        driver.close()

if __name__ == "__main__":
    main()