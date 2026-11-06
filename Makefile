# Components are directories under services/ and tools/ that contain a Makefile.
# Root targets delegate to each component; components own their build/test/install.
COMPONENTS := $(patsubst %/Makefile,%,$(wildcard services/*/Makefile tools/*/Makefile))
RECIPE_TESTS := $(wildcard recipes/*/*-test.sh)

# fmt-changed formats the files that differ from the merge base with this ref.
FMT_BASE ?= origin/main
GOLINES := golines -m 100 --base-formatter=gofumpt

.PHONY: build test test-recipes sandbox-settings sandbox-verify fmt-changed install-all components

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

# Format only the Go files that differ from FMT_BASE: committed on the branch,
# staged, unstaged or untracked. Components are not golines-clean on main, so
# a tree-wide format would rewrap files the change never touched. Deleted
# files, testdata fixtures and generated code (*.pb.go, or a "Code generated
# ... DO NOT EDIT." header) are left alone. Same tool and flags as the
# component fmt targets; prints each file it rewrote.
fmt-changed:
	@for t in golines gofumpt; do command -v $$t >/dev/null || { \
		echo "fmt-changed: $$t not found on PATH; install it with:" >&2; \
		echo "  go install github.com/segmentio/golines@latest" >&2; \
		echo "  go install mvdan.cc/gofumpt@latest" >&2; exit 1; }; done
	@base=$$(git merge-base $(FMT_BASE) HEAD) || exit 1; \
	files=$$({ git -c core.quotePath=false diff --name-only --diff-filter=d $$base -- '*.go'; \
		git -c core.quotePath=false ls-files --others --exclude-standard -- '*.go'; } | sort -u | \
		while IFS= read -r f; do \
			case "$$f" in (*.pb.go|testdata/*|*/testdata/*) continue ;; esac; \
			sed '/^package /q' "$$f" | grep -qE '^// Code generated .* DO NOT EDIT\.$$' && continue; \
			printf '%s\n' "$$f"; \
		done); \
	[ -n "$$files" ] || { echo "fmt-changed: no changed Go files"; exit 0; }; \
	dirty=$$(printf '%s\n' "$$files" | tr '\n' '\0' | xargs -0 $(GOLINES) -l) || exit 1; \
	[ -n "$$dirty" ] || { echo "fmt-changed: changed Go files already formatted"; exit 0; }; \
	printf '%s\n' "$$dirty" | tr '\n' '\0' | xargs -0 $(GOLINES) -w || exit 1; \
	printf '%s\n' "$$dirty" | sed 's/^/fmt-changed: formatted /'

install-all:
	@for c in $(COMPONENTS); do $(MAKE) -C "$$c" install || exit 1; done

components:
	@echo $(COMPONENTS)
