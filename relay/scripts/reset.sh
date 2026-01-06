#!/usr/bin/env bash

# Complete reset script for testing
# Usage:
#   ./scripts/reset.sh              # Reset everything
#   ./scripts/reset.sh --redis-only # Only clear Redis
#   ./scripts/reset.sh --codegraph-only    # Only clear ArangoDB
#   ./scripts/reset.sh --no-reindex # Reset but don't re-index

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Load environment
if [ -f .env ]; then
    export $(grep -v '^#' .env | xargs)
fi

# Configuration
REDIS_URL="${REDIS_URL:-redis://localhost:6379/0}"
REDIS_STREAM="${REDIS_STREAM:-relay_events}"
REDIS_GROUP="${REDIS_CONSUMER_GROUP:-relay_group}"
REDIS_DLQ_STREAM="${REDIS_DLQ_STREAM:-relay_events_dlq}"

ARANGO_URL="${ARANGO_URL:-http://localhost:8529}"
ARANGO_USERNAME="${ARANGO_USERNAME:-root}"
ARANGO_PASSWORD="${ARANGO_PASSWORD:-your-secret-password}"
ARANGO_DATABASE="${ARANGO_DATABASE:-codegraph}"

# Parse arguments
RESET_REDIS=true
RESET_DB=true
REINDEX=true

for arg in "$@"; do
    case $arg in
        --redis-only)
            RESET_DB=false
            REINDEX=false
            ;;
        --codegraph-only)
            RESET_REDIS=false
            REINDEX=false
            ;;
        --no-reindex)
            REINDEX=false
            ;;
    esac
done

echo -e "${BLUE}================================================${NC}"
echo -e "${BLUE}  Relay Testing Reset Script${NC}"
echo -e "${BLUE}================================================${NC}"
echo ""

# ============================================================================
# Redis Reset
# ============================================================================
if [ "$RESET_REDIS" = true ]; then
    echo -e "${YELLOW}[1] Resetting Redis Streams...${NC}"
    echo ""

    # Main stream
    echo -e "  → Checking stream '${REDIS_STREAM}'..."

    # Get pending messages
    PENDING=$(redis-cli -u "$REDIS_URL" XPENDING "$REDIS_STREAM" "$REDIS_GROUP" - + 100 2>/dev/null || echo "")

    if [ -n "$PENDING" ] && [ "$PENDING" != "(empty array)" ]; then
        # Extract message IDs and acknowledge them
        MESSAGE_IDS=$(echo "$PENDING" | awk 'NR % 4 == 1' | tr '\n' ' ')
        if [ -n "$MESSAGE_IDS" ]; then
            ACK_COUNT=$(redis-cli -u "$REDIS_URL" XACK "$REDIS_STREAM" "$REDIS_GROUP" $MESSAGE_IDS)
            echo -e "    ${GREEN}✓${NC} Acknowledged $ACK_COUNT pending messages"
        fi
    else
        echo -e "    ${GREEN}✓${NC} No pending messages"
    fi

    # Trim stream
    TRIMMED=$(redis-cli -u "$REDIS_URL" XTRIM "$REDIS_STREAM" MAXLEN 0)
    echo -e "    ${GREEN}✓${NC} Deleted $TRIMMED messages from stream"

    # DLQ stream
    echo -e "  → Checking DLQ stream '${REDIS_DLQ_STREAM}'..."
    DLQ_LEN=$(redis-cli -u "$REDIS_URL" XLEN "$REDIS_DLQ_STREAM" 2>/dev/null || echo "0")
    if [ "$DLQ_LEN" -gt 0 ]; then
        redis-cli -u "$REDIS_URL" DEL "$REDIS_DLQ_STREAM" > /dev/null
        echo -e "    ${GREEN}✓${NC} Deleted DLQ stream ($DLQ_LEN messages)"
    else
        echo -e "    ${GREEN}✓${NC} DLQ stream empty"
    fi

    echo ""
