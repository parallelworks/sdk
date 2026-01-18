.PHONY: generate publish publish-ts publish-py publish-go publish-ci help

VERSION ?= $(error VERSION is required. Usage: make publish VERSION=0.1.0)
TAG = v$(VERSION)

help:
	@echo "Usage:"
	@echo "  make generate              # Regenerate all SDK clients"
	@echo "  make publish VERSION=0.1.0 # Publish all packages"
	@echo "  make publish-ts VERSION=0.1.0"
	@echo "  make publish-py VERSION=0.1.0"
	@echo "  make publish-go VERSION=0.1.0"
	@echo "  make publish-ci VERSION=0.1.0  # Trigger GitHub Actions"

generate:
	./scripts/generate.sh

publish: publish-ts publish-py publish-go
	@echo ""
	@echo "✅ All packages published: $(TAG)"

publish-ts:
	@echo "📦 Publishing TypeScript $(TAG)..."
	cd typescript && npm version $(VERSION) --no-git-tag-version
	cd typescript && pnpm install && pnpm build
	cd typescript && pnpm publish --no-git-checks
	@echo "✅ npm install @parallelworks/client@$(VERSION)"

publish-py:
	@echo "📦 Publishing Python $(TAG)..."
	cd python && perl -i -pe 's/^version = .*/version = "$(VERSION)"/' pyproject.toml
	cd python && perl -i -pe 's/__version__ = .*/__version__ = "$(VERSION)"/' parallelworks_client/__init__.py
	cd python && uv build && uv publish
	@echo "✅ pip install parallelworks-client==$(VERSION)"

publish-go:
	@echo "📦 Publishing Go $(TAG)..."
	git add -A
	git commit -m "chore: release $(TAG)" || true
	git tag go/$(TAG)
	git push
	git push origin go/$(TAG)
	@echo "✅ go get github.com/parallelworks/sdk/go@$(TAG)"

publish-ci:
	@echo "🚀 Triggering GitHub Actions for $(TAG)..."
	gh workflow run generate-sdks.yml -f version=$(TAG)
	@echo "✅ Workflow triggered"
