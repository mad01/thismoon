.PHONY: build build-local check test lint fmt

# Apple container's local exporter writes to <dest>/<platform>/ (e.g.
# linux_arm64) — export to a staging dir, then copy only the build artifacts
# into dist/.
build: ## sandboxed build: npm ci + npm run build in a throwaway Apple-container VM
	rm -rf .build-out
	container build -f Dockerfile.build --target export \
	  --output type=local,dest=./.build-out .
	cp .build-out/*/webkit.js  dist/webkit.js
	cp .build-out/*/webkit.css dist/webkit.css
	cp .build-out/*/boot.js    dist/boot.js
	rm -rf .build-out
	@echo "webkit: dist/ rebuilt in container — review 'git diff dist/' before commit"

build-local: ## fallback: build on host (only if the container runtime is unavailable)
	npm run build

check: ## typecheck only (host)
	npm run check

test: ## node unit tests + go embed tests (host)
	npm test
	go test ./...

fmt: ## format Go (gofumpt + goimports + golines)
	gofumpt -w .
	goimports -w .
	golines -m 100 --base-formatter=gofumpt -w .

lint: ## golangci-lint over the Go package
	golangci-lint run ./...
