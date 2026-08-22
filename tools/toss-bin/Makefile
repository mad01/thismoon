BIN      ?= toss-bin
PREFIX   ?= $(HOME)/.local/bin
DEST     := $(PREFIX)/$(BIN)

.PHONY: build install resign clean test test-integration gen-operating-doc

# Regenerate the embedded operating doc before every compile so the
# constant can never lag operating.md. The generated file is committed;
# a plain `swift build` still works without this step.
gen-operating-doc:
	./scripts/gen-operating-doc.sh

build: gen-operating-doc
	swift build --configuration=release

# Installs to ~/.local/bin (not ~/code/bin like the Go tools) — the path the
# shell rm wrapper has always resolved toss-bin from.
install: build
	mkdir -p $(PREFIX)
	cp .build/release/$(BIN) $(DEST)
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

test: gen-operating-doc
	swift test

test-integration:
	./test

clean:
	swift package clean
