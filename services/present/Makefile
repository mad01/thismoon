BIN      ?= present
PREFIX   ?= $(HOME)/code/bin
DEST     := $(PREFIX)/$(BIN)

COMPONENT := present
include ../../buildinfo.mk

IMAGE           ?= ghcr.io/mad01/present:dev
KIND_CLUSTER    ?= kind
# KIND_VERSION is the kind release the CI job installs by checksum; the node
# image below is the one that release ships for this Kubernetes minor.
KIND_VERSION    ?= v0.32.0
KIND_NODE_IMAGE ?= kindest/node:v1.36.1@sha256:3489c7674813ba5d8b1a9977baea8a6e553784dab7b84759d1014dbd78f7ebd5

.PHONY: build install resign clean test tidy fmt lint cache cache-force image kind-test kind-down

build:
	go build -ldflags "$(BUILDINFO_LDFLAGS)" -o $(BIN) ./cmd/present

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

cache:
	@./scripts/cache-assets.sh

cache-force:
	@rm -rf $(HOME)/.config/present/assets
	@./scripts/cache-assets.sh

test:
	go test ./...

tidy:
	go mod tidy

fmt:
	golines -m 100 --base-formatter=gofumpt -w .

lint:
	golangci-lint run ./...

# Container image for the shared instance, built from the repo root for the
# host's architecture and loaded into the local docker daemon.
image:
	docker buildx build --load --platform linux/$$(go env GOARCH) \
	  -f Dockerfile -t $(IMAGE) \
	  --build-arg VERSION=$(GIT_COMMIT) \
	  --build-arg COMMIT=$(GIT_SHA) \
	  --build-arg TAG=$(GIT_TAG) \
	  --build-arg BUILD_TIME=$(BUILD_TIME) \
	  ../..

# End-to-end against a kind cluster: reuses the cluster named KIND_CLUSTER
# when it exists (creates it otherwise), loads the image under the :ci tag
# the kind overlay expects, rolls the deployment, then runs the
# cluster-gated tests through scripts/kind-e2e.sh. Leaves the deployment
# running; kind-down removes it. A kind binary other than KIND_VERSION only
# warns: the node image pin is what keeps CI and this target on one minor.
kind-test: image
	@kind version | grep -q '^kind $(KIND_VERSION) ' || echo "warning: local kind is $$(kind version | cut -d' ' -f2), CI installs $(KIND_VERSION)" >&2
	kind get clusters | grep -qx '$(KIND_CLUSTER)' || kind create cluster --name '$(KIND_CLUSTER)' --image '$(KIND_NODE_IMAGE)'
	docker tag $(IMAGE) ghcr.io/mad01/present:ci
	kind load docker-image ghcr.io/mad01/present:ci --name '$(KIND_CLUSTER)'
	kubectl apply -k deploy/overlays/kind
	kubectl -n present rollout restart deploy/present
	kubectl -n present rollout status deploy/present --timeout=180s
	./scripts/kind-e2e.sh

# Remove the present namespace from the cluster. The CRD stays: removing it
# would take every page in every namespace with it.
kind-down:
	kubectl delete namespace present --ignore-not-found

clean:
	rm -f $(BIN)
