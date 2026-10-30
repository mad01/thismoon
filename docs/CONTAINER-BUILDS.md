# Containerized builds with Apple container

Companion to [docs/SANDBOXING.md](SANDBOXING.md). srt confines a process
that has to run on the host. This page covers the other half of the
supply-chain story: build steps that run third-party install-time code (npm
postinstall scripts, pip sdist builds, gem extensions) and need no host
hardware. Those run in a throwaway VM via
[Apple container](https://github.com/apple/container), and only the named
artifacts come back out.

Reference implementation: [`webkit/Makefile`](../webkit/Makefile). Its
`make build` runs `npm ci` and `npm run build` inside a container and copies
the built files into `dist/`.

## When a container is the right tool

Use a container VM when all of these hold:

- The step executes third-party code at install or build time. Any package
  manager fetch counts: postinstall hooks run arbitrary code with your
  user's privileges.
- It needs no host hardware. A Linux VM has no Metal, so anything that
  needs the GPU belongs on the host under a seatbelt profile instead
  ([the legacy path](SANDBOXING.md#the-legacy-path-a-seatbelt-profile-under-t-man)).
- Its output is a set of files. Builds produce artifacts; services hold
  ports and state, which is t-man and srt territory.

The two tools compose: webkit's `dist/` is built in a container, while the
runtime processes that serve it run under srt.

## The extraction pattern

The whole point is one-way flow: source goes in via the build context,
artifacts come out via the exporter, and the host filesystem is never
mounted into the VM.

```dockerfile
FROM node:26-bookworm-slim AS deps
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci

FROM deps AS build
COPY tsconfig.json build.mjs ./
COPY src ./src
RUN npm run build

# Export stage: only the named artifacts.
FROM scratch AS export
COPY --from=build /app/dist /
```

```make
build:
	rm -rf .build-out
	container build -f Dockerfile.build --target export \
	  --output type=local,dest=./.build-out .
	cp .build-out/*/webkit.js  dist/webkit.js
	cp .build-out/*/webkit.css dist/webkit.css
	rm -rf .build-out
```

The rules baked into that shape:

- **Multi-stage with a `scratch` export stage.** The exporter dumps the
  final stage's filesystem; `FROM scratch` plus `COPY --from=build` means
  only the artifacts you name leave the VM.
- **`--output type=local`** (the BuildKit local exporter) is the only exit
  path. Never bind-mount source or output directories.
- **Explicit `COPY`s, never `COPY . .`.** This keeps the build context
  tight regardless of `.dockerignore` handling (apple/container#425), and
  keeps untracked host files (credentials, scratch dirs) out of the VM.
- **Export to a fresh staging dir, then copy.** The local exporter's layout
  depends on the destination's prior state: a fresh destination gives
  `<dest>/<platform>/` (such as `linux_arm64/`), an existing one gives
  `<dest>/out.tar/<platform>/`. Delete the staging dir first and glob the
  platform component.
- **Keep a `build-local` fallback** for machines without the runtime, and
  know that it builds on the host, unsandboxed. Review `git diff dist/`
  before committing either way.

## Runtime setup (manual prerequisite)

No recipe installs the `container` runtime; it is a by-hand step on each
machine:

- Install the signed pkg from
  [apple/container releases](https://github.com/apple/container/releases);
  it lands root-owned under `/usr/local`: the `container` CLI, the
  `container-apiserver`, and the runtime plugins in `libexec/container`.
  Don't use mise's `container` entry: it copies the CLI out of that pkg
  and leaves the apiserver behind.
- First run needs a Linux kernel:
  `container system start --enable-kernel-install` does it
  non-interactively.

Without the runtime, `make build` in webkit isn't available and only the
unsandboxed `build-local` path works. Install the pkg rather than getting
comfortable with the fallback.
