#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$SCRIPT_DIR"

# Parse optional SDK arguments (e.g., `generate.sh go python` to skip ts)
# Valid values: ts, go, python. If none provided, all are generated.
# Pass --push to commit and push the regenerated files when done.
REQUESTED_SDKS=()
PUSH=false
for arg in "$@"; do
    case "$arg" in
        --push)
            PUSH=true
            ;;
        ts | go | python)
            REQUESTED_SDKS+=("$arg")
            ;;
        *)
            echo "Unknown argument: $arg" >&2
            echo "Usage: generate.sh [ts] [go] [python] [--push]" >&2
            exit 1
            ;;
    esac
done

should_generate() {
    local sdk="$1"
    if [[ ${#REQUESTED_SDKS[@]} -eq 0 ]]; then
        return 0 # no filter, generate all
    fi
    for req in "${REQUESTED_SDKS[@]}"; do
        if [[ "$req" == "$sdk" ]]; then
            return 0
        fi
    done
    return 1
}

echo "Generating SDKs from openapi.json..."
echo ""

# ============ Generate OpenAPI spec ============
echo "Generating OpenAPI spec from Go ingress..."
cd "$ROOT_DIR/cmd/ingress"
go run ./internal/routes/generate/generate-openapi.go
cd "$ROOT_DIR"
echo "OpenAPI spec generated"
echo ""

# ============ TypeScript ============
if should_generate ts; then
    echo "Generating TypeScript client..."
    cd "$ROOT_DIR"
    pnpm -F @parallelworks/client generate-types
    echo "TypeScript client generated"
    echo ""
fi

# ============ Go ============
if should_generate go; then
    echo "Generating Go SDK..."
    cd "$SCRIPT_DIR"

    # Uses the version pinned in go.mod via `go get -tool`
    cd "$ROOT_DIR"
    go tool github.com/parallelworks/openapi-client-generator generate \
        -s sdk/openapi.json \
        -o sdk/go \
        -p parallelworks \
        --user-agent parallelworks-go-sdk/7

    cd "$SCRIPT_DIR/go"
    go mod tidy
    go fmt ./...
    echo "Go SDK generated"
    echo ""
fi

# ============ Python ============
if should_generate python; then
    echo "Generating Python SDK..."
    cd "$SCRIPT_DIR"

    # Save hand-written files before generation overwrites them
    PY_BACKUP="$(mktemp -d)"
    trap 'rm -rf "$PY_BACKUP"' EXIT
    cp ./python/parallelworks_client/__init__.py "$PY_BACKUP/__init__.py"
    cp ./python/parallelworks_client/auth.py "$PY_BACKUP/auth.py"

    # Use uv to run openapi-python-client
    uvx openapi-python-client generate \
        --path ./openapi.json \
        --output-path ./python/parallelworks_client \
        --meta none \
        --overwrite

    # Restore hand-written files that the generator overwrites
    mv "$PY_BACKUP/__init__.py" ./python/parallelworks_client/__init__.py
    mv "$PY_BACKUP/auth.py" ./python/parallelworks_client/auth.py

    echo "Python SDK generated"
    echo ""
fi

echo "All SDKs generated successfully!"

# ============ Push ============
if [[ "$PUSH" == true ]]; then
    echo ""
    echo "Committing and pushing regenerated files..."
    cd "$ROOT_DIR"

    PATHS_TO_ADD=("sdk/openapi.json")
    should_generate ts && PATHS_TO_ADD+=("sdk/typescript")
    should_generate go && PATHS_TO_ADD+=("sdk/go")
    should_generate python && PATHS_TO_ADD+=("sdk/python")

    git add "${PATHS_TO_ADD[@]}"
    if git diff --cached --quiet; then
        echo "No SDK changes to push"
    else
        git commit -m "chore: regenerate SDKs"
        git push
        echo "Pushed regenerated SDK files"
    fi
fi
