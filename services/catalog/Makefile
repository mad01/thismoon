BINARY_NAME ?= catalog
CMD_PATH := ./cmd/catalog
PREFIX ?= $(HOME)/code/bin
DEST := $(PREFIX)/$(BINARY_NAME)

COMPONENT := catalog
include ../../buildinfo.mk

.PHONY: build test install clean fmt lint tidy

build:
	@go build -ldflags "$(BUILDINFO_LDFLAGS)" -o $(BINARY_NAME) $(CMD_PATH)

test:
	@go test -timeout 30s ./...

install: build
	@mkdir -p $(PREFIX)
	@cp $(BINARY_NAME) $(DEST)
	@xattr -dr com.apple.provenance $(DEST) 2>/dev/null || true
	@if security find-identity -v -p codesigning 2>/dev/null | grep -q "mad01 Local Signing"; then \
		codesign -f -s "mad01 Local Signing" -i com.mad01.$(BINARY_NAME) $(DEST); \
	else \
		codesign -fs - $(DEST); \
	fi
	@echo "installed $(BINARY_NAME) to $(DEST)"

clean:
	@rm -f $(BINARY_NAME)

fmt:
	@gofmt -w .

tidy:
	@go mod tidy

lint:
	@golangci-lint run ./...
