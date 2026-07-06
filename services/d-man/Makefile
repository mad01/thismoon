BIN      ?= d-man
PREFIX   ?= $(HOME)/code/bin
DEST     := $(PREFIX)/$(BIN)

GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
LDFLAGS    := -X github.com/mad01/thismoon/services/d-man/internal/cli.Version=$(GIT_COMMIT)

.PHONY: build install resign clean test tidy fmt lint

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/d-man

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
	golines -m 100 --base-formatter=gofumpt -w .

lint:
	golangci-lint run ./...

clean:
	rm -f $(BIN)
