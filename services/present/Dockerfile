# syntax=docker/dockerfile:1
# present as a shared instance. The build context is the repo root: one Go
# module, so the service needs go.mod, kit/, webkit/, and buildinfo/ beside
# it. The build stage runs on the build platform and cross-compiles for
# TARGETARCH, so a multi-arch image needs no emulation; the final stage only
# copies files.
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm@sha256:a688600ca24f8a4d3ca77f95b0dd40704a9fc787c826660eb7ba0b641b8b175d AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
ARG COMMIT=""
ARG TAG=""
ARG BUILD_TIME=""
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
# Same ldflags as buildinfo.mk and release.yml, so /version and
# `present version -o json` report the four-key object like every build.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags "-X github.com/mad01/thismoon/buildinfo.Version=$VERSION \
                -X github.com/mad01/thismoon/buildinfo.Commit=$COMMIT \
                -X github.com/mad01/thismoon/buildinfo.Tag=$TAG \
                -X github.com/mad01/thismoon/buildinfo.BuildTime=$BUILD_TIME" \
      -o /out/present ./services/present/cmd/present
# The page shell loads fonts and the graph and chart libraries from
# /assets, which local installs cache into the workdir. The image bakes
# them in, so a pod needs no network and no writable directory.
RUN PRESENT_WORKDIR=/out/state ./services/present/scripts/cache-assets.sh

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/present /present
COPY --from=build /out/state/assets /var/lib/present/assets
ENV PRESENT_WORKDIR=/var/lib/present
EXPOSE 7423
ENTRYPOINT ["/present"]
CMD ["serve", "--shared", "--store", "k8s", "--bind", "0.0.0.0"]
