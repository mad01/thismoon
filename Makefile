# Components are directories under services/ and tools/ that contain a Makefile.
# Root targets delegate to each component; components own their build/test/install.
COMPONENTS := $(patsubst %/Makefile,%,$(wildcard services/*/Makefile tools/*/Makefile))

.PHONY: build test install-all components

build:
	@for c in $(COMPONENTS); do $(MAKE) -C "$$c" build || exit 1; done

# go test exits non-zero when the module has no packages, so skip it until Go code lands
test:
	@if [ -n "$$(go list ./... 2>/dev/null)" ]; then go test ./...; else echo "no Go packages yet, skipping go test"; fi
	@for c in $(COMPONENTS); do $(MAKE) -C "$$c" test || exit 1; done

install-all:
	@for c in $(COMPONENTS); do $(MAKE) -C "$$c" install || exit 1; done

components:
	@echo $(COMPONENTS)
