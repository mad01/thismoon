BINARY_NAME := csl
CMD_PATH := ./cmd/csl
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Semantic search uses hugot's onnxruntime (ORT) backend, which links
# libtokenizers.a at build time and loads libonnxruntime.dylib at runtime. The
# `ortlib` target prefetches both into ORTLIB (idempotent), so a clean machine
# builds without manual setup. Build with -tags ORT and point the linker at
# ORTLIB. Unit tests use a fake embedder, so `make test` needs neither the tag
# nor the libs.
ORTLIB := $(HOME)/.local/share/csl/ortlib
ORT_TAGS := -tags ORT
ORT_ENV := CGO_LDFLAGS="-L$(ORTLIB)"

.PHONY: build ortlib test install clean fmt lint

ortlib:
	@ORTLIB="$(ORTLIB)" ./scripts/fetch-ortlib.sh

build: ortlib
	@$(ORT_ENV) go build $(ORT_TAGS) \
		-ldflags "-X github.com/mad01/thismoon/services/csl/internal/cli.Version=$(GIT_COMMIT)" \
		-o $(BINARY_NAME) $(CMD_PATH)

test:
	@go test -timeout 120s ./...

install: build
	@mkdir -p ~/code/bin
	@cp $(BINARY_NAME) ~/code/bin/$(BINARY_NAME)
	@xattr -dr com.apple.provenance ~/code/bin/$(BINARY_NAME) 2>/dev/null || true
	@if security find-identity -v -p codesigning 2>/dev/null | grep -q "mad01 Local Signing"; then \
		codesign -f -s "mad01 Local Signing" -i com.mad01.$(BINARY_NAME) ~/code/bin/$(BINARY_NAME); \
	else \
		codesign -fs - ~/code/bin/$(BINARY_NAME); \
	fi
	@echo "installed $(BINARY_NAME) to ~/code/bin/$(BINARY_NAME)"

clean:
	@rm -f $(BINARY_NAME)

fmt:
	@golines -m 100 --base-formatter=gofumpt -w .

lint:
	@golangci-lint run ./...