fi

# ============================================================================
# Database Reset
# ============================================================================
if [ "$RESET_DB" = true ]; then
    echo -e "${YELLOW}[2] Resetting ArangoDB...${NC}"
    echo ""

    # ArangoDB
    echo -e "  → Deleting ArangoDB database '${ARANGO_DATABASE}'..."
    RESPONSE=$(curl -s -w "\n%{http_code}" -X DELETE \
        "${ARANGO_URL}/_api/database/${ARANGO_DATABASE}" \
        -u "${ARANGO_USERNAME}:${ARANGO_PASSWORD}")

    HTTP_CODE=$(echo "$RESPONSE" | tail -n 1)
    BODY=$(echo "$RESPONSE" | sed '$d')

    if [ "$HTTP_CODE" == "200" ]; then
        echo -e "    ${GREEN}✓${NC} ArangoDB database deleted"
    elif [ "$HTTP_CODE" == "404" ]; then
        echo -e "    ${GREEN}✓${NC} Database didn't exist (already clean)"
    else
        echo -e "    ${YELLOW}⚠${NC} HTTP $HTTP_CODE"
        echo "    Response: $BODY"
    fi

    echo ""
fi

# ============================================================================
# Verification
# ============================================================================
echo -e "${YELLOW}[3] Verification...${NC}"
echo ""

if [ "$RESET_REDIS" = true ]; then
    STREAM_LEN=$(redis-cli -u "$REDIS_URL" XLEN "$REDIS_STREAM")
    PENDING_COUNT=$(redis-cli -u "$REDIS_URL" XPENDING "$REDIS_STREAM" "$REDIS_GROUP" | grep "pending" | awk '{print $2}')
    echo -e "  ${GREEN}→${NC} Redis stream length: ${STREAM_LEN}"
    echo -e "  ${GREEN}→${NC} Pending messages: ${PENDING_COUNT:-0}"
fi

if [ "$RESET_DB" = true ]; then
    ARANGO_DBS=$(curl -s "${ARANGO_URL}/_db/_system/_api/database" -u "${ARANGO_USERNAME}:${ARANGO_PASSWORD}" | grep -o '"codegraph"' | wc -l)

    if [ "$ARANGO_DBS" -eq 0 ]; then
        echo -e "  ${GREEN}→${NC} ArangoDB: codegraph database deleted"
    else
        echo -e "  ${YELLOW}→${NC} ArangoDB: codegraph still exists"
    fi
fi

echo ""

# ============================================================================
# Re-index
# ============================================================================
if [ "$REINDEX" = true ]; then
    echo -e "${YELLOW}[4] Re-indexing codegraph...${NC}"
    echo ""

    # Check if reindex script exists
    if [ -f "./scripts/reindex-codegraph.sh" ]; then
        ./scripts/reindex-codegraph.sh --skip-cleanup
    else
        echo -e "  ${YELLOW}⚠${NC} Reindex script not found, skipping"
        echo -e "  ${BLUE}→${NC} Run manually: make reindex-codegraph"
    fi
else
    echo -e "${BLUE}[4] Skipping re-index (use 'make reindex-codegraph' to index)${NC}"
    echo ""
fi

# ============================================================================
# Summary
# ============================================================================
echo -e "${GREEN}================================================${NC}"
echo -e "${GREEN}  ✓ Reset Complete!${NC}"
echo -e "${GREEN}================================================${NC}"
echo ""

if [ "$RESET_REDIS" = true ]; then
    echo -e "  ${GREEN}✓${NC} Redis streams cleared"
fi

if [ "$RESET_DB" = true ]; then
    echo -e "  ${GREEN}✓${NC} ArangoDB database deleted"
fi

if [ "$REINDEX" = true ]; then
    echo -e "  ${GREEN}✓${NC} Codegraph re-indexed"
fi

echo ""
echo -e "${BLUE}System is ready for testing!${NC}"
echo ""
