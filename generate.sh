#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$SCRIPT_DIR"

echo "Generating SDKs from openapi.json..."
echo ""

# ============ Generate OpenAPI spec ============
echo "Generating OpenAPI spec from Go ingress..."
cd "$ROOT_DIR/packages/go/ingress"
go run ./internal/routes/generate/generate-openapi.go
cd "$ROOT_DIR"
pnpm biome format --write sdk/openapi.json
echo "OpenAPI spec generated"
echo ""

# ============ TypeScript ============
echo "Generating TypeScript client..."
cd "$ROOT_DIR"
pnpm -F @parallelworks/client generate-types
echo "TypeScript client generated"
echo ""

# ============ Go ============
echo "Generating Go SDK..."
cd "$SCRIPT_DIR"

# Uses the version pinned in go.mod via `go get -tool`
cd "$ROOT_DIR"
go tool github.com/parallelworks/openapi-client-generator generate \
    -s sdk/openapi.json \
    -o sdk/go \
    -p parallelworks

cd "$SCRIPT_DIR/go"
go mod tidy
go fmt ./...
echo "Go SDK generated"
echo ""

# ============ Python ============
echo "Generating Python SDK..."
cd "$SCRIPT_DIR"

# Use uv to run openapi-python-client
uvx openapi-python-client generate \
    --path ./openapi.json \
    --output-path ./python/parallelworks_client \
    --meta none \
    --overwrite

echo "Python SDK generated"
echo ""

echo "All SDKs generated successfully!"
