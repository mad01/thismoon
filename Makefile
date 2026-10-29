# Components are directories under services/ and tools/ that contain a Makefile.
# Root targets delegate to each component; components own their build/test/install.
COMPONENTS := $(patsubst %/Makefile,%,$(wildcard services/*/Makefile tools/*/Makefile))
RECIPE_TESTS := $(wildcard recipes/*/*-test.sh)

.PHONY: build test test-recipes sandbox-settings sandbox-verify install-all components

build:
	@for c in $(COMPONENTS); do $(MAKE) -C "$$c" build || exit 1; done

# go list exits non-zero when the module has no packages AND when a package is
# broken — only the first may skip go test; the second must fail loud.
test: test-recipes
	@if out=$$(go list ./... 2>&1); then go test ./...; \
	elif echo "$$out" | grep -q "matched no packages"; then echo "no Go packages yet, skipping go test"; \
	else echo "$$out"; exit 1; fi
	@for c in $(COMPONENTS); do $(MAKE) -C "$$c" test || exit 1; done

test-recipes:
	@for t in $(RECIPE_TESTS); do bash "$$t" || exit 1; done

# Render every access manifest (spec.access in a service-info.yaml, or a
# recipe's access.yaml) to its recipe's <label>.srt.json. The manifest is the
# source of truth (docs/adr/0027); sandbox-verify is the gate CI runs.
sandbox-settings:
	go run ./tools/srtspec/cmd/srtspec generate .
	go run ./tools/srtspec/cmd/srtspec generate examples/dotfiles

# Fail when a committed settings file is not its manifest's render or has no
# manifest, when a component's recipe launches a process without spec.access,
# or when an expansion layer beside a recipe does more than widen. Writes
# nothing; sandbox-settings fixes the renders.
sandbox-verify:
	go run ./tools/srtspec/cmd/srtspec verify .
	go run ./tools/srtspec/cmd/srtspec verify examples/dotfiles

install-all:
	@for c in $(COMPONENTS); do $(MAKE) -C "$$c" install || exit 1; done

components:
	@echo $(COMPONENTS)
