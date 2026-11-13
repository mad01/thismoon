BIN      ?= brain
PREFIX   ?= $(HOME)/code/bin
DEST     := $(PREFIX)/$(BIN)

COMPONENT := brain
include ../../buildinfo.mk

.PHONY: build install resign clean test tidy fmt lint bundle bundle-local test-js

# The graph bundle the UI embeds: web/ builds it, the ui package serves it.
WEB    := web
ASSETS := internal/server/ui/assets
BUNDLE := brain-graph.bundle.js

build:
	go build -ldflags "$(BUILDINFO_LDFLAGS)" -o $(BIN) ./cmd/brain

install: build
	mkdir -p $(PREFIX)
	cp $(BIN) $(DEST)
	$(MAKE) resign BIN=$(DEST)

resign:
	@if [ "$$(uname)" = "Darwin" ]; then \
		xattr -c $(BIN); \
		if security find-identity -v -p codesigning 2>/dev/null | grep -q "mad01 Local Signing"; then \
			codesign --force --sign "mad01 Local Signing" -i com.mad01.$(notdir $(BIN)) $(BIN); \
		else \
			codesign --force --sign - $(BIN); \
		fi; \
	fi

test:
	go test ./...

tidy:
	go mod tidy

fmt:
	golines -m 100 --base-formatter=gofumpt --no-reformat-tags -w .

lint:
	golangci-lint run ./...

# Apple container's local exporter writes to <dest>/<platform>/ (e.g.
# linux_arm64), so export to a staging dir, then copy only the two artifacts.
bundle: ## sandboxed build: npm ci + esbuild in a throwaway Apple-container VM
	rm -rf $(WEB)/.build-out
	cd $(WEB) && container build -f Dockerfile.build --target export \
	  --output type=local,dest=./.build-out .
	cp $(WEB)/.build-out/*/$(BUNDLE)  $(ASSETS)/$(BUNDLE)
	cp $(WEB)/.build-out/*/SHA256SUMS $(ASSETS)/SHA256SUMS
	rm -rf $(WEB)/.build-out
	@cat $(ASSETS)/SHA256SUMS

bundle-local: ## fallback: build on the host (only if the container runtime is unavailable)
	cd $(WEB) && npm ci && npm run build
	cp $(WEB)/dist/$(BUNDLE)  $(ASSETS)/$(BUNDLE)
	cp $(WEB)/dist/SHA256SUMS $(ASSETS)/SHA256SUMS
	@cat $(ASSETS)/SHA256SUMS

test-js: ## node tests over the UI's pure helpers
	node --test internal/server/ui/test/*.test.mjs

clean:
	rm -f $(BIN)
