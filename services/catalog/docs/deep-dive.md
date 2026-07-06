# catalog deep-dive

The entity model, registry, validate flow, and how to work on catalog by hand.

## Entity model

catalog uses two kinds: **System** and **Component**. A System is a logical grouping declared at a repo root. A Component is a buildable or deployable unit (a CLI, MCP server, library, app, or service) that links back to its System via `spec.system`.

Names must be globally unique across all Systems and Components combined — a System and a Component can't share a name. When a repo name and its binary would collide, keep the System as the repo name and give the Component a distinct form: use the binary name if it differs (`kitty-session` → `ks`) or a `-cli` suffix otherwise (`ralph` → `ralph-cli`).

## service-info.yaml

Every catalogued entity lives in a `service-info.yaml` file. A single file may hold multiple documents separated by `---`, which is the typical pattern for a single-tool repo that declares its System and Component together.

**Required fields (all entities):**
- `apiVersion: catalog.mad01/v1alpha1`
- `kind: System | Component`
- `metadata.name` — alphanumeric, `-`, `_`, `.`; 1–63 characters; globally unique
- `spec.owner`

**Required for Components only:**
- `spec.system` — the name of the parent System

**Optional fields:**
- `metadata.description`
- `metadata.tags` — YAML list of strings
- `spec.type` — `cli | mcp-server | library | app | service`
- `spec.lifecycle` — `production | deprecated | experimental`

Name format (from source): alphanumeric start and end, with `-`, `_`, `.` allowed in the middle; max 63 characters.

Full example:

```yaml
apiVersion: catalog.mad01/v1alpha1
kind: System
metadata:
  name: dotfiles
  description: Personal dev-tooling monorepo
  tags: [monorepo, tooling]
spec:
  owner: mad01
---
apiVersion: catalog.mad01/v1alpha1
kind: Component
metadata:
  name: present
  tags: [go, cli, mcp]
spec:
  type: cli
  lifecycle: production
  owner: mad01
  system: dotfiles
```

## Registry

catalog discovers entities by walking the repos listed in a registry file. The default location is `~/.config/catalog/registry.yaml`. On this host that file is symlinked from `dotfiles/recipes/catalog/registry.yaml` — edit it there, not in place.

```yaml
sources:
  - path: ~/code/src/github.com/mad01/dotfiles
  - path: ~/code/src/github.com/mad01/thismoon
```

Paths may use `~`; catalog expands them at load time.

## validate

`catalog validate` parses every `service-info.yaml` it finds, checks the schema, then enforces global name uniqueness.

```sh
# validate the whole registry
catalog validate

# validate specific dirs or files (what CI does)
catalog validate ./path/to/repo
catalog validate ./path/to/service-info.yaml
```

Output on success: `ok: N entities valid, names globally unique`.

On failure the error names every duplicate, the kind, and the source file for each occurrence — read it to find which file to fix.

**What validate checks:**
1. Known kind (`System` or `Component`)
2. `metadata.name` present, ≤63 chars, matches `^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$`
3. `spec.owner` present
4. `spec.system` present for Components
5. All names globally unique across every entity in scope

## Adding an entity

**New tool in the dotfiles monorepo:** add a `service-info.yaml` to the tool's subdirectory with `kind: Component` and `spec.system: dotfiles`. No registry change needed — dotfiles is already a registered source.

**New standalone repo:** add a root `service-info.yaml` with both a `System` and a `Component` doc (separated by `---`). Then register the repo path in both `catalog/registry.yaml` (the sample shipped with this repo) and `dotfiles/recipes/catalog/registry.yaml` (the live registry).

Then validate and refresh:

```sh
catalog validate ./path/to/new/repo
# then refresh the running UI (see below)
```

## Removing an entity

- **Tool deleted from a repo:** delete its `service-info.yaml`, then refresh.
- **Whole repo retired:** delete its `service-info.yaml` and remove its path from both registries, then refresh.
- **Winding down but staying put:** set `spec.lifecycle: deprecated` instead of deleting — it shows up but reads as retired.

## Refreshing the UI

After any registry or entity change, the running catalog server needs to re-scan. Three ways:
- Click **Refresh** in the web UI
- `POST /api/refresh`
- `t-man restart catalog-web`

The **Add** form in the UI writes a new `service-info.yaml` into a registered repo and refreshes automatically. It doesn't register new repo paths — do that by hand for a new repo.

## Working on catalog

### Build and run locally

```sh
make install   # builds and installs to ~/code/bin/catalog
catalog web    # serves on http://127.0.0.1:7575 (loopback only)
```

Without installing:

```sh
make build
./catalog web
```

The web UI is at `http://catalog.this/` when the t-man service is running and d-man is routing that hostname.

### Tests and linting

```sh
make test   # go test ./... with the race detector
make lint   # golangci-lint
make fmt    # gofmt -w .
```

The functional core in `internal/catalog/` is pure Go with no I/O — tests run fast and deterministically.

### Debugging a validation failure

`catalog validate` prints the error with the duplicate name, its kind, and the source file path for each occurrence. Common causes:

- **Duplicate name across repos:** two `service-info.yaml` files declare the same `metadata.name`. Rename one. Convention: if a System and its binary clash, add `-cli` or use the short binary name (`csl`, `ks`).
- **Wrong `spec.system`:** a Component references a System name that doesn't exist or is misspelled. `catalog list` shows all Systems to cross-check.
- **Missing `spec.owner`:** required on both kinds.
- **Invalid name characters:** only `[a-zA-Z0-9._-]` with alphanumeric at both ends; hyphens and dots are allowed mid-name.

### Architecture

| Package | Role |
|---------|------|
| `internal/catalog/` | Pure functional core: parse, scan, index, query, uniqueness. No I/O. |
| `internal/cli/` | Cobra commands: `web`, `list`, `validate`, `version`. |
| `internal/web/` | HTTP shell, JSON API, embedded vanilla HTML/CSS/JS frontend. |

The frontend is hand-written vanilla HTML/CSS/JS with no build toolchain. The only third-party JavaScript is Cytoscape.js, vendored at `internal/web/assets/static/cytoscape-3.31.0.min.js`. To bump it, replace that file and update the `<script>` tag in `assets/index.html`.

The shared header, theme toggle, and controls come from `github.com/mad01/thismoon/webkit` — don't add them locally.

### Further reading

- Authoring rules and agent-facing context: [`CLAUDE.md`](../CLAUDE.md) and the `Catalog it` section in the dotfiles root `CLAUDE.md`.
- Live registry: `dotfiles/recipes/catalog/registry.yaml`
