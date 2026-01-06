#!/usr/bin/env bash

# Script to clear and re-index the relay codebase into ArangoDB
# Usage: ./scripts/reindex-codegraph.sh [--skip-cleanup]

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Load environment variables
if [ -f .env ]; then
    export $(grep -v '^#' .env | xargs)
fi

# Configuration from environment or defaults
ARANGO_URL="${ARANGO_URL:-http://localhost:8529}"
ARANGO_USERNAME="${ARANGO_USERNAME:-root}"
ARANGO_PASSWORD="${ARANGO_PASSWORD:-your-secret-password}"
ARANGO_DATABASE="${ARANGO_DATABASE:-codegraph}"

TARGET_REPO_PATH="${TARGET_REPO_PATH:-$(pwd)}"
CODEGRAPH_BIN="../codegraph/golang/bin/codegraph"

# Parse arguments
SKIP_CLEANUP=false
for arg in "$@"; do
    if [ "$arg" == "--skip-cleanup" ]; then
        SKIP_CLEANUP=true
    fi
done

echo -e "${BLUE}================================================${NC}"
echo -e "${BLUE}  Relay Codegraph Reindex Script${NC}"
echo -e "${BLUE}================================================${NC}"
echo ""

# Step 1: Cleanup existing data
if [ "$SKIP_CLEANUP" = false ]; then
    echo -e "${YELLOW}[1/3] Cleaning up existing data...${NC}"
    echo ""

    # Delete ArangoDB database
    echo -e "  → Deleting ArangoDB database '${ARANGO_DATABASE}'..."
    RESPONSE=$(curl -s -w "\n%{http_code}" -X DELETE \
        "${ARANGO_URL}/_api/database/${ARANGO_DATABASE}" \
        -u "${ARANGO_USERNAME}:${ARANGO_PASSWORD}")

    HTTP_CODE=$(echo "$RESPONSE" | tail -n 1)
    BODY=$(echo "$RESPONSE" | sed '$d')

    if [ "$HTTP_CODE" == "200" ]; then
        echo -e "    ${GREEN}✓${NC} ArangoDB database deleted"
    elif [ "$HTTP_CODE" == "404" ]; then
        echo -e "    ${YELLOW}⚠${NC} Database didn't exist (already clean)"
    else
        echo -e "    ${RED}✗${NC} Failed to delete database (HTTP $HTTP_CODE)"
        echo "    Response: $BODY"
    fi

    echo ""
else
    echo -e "${YELLOW}[1/3] Skipping cleanup (--skip-cleanup flag set)${NC}"
    echo ""
fi

# Step 2: Verify cleanup
echo -e "${YELLOW}[2/3] Verifying cleanup...${NC}"
echo ""

# Check ArangoDB databases
ARANGO_DBS=$(curl -s "${ARANGO_URL}/_db/_system/_api/database" \
    -u "${ARANGO_USERNAME}:${ARANGO_PASSWORD}" | grep -o '"result":\[[^]]*\]')
echo -e "  → ArangoDB databases: ${ARANGO_DBS}"

echo ""

# Step 3: Build latest codegraph binary
echo -e "${YELLOW}[3/4] Building latest codegraph binary...${NC}"
echo ""

echo -e "  → Building codegraph..."
(cd ../codegraph/golang && go build -o bin/codegraph cmd/codegraph/main.go)

if [ ! -f "$CODEGRAPH_BIN" ]; then
    echo -e "${RED}✗ Failed to build codegraph binary${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Codegraph binary built${NC}"
echo ""

# Step 4: Run codegraph extraction and ingestion
echo -e "${YELLOW}[4/4] Running codegraph extraction and ingestion...${NC}"
echo ""

echo -e "  → Target repository: ${TARGET_REPO_PATH}"
echo -e "  → Codegraph binary: ${CODEGRAPH_BIN}"
echo ""

# Export configuration for codegraph
export TARGET_REPO_PATH
export ARANGO_URL
export ARANGO_USERNAME
export ARANGO_PASSWORD
export ARANGO_DATABASE

# Run codegraph
# Note: testdata directories are automatically skipped by Go's packages.Load
echo -e "${BLUE}  Starting extraction and ingestion...${NC}"
echo ""

START_TIME=$(date +%s)

set +e
$CODEGRAPH_BIN
CODEGRAPH_STATUS=$?
set -e

if [ $CODEGRAPH_STATUS -ne 0 ]; then
    echo -e "${RED}✗ Codegraph failed with status ${CODEGRAPH_STATUS}${NC}"
    exit $CODEGRAPH_STATUS
fi

END_TIME=$(date +%s)
ELAPSED=$((END_TIME - START_TIME))

echo ""
echo -e "${GREEN}================================================${NC}"
echo -e "${GREEN}  ✓ Reindex Complete!${NC}"
echo -e "${GREEN}================================================${NC}"
echo ""
echo -e "  Total time: ${ELAPSED}s"
echo ""

# Step 4: Show statistics
echo -e "${BLUE}Statistics:${NC}"
echo ""

# ArangoDB collection info
ARANGO_COLLECTIONS=$(curl -s "${ARANGO_URL}/_db/${ARANGO_DATABASE}/_api/collection" \
    -u "${ARANGO_USERNAME}:${ARANGO_PASSWORD}")

echo -e "  ${GREEN}→${NC} ArangoDB database '${ARANGO_DATABASE}' ready"

echo ""
echo -e "${BLUE}Next steps:${NC}"
echo -e "  • Start the relay worker: ${YELLOW}make run-worker${NC}"
echo -e "  • Test the retriever with: ${YELLOW}BRAIN_DEBUG_DIR=debug_logs make run-worker${NC}"
echo ""
