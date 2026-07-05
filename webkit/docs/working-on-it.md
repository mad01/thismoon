# Working on webkit

Build pipeline, dev loop, adding components, and shipping changes to consumers.

## Prerequisites

- [apple/container](https://github.com/apple/container/releases) — the default build runs inside a throwaway Linux VM; `container system start` before `make build`
- Node.js (any recent LTS) — needed for `make build-local` and `npm test`
- Go — for `go test ./...`

## Build

```bash
make build        # sandboxed: npm ci + npm run build inside a throwaway VM
make build-local  # fallback when the container runtime is unavailable
```

The sandboxed build is the default. `npm ci` runs arbitrary install-time code (postinstall scripts, esbuild, tsc, Prism and transitive deps), so the VM keeps those off the host. Only `dist/webkit.js` and `dist/webkit.css` come out, byte-identical to a host build.

`dist/` is **committed** — Go consumers embed it via `//go:embed` and need it present without a node toolchain.

After building, always check the diff before committing:

```bash
git diff dist/
```

## Tests

```bash
npm test        # node --test: pure-function unit tests for toBionic, clampSize
go test ./...   # asserts Handler() serves embedded CSS/JS; checks wk-header and wk-card presence
make check      # tsc --noEmit typecheck only (host, no build)
```

## Dev loop

1. Edit `src/webkit.css` (component styles) and/or `src/webkit.ts` (behaviour).
2. Rebuild: `make build` (sandboxed) or `make build-local` (host).
3. Review `git diff dist/` — the bundle rides into every consumer binary via `go:embed`.
4. Open `examples/gallery.html` directly in a browser (no server needed) to see all components.
5. If a consumer is running locally, `t-man restart <service>` picks up the new binary after `make install` there.

## Adding a component

1. Add CSS rules to `src/webkit.css` — tag and attribute selectors only, no Shadow DOM.
2. Register the element as a no-op in `src/webkit.ts` by adding its tag to the `NOOP_ELEMENTS` array. Give it its own constructor class; don't share a superclass (each element stays independently removable).
3. Add an example to `examples/gallery.html`.
4. Document it in `docs/components.md`.
5. Add an assertion in `embed_test.go` that the new token appears in `webkit.css`.
6. Rebuild and review `git diff dist/`.

## The `go:embed` contract

`embed.go` exposes:

- `webkit.Handler() http.Handler` — mount at `GET /webkit/` in any consumer
- `webkit.FS() fs.FS` — for direct file access
- `GET /webkit/webkit.css` and `GET /webkit/webkit.js` — the compiled assets
- `GET /webkit/version` — JSON `{"module":"github.com/mad01/thismoon/webkit","version":"<hash>"}` where `<hash>` is a short SHA-256 over the embedded `dist/` bytes, served `no-store`; the same hash is the asset `ETag`

`webkit.NoCacheHTML(w http.ResponseWriter)` sets `Cache-Control: no-cache` on consumer HTML responses so a webkit bump shows up on the next page navigation rather than after a force-refresh.

`webkit.js` polls `/webkit/version` every ~5s and reloads the page when the hash changes, so open pages pick up a redeploy without a manual refresh.

## Shipping changes to all consumers

Consumers compile against the committed webkit source — merging to `main` is the bump. To roll a running service onto the new assets:

```bash
make install         # rebuild and install the consumer binary
t-man restart <service>   # restart the running service
```

Verify the new assets are live:

```bash
curl http://<tool>.this/webkit/version
```

All consumers should report the same hash. The current consumer table:

| Consumer | Repo | t-man service |
|----------|------|---------------|
| present  | dotfiles `present/` | `present` |
| speak    | dotfiles `speak/` | `speak-web` |
| status   | dotfiles `status/` | `status` |
| csl      | code-search-local | `csl-web` |
| catalog  | catalog | `catalog-web` |

Dotfiles consumers (present, speak, status) can share a single commit for their `go.mod` bumps. Each standalone repo (csl, catalog) gets its own commit.

Keep the consumer table above in sync when consumers are added or removed — the authoritative copy is in `CLAUDE.md`.

## FOUC guard

Inline this snippet in `<head>` before any stylesheet on every consumer page. It reads `webkit-theme` and `webkit-size` before paint so persisted preferences don't flash:

```html
<script>(function(){try{var t=localStorage.getItem('webkit-theme')||'light';document.documentElement.setAttribute('data-theme',t);var s=parseInt(localStorage.getItem('webkit-size'),10);if(!isNaN(s)){s=Math.max(12,Math.min(24,s));document.documentElement.style.fontSize=s+'px';}}catch(e){}})();</script>
```

The same string is available at runtime as `Webkit.bootSnippet` — server templates can inject it as a single source of truth rather than copying it by hand.
