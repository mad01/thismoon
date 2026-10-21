BIN      ?= belt
PREFIX   ?= $(HOME)/code/bin
DEST     := $(PREFIX)/$(BIN)

COMPONENT := belt
include ../../buildinfo.mk

.PHONY: build install resign clean test test-js tidy fmt lint

build:
	go build -ldflags "$(BUILDINFO_LDFLAGS)" -o $(BIN) ./cmd/belt

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
	$(MAKE) test-js

# The pi package runs under node's test runner. Build and install never need
# node, so a machine without it skips these tests.
test-js:
	@if command -v node >/dev/null 2>&1; then \
		node --test pi/test/*.test.mjs; \
	else \
		echo "belt: node not found, skipping pi package tests"; \
	fi

tidy:
	go mod tidy

fmt:
	golines -m 100 --base-formatter=gofumpt -w .

lint:
	golangci-lint run ./...

clean:
	rm -f $(BIN)
