BIN      ?= suspenders
PREFIX   ?= $(HOME)/code/bin
DEST     := $(PREFIX)/$(BIN)

GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
LDFLAGS    := -X github.com/mad01/thismoon/tools/suspenders/internal/cli.Version=$(GIT_COMMIT)

.PHONY: build install resign clean test test-integration tidy fmt lint

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/suspenders

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

test-integration:
	@./tests/integration/test_hook_install/run_test.sh
	@./tests/integration/test_hook_allows_clean/run_test.sh
	@./tests/integration/test_hook_blocks_secret/run_test.sh
	@./tests/integration/test_hook_chain/run_test.sh
	@./tests/integration/test_hook_csl_takeover/run_test.sh
	@./tests/integration/test_hook_update/run_test.sh
	@./tests/integration/test_hook_uninstall/run_test.sh
	@./tests/integration/test_hook_install_all/run_test.sh
	@./tests/integration/test_scan_staged/run_test.sh
	@./tests/integration/test_history_scan/run_test.sh
	@./tests/integration/test_history_clean/run_test.sh

tidy:
	go mod tidy

fmt:
	golines -m 100 --base-formatter=gofumpt -w .

lint:
	golangci-lint run ./...

clean:
	rm -f $(BIN)
