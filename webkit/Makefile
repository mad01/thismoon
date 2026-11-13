.PHONY: build build-local check test test-local lint fmt

# Apple container's local exporter writes to <dest>/<platform>/ (e.g.
# linux_arm64) — export to a staging dir, then copy only the build artifacts
# into dist/.
build: ## sandboxed build: npm ci + npm run build in a throwaway Apple-container VM
	rm -rf .build-out
	container build -f Dockerfile.build --target export \
	  --output type=local,dest=./.build-out .
	cp .build-out/*/webkit.js   dist/webkit.js
	cp .build-out/*/webkit.css  dist/webkit.css
	cp .build-out/*/boot.js     dist/boot.js
	cp .build-out/*/themes.json dist/themes.json
	cp .build-out/*/themes.html dist/themes.html
	cp .build-out/*/themes.js   dist/themes.js
	rm -rf .build-out
	@echo "webkit: dist/ rebuilt in container — review 'git diff dist/' before commit"

build-local: ## fallback: build on host (only if the container runtime is unavailable)
	npm run build

check: ## typecheck only (host)
	npm run check

test: ## in a throwaway Apple-container VM: committed dist/ must match a fresh build, then node tests (unit + DOM) on it; then go embed tests (host)
	rm -rf .test-out
	container build -f Dockerfile.build --target test-export \
	  --output type=local,dest=./.test-out .
	rm -rf .test-out
	go test ./...

test-local: ## fallback: every test on the host (DOM tests need `npm ci` on the host first)
	npm test
	npm run test:dom
	go test ./...

fmt: ## format Go (gofumpt + goimports + golines)
	gofumpt -w .
	goimports -w .
	golines -m 100 --base-formatter=gofumpt -w .

lint: ## golangci-lint over the Go package
	golangci-lint run ./...
