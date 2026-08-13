BINARY_NAME := catalog
CMD_PATH := ./cmd/catalog

COMPONENT := catalog
include ../../buildinfo.mk

.PHONY: build test install clean fmt lint tidy

build:
	@go build -ldflags "$(BUILDINFO_LDFLAGS)" -o $(BINARY_NAME) $(CMD_PATH)

test:
	@go test -timeout 30s ./...

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
	@gofmt -w .

tidy:
	@go mod tidy

lint:
	@golangci-lint run ./...
