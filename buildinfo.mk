# Shared build metadata for thismoon components, injected into the
# github.com/mad01/thismoon/buildinfo package at link time.
#
# Include it from a component Makefile after setting COMPONENT (the name the
# component's release tags are prefixed with), then link with the flags:
#
#     COMPONENT := keep
#     include ../../buildinfo.mk
#
#     build:
#     	go build -ldflags "$(BUILDINFO_LDFLAGS)" -o $(BIN) ./cmd/keep
#
# The relative include path holds on fleet machines too: ralph's sources cache
# preserves the repo layout, and component Makefiles always run with the
# component directory as the working directory.
#
# Every git command falls back to an empty value outside a checkout, so a build
# from `git archive` output (the clean-checkout CI gate) still links.

ifndef COMPONENT
$(error COMPONENT must be set before including buildinfo.mk)
endif

BUILDINFO_PKG := github.com/mad01/thismoon/buildinfo

GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
GIT_SHA    := $(shell git rev-parse HEAD 2>/dev/null)
GIT_TAG    := $(shell git describe --tags --match '$(COMPONENT)/v*' --abbrev=0 2>/dev/null)
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

BUILDINFO_LDFLAGS := -X $(BUILDINFO_PKG).Version=$(GIT_COMMIT) \
                     -X $(BUILDINFO_PKG).Commit=$(GIT_SHA) \
                     -X $(BUILDINFO_PKG).Tag=$(GIT_TAG) \
                     -X $(BUILDINFO_PKG).BuildTime=$(BUILD_TIME)
