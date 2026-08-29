BIN      ?= csl
PREFIX   ?= $(HOME)/code/bin
DEST     := $(PREFIX)/$(BIN)
CMD_PATH := ./cmd/csl

COMPONENT := csl
include ../../buildinfo.mk

.PHONY: build test install resign clean fmt lint

build:
	@go build -ldflags "$(BUILDINFO_LDFLAGS)" -o $(BIN) $(CMD_PATH)

test:
	@go test -timeout 120s ./...

install: build
	@mkdir -p $(PREFIX)
	@cp $(BIN) $(DEST)
	@$(MAKE) resign BIN=$(DEST)
	@echo "installed $(BIN) to $(DEST)"

resign:
	@if [ "$$(uname)" = "Darwin" ]; then \
		xattr -dr com.apple.provenance $(BIN) 2>/dev/null || true; \
		if security find-identity -v -p codesigning 2>/dev/null | grep -q "mad01 Local Signing"; then \
			codesign --force --sign "mad01 Local Signing" -i com.mad01.$(notdir $(BIN)) $(BIN); \
		else \
			codesign --force --sign - $(BIN); \
		fi; \
	fi

clean:
	@rm -f $(BIN)

fmt:
	@golines -m 100 --base-formatter=gofumpt -w .

lint:
	@golangci-lint run ./...
