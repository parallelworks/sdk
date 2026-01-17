#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$ROOT_DIR"

echo "🔧 Generating SDKs from openapi.json..."
echo ""

# ============ TypeScript ============
echo "📦 Generating TypeScript SDK..."
cd "$ROOT_DIR/typescript"
pnpm install --frozen-lockfile 2>/dev/null || pnpm install
pnpm run generate
echo "✅ TypeScript SDK generated"
echo ""

# ============ Go ============
echo "🐹 Generating Go SDK..."
cd "$ROOT_DIR"

# Check if oapi-codegen is installed
if ! command -v oapi-codegen &> /dev/null; then
    echo "   Installing oapi-codegen..."
    go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
fi

# oapi-codegen doesn't support OpenAPI 3.1, use the 3.0 version
oapi-codegen -generate types,client \
    -package parallelworks \
    ./openapi-3.0.json > ./go/client.go

cd "$ROOT_DIR/go"
go mod tidy
go fmt ./...
echo "✅ Go SDK generated"
echo ""

# ============ Python ============
echo "🐍 Generating Python SDK..."
cd "$ROOT_DIR"

# Use uv to run openapi-python-client
uvx openapi-python-client generate \
    --path ./openapi.json \
    --output-path ./python/parallelworks_client \
    --meta none \
    --overwrite

echo "✅ Python SDK generated"
echo ""

echo "🎉 All SDKs generated successfully!"
