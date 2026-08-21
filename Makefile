# Components are directories under services/ and tools/ that contain a Makefile.
# Root targets delegate to each component; components own their build/test/install.
COMPONENTS := $(patsubst %/Makefile,%,$(wildcard services/*/Makefile tools/*/Makefile))
RECIPE_TESTS := $(wildcard recipes/*/*-test.sh)

.PHONY: build test test-recipes install-all components

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

install-all:
	@for c in $(COMPONENTS); do $(MAKE) -C "$$c" install || exit 1; done

components:
	@echo $(COMPONENTS)
