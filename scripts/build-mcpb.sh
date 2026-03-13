#!/bin/bash
# Build .mcpb bundle from GoReleaser output
set -euo pipefail

VERSION=${1:-$(git describe --tags --always)}
DIST_DIR="dist/mcpb"

echo "Building .mcpb bundle for version ${VERSION}..."

mkdir -p "${DIST_DIR}"
cp manifest.json "${DIST_DIR}/"

# Package as ZIP with .mcpb extension
cd "${DIST_DIR}" && zip -r "../mcp-server-email-${VERSION}.mcpb" .

echo "Built: dist/mcp-server-email-${VERSION}.mcpb"
