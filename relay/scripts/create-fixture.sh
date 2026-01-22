#!/usr/bin/env bash

# Script to create a complete fixture snapshot of the relay codebase
# Usage: ./scripts/create-fixture.sh <fixture-name>
# Example: ./scripts/create-fixture.sh relay-v2-snapshot

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RELAY_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
FIXTURES_DIR="$RELAY_ROOT/testdata/fixtures"

# Parse arguments
FIXTURE_NAME="$1"

if [ -z "$FIXTURE_NAME" ]; then
    echo -e "${RED}Error: Fixture name is required${NC}"
    echo ""
    echo "Usage: ./scripts/create-fixture.sh <fixture-name>"
    echo "Example: ./scripts/create-fixture.sh relay-v2-snapshot"
    exit 1
fi

FIXTURE_PATH="$FIXTURES_DIR/$FIXTURE_NAME"

# Check if fixture already exists
if [ -d "$FIXTURE_PATH" ]; then
    echo -e "${YELLOW}Warning: Fixture '$FIXTURE_NAME' already exists at $FIXTURE_PATH${NC}"
    read -p "Overwrite? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Aborted."
        exit 1
    fi
    rm -rf "$FIXTURE_PATH"
fi

echo -e "${BLUE}================================================${NC}"
echo -e "${BLUE}  Creating Fixture: $FIXTURE_NAME${NC}"
echo -e "${BLUE}================================================${NC}"
echo ""

# Step 1: Create fixture directory
echo -e "${YELLOW}[1/5] Creating fixture directory...${NC}"
mkdir -p "$FIXTURE_PATH"
echo -e "  ${GREEN}✓${NC} Created $FIXTURE_PATH"
echo ""

# Step 2: Copy source directories (excluding testdata, vendor initially)
echo -e "${YELLOW}[2/5] Copying source directories...${NC}"

# Directories to copy
DIRS_TO_COPY=(
    "cmd"
    "common"
    "core"
    "internal"
)

for dir in "${DIRS_TO_COPY[@]}"; do
    if [ -d "$RELAY_ROOT/$dir" ]; then
        echo -e "  → Copying $dir/"
        cp -r "$RELAY_ROOT/$dir" "$FIXTURE_PATH/"
    fi
done

echo -e "  ${GREEN}✓${NC} Source directories copied"
echo ""

# Step 3: Copy go.mod and go.sum, then update module path
echo -e "${YELLOW}[3/5] Setting up Go module...${NC}"

# Copy go.mod and update module path
cp "$RELAY_ROOT/go.mod" "$FIXTURE_PATH/go.mod"
cp "$RELAY_ROOT/go.sum" "$FIXTURE_PATH/go.sum"

# Update module path in go.mod
MODULE_PATH="basegraph.co/$FIXTURE_NAME"
sed -i '' "s|^module basegraph.co/relay|module $MODULE_PATH|" "$FIXTURE_PATH/go.mod"

echo -e "  → Module path: $MODULE_PATH"
echo -e "  ${GREEN}✓${NC} Go module configured"
echo ""

# Step 4: Update all import paths in the fixture
echo -e "${YELLOW}[4/5] Updating import paths...${NC}"

# Find all .go files and update imports
find "$FIXTURE_PATH" -name "*.go" -type f | while read -r file; do
    # Replace basegraph.co/relay with the new module path
    sed -i '' "s|\"basegraph.co/relay/|\"$MODULE_PATH/|g" "$file"
done

echo -e "  ${GREEN}✓${NC} Import paths updated"
echo ""

# Step 5: Copy vendor directory
echo -e "${YELLOW}[5/5] Copying vendor directory...${NC}"

if [ -d "$RELAY_ROOT/vendor" ]; then
    echo -e "  → Copying vendor/ (this may take a moment)"
    cp -r "$RELAY_ROOT/vendor" "$FIXTURE_PATH/"
    echo -e "  ${GREEN}✓${NC} Vendor directory copied"
else
    echo -e "  ${YELLOW}⚠${NC} No vendor directory found, skipping"
fi

echo ""

# Summary
echo -e "${GREEN}================================================${NC}"
echo -e "${GREEN}  ✓ Fixture Created Successfully!${NC}"
echo -e "${GREEN}================================================${NC}"
echo ""
echo -e "  Fixture: ${BLUE}$FIXTURE_NAME${NC}"
echo -e "  Path: $FIXTURE_PATH"
echo -e "  Module: $MODULE_PATH"
echo ""

# Show size
FIXTURE_SIZE=$(du -sh "$FIXTURE_PATH" | cut -f1)
echo -e "  Size: $FIXTURE_SIZE"
echo ""

echo -e "${BLUE}Next steps:${NC}"
echo -e "  • Index the fixture:"
echo -e "    ${YELLOW}cd ../codegraph/golang && FIXTURE=$FIXTURE_NAME make index-fixture${NC}"
echo ""
echo -e "  • Run worker against fixture:"
echo -e "    ${YELLOW}REPO_ROOT=$FIXTURE_PATH MODULE_PATH=$MODULE_PATH make run-worker${NC}"
echo ""
