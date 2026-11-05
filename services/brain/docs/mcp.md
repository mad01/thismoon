# `brain mcp`: MCP stdio server reference

`brain mcp` is a [Model Context Protocol](https://modelcontextprotocol.io) stdio server bundled into the `brain` binary. It exposes your vaults (search, read, list, backlinks, graph, write, edit, promote) as native MCP tools so Claude Code can operate on your notes without shelling out through `Bash()`. The config names one vault with `vault:`, or several private and shared vaults with a `vaults:` list.

There is no long-running server. The MCP client spawns `brain mcp` as a subprocess per session, pipes JSON-RPC 2.0 over stdin/stdout, and the process exits when stdin closes. Each tool handler is a thin adapter over the internal `library`, `search`, `vault`, and `markdown` packages that back the cobra commands.

## Registration

```bash
claude mcp add --scope user brain -- brain mcp
```

Registering it across machines is machine-private wiring, so it belongs in your own config repo rather than in thismoon (see docs/adr/0006 at the repo root).

Verify after registering:

```bash
claude mcp list
# expect: brain: brain mcp - ✓ Connected
```

The vaults come from the config file `brain mcp` resolved at startup (`--config`, then `BRAIN_CONFIG`, then `config.yaml` under `$XDG_CONFIG_HOME/brain` or `~/.config/brain`): one `vault: <path>`, or a `vaults:` list of private and shared vaults. Every tool call reads that file again, so a config edit applies to the next call. Each vault has its own indexes in the state directory, `$XDG_STATE_HOME/brain` or `~/.local/state/brain`, under `index/<vault>/` and `semantic/<vault>/`; an install that already built an index under `~/.config/brain` keeps it there. `config.md` beside the service has the full rules. Each vault is its own repo, checked out wherever its path points. Keeping vaults outside thismoon means the tool's history stays clean of personal-knowledge churn.

## Vault model

Each vault is a directory with four top-level tiers:

| Tier | Directory | Default `type` | Default `status` | Purpose |
|---|---|---|---|---|
| inbox | `inbox/` | `note` | `raw` | Raw captures, unprocessed |
| notes | `notes/` | `note` | `validated` | First-class knowledge |
| outputs | `outputs/` | `output` | `validated` | Finished artifacts (filenames are date-prefixed `YYYY-MM-DD-…`) |
| MOCs | `MOCs/` | `moc` | `validated` | Maps of Content (curated indexes) |

Every note is a markdown file with YAML frontmatter:

```yaml
---
title: Example
aliases: [Other name]
tags: [kubernetes, ops]
type: note
status: validated
source: https://example.com
evidence: B           # optional; trust tier for empirical notes (A/B/C/D/mixed)
created: 2025-01-02T03:04:05Z
updated: 2025-01-02T03:04:05Z
id: 01JABCDEFGHJKMNPQRSTVWXYZ0
---
body text with [[wikilinks]]
```

### Vaults, note ids and note refs

Several vaults can be configured. A private vault is one you write directly. A shared vault (`shared: true`) is one a team keeps in its own repo.

- A shared vault is read-only here: `brain_write`, `brain_edit` and `brain_promote` fail on it with a read-only error. Its notes change through a pull request.
- Writes go to the default vault (the one marked `default: true`, else the first private vault) unless the `vault` input names another.
- Search (literal, semantic and hybrid), count, list and graph cover every vault unless the `vault` input narrows them. Every hit and note result names its `vault`.

A note id is the Universally Unique Lexicographically Sortable Identifier (ULID) in the `id` frontmatter field. `brain_write` assigns one, and `brain_promote` gives one to a note that has none. Neither edit nor promote ever changes an id, so it names the note when its path changes. `brain migrate --ids` backfills notes written before ids existed.

A note ref names one note in a `path` or `center` input:

| Form | Example | Names |
|---|---|---|
| `vault:path` | `team:notes/topic.md` | The note at that path in that vault |
| `id:<ulid>` | `id:01JABCDEFGHJKMNPQRSTVWXYZ0` | The note whose `id` is that ULID |
| bare path | `notes/topic.md` | The note at that vault-relative path, when exactly one vault has it |

A bare path found in more than one vault fails as ambiguous, and the error lists the qualified `vault:path` candidates. An id two notes share (a copied file) is ambiguous too. On a tool with a `vault` input, setting it limits the lookup to that vault.

### Evidence (trust tier)

Read-side tools (`brain_search`, `brain_read`, `brain_peek`, `brain_list_tier`, `brain_backlinks`, `brain_graph`) all surface an `evidence` field so callers can reason about a note's correctness level without string-matching tags.

| Value | Meaning |
|---|---|
| `A` | Replicated RCT / meta-analysis with named effect size + CI |
| `B` | Single well-designed peer-reviewed study, reasonable N |
| `C` | Preprint, survey, mechanism-plausible, category-evidence extended |
| `D` | Refuted, apocryphal or null under blinding, kept as a do-not-cite record |
| `mixed` | Composite note; inline per-claim grades in body |
| (empty) | Non-empirical note (profile, MOC, tool reference) |

The field is read with a two-step precedence: the explicit `evidence:` frontmatter field wins; otherwise an `evidence/<tier>` tag is parsed as fallback. New notes should prefer the explicit field; the tag form remains supported for legacy notes.

A `tier` input on search / count / list tools translates into a `file:^<tier>/` regex prefix against the zoekt index (e.g. `tier: "notes"` becomes `file:^notes/`). `ParseTier` accepts `inbox`, `notes`, `outputs`, and `moc`/`mocs`/`MOCs` (case-insensitive).

### Note lifecycle

`status` is one of `raw`, `validated`, `promoting`, `disputed`, `superseded`, `retracted` and `withdrawn`. `expired` is computed and never stored: a raw note past `expires_at`, or a validated note past `review_by`. `brain_write` dates a raw note with `expires_at` (30 days out) and a validated one with `review_by` (90 days with `volatility: high`, 365 otherwise). `brain_promote` and a status change through `brain_edit` record `status_at`. The lifecycle fields sit in `frontmatter.extra`.

Superseded, retracted and withdrawn notes are revoked. The CLI revokes a note (`brain supersede`, `brain retract`, `brain withdraw`); there is no MCP tool for it. Every search tool leaves revoked notes out unless `include_revoked` is true or `status` names a revoked status, and the filter runs before `limit`. `brain_read` and `brain_peek` return a `banner` for a revoked or disputed note; show it before quoting the note. A withdrawn note is a tombstone with no body.

`brain_write`, `brain_edit` and `brain_promote` scan the note they would write with the suspenders rules and refuse it, writing nothing, when it holds a secret. The error names the rule and the line and shows the value redacted. A false positive passes with `suspenders:ignore` on its line or an entry in the vault's `.suspenders.yaml`. The CLI exits 8 (`secret_found`) on the same refusal.

`brain check` (CLI only) checks notes against these rules and the vault's `.brain/policy.yaml`; see `brain docs`.

## Tools

Registered tools (fourteen): `brain_search`, `brain_semantic_search`, `brain_hybrid_search`, `brain_count`, `brain_query_validate`, `brain_read`, `brain_peek`, `brain_list_tier`, `brain_backlinks`, `brain_graph`, `brain_write`, `brain_edit`, `brain_promote`, `brain_doctor`.

`brain_semantic_search` and `brain_hybrid_search` take `vault` and `include_revoked` inputs like `brain_search`. Their hits carry `vault`, the note's `id` when it has one, and its `status`, `review_by` and `stale` as the note says now, and their results carry `warnings[]` (see Indexing).

### `brain_search`

Search notes by text with zoekt query syntax, optionally filtered by vault / tier / tag / type / status.

**Input:**

| Field | Type | Required | Description |
|---|---|---|---|
| `query` | string | yes | Zoekt query (see syntax below) |
| `tier` | string | no | Restrict to a single tier (`inbox`, `notes`, `outputs`, `MOCs`) |
| `tag` | string | no | Restrict to notes whose frontmatter `tags` contains this tag |
| `type` | string | no | Restrict to notes with this frontmatter `type` |
| `status` | string | no | Restrict to notes with this frontmatter `status` |
| `lang` | string | no | Restrict to files of this language (usually `markdown`) |
| `file` | string | no | Restrict to vault-relative paths matching this regex |
| `case_sensitive` | bool | no | Force case-sensitive matching; default is smart case |
| `limit` | int | no | Max file results (default `50`) |
| `output_mode` | string | no | `files_with_matches` (default) or `content` |
| `context_lines` | int | no | Context lines around each match; content mode only |
| `vault` | string | no | Restrict to this configured vault; default searches every vault |
| `include_revoked` | bool | no | Include superseded, retracted and withdrawn notes, left out by default |

**Output:**

| Field | Type | Description |
|---|---|---|
| `output_mode` | string | Echoes the mode actually used |
| `files` | array | Unique file paths; set when `output_mode=files_with_matches` |
| `files[].vault` | string | Name of the vault holding the note |
| `files[].path` | string | Vault-relative path |
| `files[].evidence` | string | Trust tier of the note (A/B/C/D/mixed), omitted when empty |
| `files[].status` | string | The note's `status` field, omitted when empty |
| `files[].review_by` | string | The note's `review_by` date, omitted when empty |
| `files[].stale` | bool | True when `review_by` has passed |
| `lines` | array | Matching lines; set when `output_mode=content` |
| `lines[].vault` | string | Name of the vault holding the note |
| `lines[].path` | string | Vault-relative path |
| `lines[].line` | int | 1-based line number |
| `lines[].column` | int | 1-based column |
| `lines[].text` | string | Matching line text |
| `lines[].before` | string | Context before, if `context_lines>0` |
| `lines[].after` | string | Context after, if `context_lines>0` |
| `lines[].evidence` | string | Trust tier of the matching note, omitted when empty |
| `lines[].status`, `lines[].review_by`, `lines[].stale` | | As on `files[]` |
| `total` | int | Number of records returned |

**Example:**

```json
// request
{"query": "kubernetes", "tier": "notes", "output_mode": "files_with_matches"}

// response
{
  "output_mode": "files_with_matches",
  "files": [
    {"vault": "personal", "path": "notes/ops/k8s-upgrades.md"},
    {"vault": "team", "path": "notes/ops/k8s-networking.md"}
  ],
  "total": 2
}
```

**Notes:**
- Each vault's index auto-refreshes before the search if that vault's fingerprint has changed since the last call. The first call after a large edit may take a second or two; subsequent calls are fast.
- Hits from several vaults are merged by zoekt score, best first, and `limit` caps the notes across all of them.
- Frontmatter-field filters (`tag`, `type`, `status`) are applied by the search layer against frontmatter parsed into the index, not by walking files at query time.

### `brain_count`

Count matches of a zoekt query across the vault, optionally grouped.

**Input:**

| Field | Type | Required | Description |
|---|---|---|---|
| `query` | string | yes | Zoekt query (same syntax as `brain_search`) |
| `tier` | string | no | Restrict to a single tier |
| `group_by` | string | no | `tier`, `tag` or `vault`; empty for a single total |
| `vault` | string | no | Restrict to this configured vault; default counts every vault |

**Output:**

| Field | Type | Description |
|---|---|---|
| `total` | int | Total match count |
| `groups` | array | Per-group counts; empty when `group_by` is unset |
| `groups[].group` | string | Tier name, tag or vault name |
| `groups[].count` | int | Matches in that group |

**Example:**

```json
// request
{"query": "TODO", "group_by": "tier"}

// response
{
  "total": 42,
  "groups": [
    {"group": "inbox", "count": 28},
    {"group": "notes", "count": 14}
  ]
}
```

**Notes:** `group_by` must be exactly `""`, `tier`, `tag`, or `vault`. Anything else errors.

### `brain_query_validate`

Validate a zoekt query and return its parsed tree or a parse error with a fixing hint. Use before a complex `brain_search` call to debug escaped regex metacharacters like `\.md$`.

**Input:**

| Field | Type | Required | Description |
|---|---|---|---|
| `query` | string | yes | The zoekt query string to validate |

**Output:**

| Field | Type | Description |
|---|---|---|
| `valid` | bool | Whether the query parsed successfully |
| `parsed` | string | String form of the parsed query tree (when valid) |
| `error` | string | Parse error message (when invalid) |
| `hint` | string | Suggestion for fixing the query (when invalid) |

**Example:**

```json
// request
{"query": "foo file:\\.md$"}

// response
{"valid": true, "parsed": "(and substr:\"foo\" file_regexp:\"\\.md$\")"}
```

### `brain_read`

Read a note's frontmatter and a slice of its body.

**Input:**

| Field | Type | Required | Description |
|---|---|---|---|
| `path` | string | yes | Note ref: `vault:path`, `id:<ulid>`, or a vault-relative path found in exactly one vault |
| `vault` | string | no | Look the path up in this configured vault only |
| `start_line` | int | no | First body line, 1-based; `0`/omit for start |
| `end_line` | int | no | Last body line, 1-based inclusive; `0`/omit for end |

**Output:**

| Field | Type | Description |
|---|---|---|
| `vault` | string | Name of the vault holding the note |
| `path` | string | Vault-relative path |
| `sha` | string | sha256 of the note's bytes; pass it to `brain_edit` as `if_match` |
| `banner` | string | One line for a superseded, retracted, withdrawn or disputed note, such as `RETRACTED on 2026-11-05 (error): ... Do not cite this note.`; omitted otherwise |
| `frontmatter` | object | `title`, `aliases`, `tags`, `type`, `status`, `source`, `evidence`, `created`, `updated`, `id`, plus `extra` with every frontmatter key brain does not model (each omitted when empty) |
| `body_lines` | array | `{line: int, text: string}` entries |

**Example:**

```json
// request
{"path": "personal:notes/ops/k8s-upgrades.md", "start_line": 1, "end_line": 5}

// response
{
  "vault": "personal",
  "path": "notes/ops/k8s-upgrades.md",
  "sha": "3b6f0e1a9c4d2b7e8f5a1c0d9e2b4a6f7c8d1e3f5a7b9c0d2e4f6a8b0c1d3e5f",
  "frontmatter": {
    "title": "Kubernetes upgrades",
    "tags": ["kubernetes", "ops"],
    "type": "note",
    "status": "validated"
  },
  "body_lines": [
    {"line": 1, "text": "## Context"},
    {"line": 2, "text": ""},
    {"line": 3, "text": "Notes from the last upgrade cycle."}
  ]
}
```

**Notes:**
- Line numbers in `body_lines` are body-relative (frontmatter excluded), not file-relative.
- Paths are rejected if they are absolute or contain `..` segments that escape the vault root.

### `brain_peek`

Cheap overview of a note: frontmatter, first N body lines, and inbound-backlink count.

**Input:**

| Field | Type | Required | Description |
|---|---|---|---|
| `path` | string | yes | Note ref: `vault:path`, `id:<ulid>`, or a vault-relative path found in exactly one vault |
| `vault` | string | no | Look the path up in this configured vault only |
| `max_body_lines` | int | no | Preview length (default `20`) |

**Output:**

| Field | Type | Description |
|---|---|---|
| `vault` | string | Name of the vault holding the note |
| `path` | string | Vault-relative path |
| `banner` | string | Same as `brain_read` |
| `frontmatter` | object | Same shape as `brain_read` |
| `preview` | array[string] | First `max_body_lines` of the body |
| `backlink_count` | int | Number of other notes with a link that resolves to this note |

**Example:**

```json
// request
{"path": "notes/ops/k8s-upgrades.md"}

// response
{
  "vault": "personal",
  "path": "notes/ops/k8s-upgrades.md",
  "frontmatter": {"title": "Kubernetes upgrades", "type": "note", "status": "validated"},
  "preview": ["## Context", "", "Notes from the last upgrade cycle."],
  "backlink_count": 3
}
```

**Notes:** `backlink_count` uses the same link resolver as `brain_backlinks` (stem, path, title and alias, across the configured vaults) and counts each linking note once. Use `brain_backlinks` for the actual list of linking lines. `.git`, `.obsidian`, `.trash`, `.brain`, and `attachments` are skipped.

### `brain_list_tier`

List notes in one tier across every vault, optionally filtered by frontmatter, sorted by `updated` desc, then by vault in config order, then `path` asc.

**Input:**

| Field | Type | Required | Description |
|---|---|---|---|
| `tier` | string | yes | `inbox`, `notes`, `outputs`, or `MOCs` |
| `tag` | string | no | Filter by frontmatter tag |
| `status` | string | no | Filter by frontmatter status |
| `type` | string | no | Filter by frontmatter type |
| `limit` | int | no | Max results (default `100`) |
| `vault` | string | no | List this configured vault only; default lists every vault |

**Output:**

| Field | Type | Description |
|---|---|---|
| `tier` | string | Echoed tier directory |
| `total` | int | Total matching notes before `limit` truncation |
| `notes` | array | Listed notes, truncated to `limit` |
| `notes[].vault` | string | Name of the vault holding the note |
| `notes[].path` | string | Vault-relative path |
| `notes[].title` | string | frontmatter.title |
| `notes[].status` | string | frontmatter.status |
| `notes[].type` | string | frontmatter.type |
| `notes[].tags` | array[string] | frontmatter.tags |
| `notes[].aliases` | array[string] | frontmatter.aliases |
| `notes[].evidence` | string | trust tier (A/B/C/D/mixed), omitted when empty |
| `notes[].updated` | string | frontmatter.updated |

**Example:**

```json
// request
{"tier": "inbox", "limit": 3}

// response
{
  "tier": "inbox",
  "total": 17,
  "notes": [
    {"vault": "personal", "path": "inbox/2025-01-02-article.md", "title": "Article capture", "status": "raw", "type": "source", "updated": "2025-01-02T10:00:00Z"}
  ]
}
```

**Notes:** Frontmatter parsing errors on individual files are silently skipped rather than failing the whole list. Missing tier directories return an empty result, not an error.

### `brain_backlinks`

Return inbound and outbound wikilinks for a note. One resolver decides every link's target, the same one `brain_peek`, `brain_promote` and `brain_graph` use:

- A target matches case-insensitively: by path when it holds a slash (`[[ops/k8s-upgrades]]` matches `notes/ops/k8s-upgrades.md`), otherwise by stem, then by title or alias. It is never matched by prefix, so `[[team]]` does not reach `team-guide`.
- A link in a private vault looks in that vault first, then in the shared vaults in config order. A link in a shared vault looks only in shared vaults.
- Within a vault, a name two notes share goes to the one in `notes`, then `MOCs`, `outputs`, `inbox`.
- Embeds (`![[stem]]`) count as links. Links inside code spans and fenced code blocks do not.
- A target that resolves nowhere is a dangling link.

**Input:**

| Field | Type | Required | Description |
|---|---|---|---|
| `path` | string | one of path/title | Note ref: `vault:path`, `id:<ulid>`, or a vault-relative path found in exactly one vault (preferred) |
| `title` | string | one of path/title | Note title or alias; matched case-insensitively against frontmatter.title and aliases. A title found in more than one vault fails as ambiguous |
| `vault` | string | no | Look the note up in this configured vault only |

**Output:**

| Field | Type | Description |
|---|---|---|
| `vault` | string | Vault of the analyzed note |
| `note_path` | string | Vault-relative path of the analyzed note |
| `evidence` | string | Trust tier of the analyzed note (A/B/C/D/mixed), omitted when empty |
| `inbound` | array | Lines in other notes that link here, each line once |
| `inbound[].vault` | string | Vault of the linking note |
| `inbound[].path` | string | Linking note path |
| `inbound[].line` | int | 1-based line number, counted from the top of the file |
| `inbound[].excerpt` | string | Matching line, trimmed to ~200 chars |
| `inbound[].evidence` | string | Trust tier of the linking note, omitted when empty |
| `outbound` | array | Wikilinks in this note |
| `outbound[].target` | string | Wikilink target as written |
| `outbound[].vault` | string | Vault of the resolved target; omitted when dangling |
| `outbound[].path` | string | Vault-relative path of the resolved target; omitted when dangling |
| `outbound[].line` | int | 1-based line number in this note, counted from the top of the file |
| `outbound[].evidence` | string | Trust tier of the resolved target note, omitted when empty |
| `outbound[].dangling` | bool | `true` when no note matches the target; omitted otherwise |

**Example:**

```json
// request
{"path": "personal:notes/ops/k8s-upgrades.md"}

// response
{
  "vault": "personal",
  "note_path": "notes/ops/k8s-upgrades.md",
  "inbound": [
    {"vault": "personal", "path": "MOCs/ops.md", "line": 18, "excerpt": "- [[k8s-upgrades]]: cluster upgrade playbook"}
  ],
  "outbound": [
    {"target": "k8s-networking", "vault": "team", "path": "notes/ops/k8s-networking.md", "line": 24},
    {"target": "etcd-backups", "line": 31, "dangling": true}
  ]
}
```

**Notes:**
- Every call reads every note in the configured vaults (skipping `.git`, `.obsidian`, `.trash`, `attachments`) to resolve links, so the `vault` input narrows the lookup of the analyzed note, not the links counted.
- Line numbers include the frontmatter lines, unlike `brain_read`'s body-relative `body_lines`.

### `brain_graph`

Return the wikilink graph of the configured vaults as nodes and edges. Links resolve with the same resolver as `brain_backlinks`. Links of a note to itself are left out.

**Input:**

| Field | Type | Required | Description |
|---|---|---|---|
| `vault` | string | no | Walk only this vault's notes; notes they link to in other vaults still appear, without their own links |
| `center` | string | no | Note ref (`vault:path`, `id:<ulid>` or a bare path); keep only the nodes within `depth` hops of it. With `vault` set, it must be in that vault |
| `depth` | int | no | Hops from `center`, following links in either direction (default `1`) |
| `tags_as_nodes` | bool | no | Add a node per tag and a `tag` edge from each note to it |

**Output:**

| Field | Type | Description |
|---|---|---|
| `nodes` | array | Every node, sorted by `id` |
| `nodes[].id` | string | `id:<note id>` for a note; `vault:path` for a note without an id, or sharing one; `dangling:<target>` (lowercased, one node per target); `tag:<name>` |
| `nodes[].vault` | string | Vault of the note; omitted for dangling and tag nodes |
| `nodes[].path` | string | Vault-relative path of the note; omitted for dangling and tag nodes |
| `nodes[].stem` | string | Filename without `.md`; for a dangling node, the target as written |
| `nodes[].title` | string | frontmatter.title; for a tag node, the tag |
| `nodes[].tier` | string | Tier directory of the note, omitted when outside the tiers |
| `nodes[].type` | string | frontmatter.type |
| `nodes[].status` | string | frontmatter.status |
| `nodes[].tags` | array[string] | frontmatter.tags |
| `nodes[].evidence` | string | Trust tier (A/B/C/D/mixed), omitted when empty |
| `nodes[].review_by` | string | The `review_by` frontmatter value, omitted when unset |
| `nodes[].degree` | int | Edges of the returned graph that touch the node, either direction |
| `nodes[].dangling` | bool | `true` for a link target no note matches |
| `edges` | array | Every edge, sorted by `source`, then `target`, then `kind` |
| `edges[].source` | string | Node id of the linking note |
| `edges[].target` | string | Node id of the linked note, dangling target or tag |
| `edges[].kind` | string | `link`, or `tag` with `tags_as_nodes` |
| `edges[].resolved` | bool | `false` for an edge to a dangling node |

**Example:**

```json
// request
{"center": "personal:notes/ops/k8s-upgrades.md", "depth": 1}

// response
{
  "nodes": [
    {"id": "dangling:etcd-backups", "stem": "etcd-backups", "degree": 1, "dangling": true},
    {"id": "id:01JABCDEFGHJKMNPQRSTVWXYZ0", "vault": "personal", "path": "notes/ops/k8s-upgrades.md", "stem": "k8s-upgrades", "title": "Kubernetes upgrades", "tier": "notes", "degree": 2, "dangling": false},
    {"id": "team:notes/ops/k8s-networking.md", "vault": "team", "path": "notes/ops/k8s-networking.md", "stem": "k8s-networking", "tier": "notes", "degree": 1, "dangling": false}
  ],
  "edges": [
    {"source": "id:01JABCDEFGHJKMNPQRSTVWXYZ0", "target": "dangling:etcd-backups", "kind": "link", "resolved": false},
    {"source": "id:01JABCDEFGHJKMNPQRSTVWXYZ0", "target": "team:notes/ops/k8s-networking.md", "kind": "link", "resolved": true}
  ]
}
```

**Notes:**
- Several links from one note to the same target make one edge.
- `degree` counts only the edges returned, so with `center` set it ignores edges to nodes outside `depth`.
- Every call reads every note in the configured vaults, like `brain_backlinks`.

### `brain_write`

Create a new note in a tier with frontmatter.

**Input:**

| Field | Type | Required | Description |
|---|---|---|---|
| `tier` | string | yes | `inbox`, `notes`, `outputs`, or `MOCs` |
| `title` | string | yes | Becomes `frontmatter.title` and the filename slug |
| `body` | string | no | Markdown body (can be empty for a stub) |
| `tags` | array[string] | no | frontmatter.tags |
| `aliases` | array[string] | no | frontmatter.aliases |
| `type` | string | no | frontmatter.type (defaults per tier) |
| `status` | string | no | frontmatter.status (defaults per tier) |
| `source` | string | no | frontmatter.source URL |
| `evidence` | string | no | Trust tier (A/B/C/D/mixed) for empirical notes |
| `filename` | string | no | Override filename (without `.md`); default is `slug(title)` |
| `vault` | string | no | Configured vault to write to; defaults to the default vault. A shared vault refuses the write |

**Output:**

| Field | Type | Description |
|---|---|---|
| `vault` | string | Name of the vault holding the note |
| `path` | string | Vault-relative path of the created note |
| `warnings` | array[string] | One per body link to a note that is not validated or is expired, such as `line 7 links [[draft]] (personal:inbox/draft.md), which is raw`; omitted when none |

**Example:**

```json
// request
{"tier": "inbox", "title": "Article on raft consensus", "tags": ["distributed"], "source": "https://example.com/raft"}

// response
{"vault": "personal", "path": "inbox/article-on-raft-consensus.md"}
```

**Notes:**
- The note gets a fresh note id in `id`.
- Errors if a note with the same filename already exists in the target tier.
- Fails with a read-only error when the target is a shared vault.
- Refuses a note that holds a secret and writes nothing (see Note lifecycle).
- Outputs are filename-prefixed `YYYY-MM-DD-<slug>.md` automatically.
- Tier defaults: `inbox` → `type=note,status=raw`; `notes` → `type=note,status=validated`; MOCs → `type=moc,status=validated`; outputs → `type=output,status=validated`.

### `brain_edit`

Modify an existing note: replace the body, append to the body, and/or merge a frontmatter patch. Updates `frontmatter.updated`.

**Input:**

| Field | Type | Required | Description |
|---|---|---|---|
| `path` | string | yes | Note ref: `vault:path`, `id:<ulid>`, or a vault-relative path found in exactly one vault |
| `vault` | string | no | Look the path up in this configured vault only |
| `body` | string | no | Replace body entirely (mutually exclusive with `append`) |
| `append` | string | no | Append to existing body (mutually exclusive with `body`) |
| `frontmatter_patch` | object | no | Partial frontmatter to merge (non-empty fields win) |
| `frontmatter_patch.title` | string | no | |
| `frontmatter_patch.aliases` | array[string] | no | |
| `frontmatter_patch.tags` | array[string] | no | |
| `frontmatter_patch.type` | string | no | |
| `frontmatter_patch.status` | string | no | |
| `frontmatter_patch.source` | string | no | |
| `frontmatter_patch.evidence` | string | no | Trust tier (A/B/C/D/mixed); pass empty string to leave unchanged |
| `frontmatter_patch.id` | string | no | Sets an id on a note that has none; changing or removing an existing id fails |
| `if_match` | string | no | The `sha` from `brain_read`; the edit is refused when the note changed since that read |

**Output:**

| Field | Type | Description |
|---|---|---|
| `vault` | string | Name of the vault holding the note |
| `path` | string | Vault-relative path of the edited note |
| `sha` | string | sha256 of the bytes written; the `if_match` for a next edit |
| `warnings` | array[string] | As on `brain_write`, when `body` or `append` is set |

**Example:**

```json
// request
{"path": "inbox/article-on-raft-consensus.md", "append": "\n## Followup\nRead the original paper.", "frontmatter_patch": {"status": "validated"}, "if_match": "3b6f0e1a9c4d2b7e8f5a1c0d9e2b4a6f7c8d1e3f5a7b9c0d2e4f6a8b0c1d3e5f"}

// response
{"vault": "personal", "path": "inbox/article-on-raft-consensus.md", "sha": "a1c3e5f7092b4d6f8a0c2e4f6b8d0a1c3e5f7092b4d6f8a0c2e4f6b8d0a1c3e5"}
```

**Notes:**
- Errors if `body` and `append` are both set, or if none of `body`/`append`/`frontmatter_patch` is set.
- With `if_match` set, a note whose sha no longer matches is left as it is and the error says to read it again before editing.
- Fails with a read-only error on a note in a shared vault.
- Refuses an edit whose result holds a secret, leaving the note as it was.

### `brain_promote`

Promote an inbox note into another tier (default `notes`): atomic file move, status flips to `validated`, every wikilink that resolved to the note is pointed at its new stem (or its new path, for a path link), and optionally a bullet is appended to a MOC.

**Input:**

| Field | Type | Required | Description |
|---|---|---|---|
| `path` | string | yes | Note ref of an inbox note: `vault:path`, `id:<ulid>`, or a vault-relative path found in exactly one vault |
| `new_title` | string | no | Optional new title (and re-slugged filename) |
| `target_tier` | string | no | Destination tier; defaults to `notes` |
| `moc` | string | no | MOC path (vault-relative or absolute) to append a bullet link to |

**Output:**

| Field | Type | Description |
|---|---|---|
| `vault` | string | Name of the vault holding the note |
| `old_path` | string | Vault-relative path before the move |
| `new_path` | string | Vault-relative path after the move |
| `backlink_rewrites` | array[string] | Vault-relative paths of notes whose wikilinks were updated |
| `moc_appended` | string | Vault-relative path of the MOC that had a bullet appended; omitted when none |

**Example:**

```json
// request
{"path": "inbox/raft-consensus.md", "new_title": "Raft consensus", "moc": "MOCs/distributed.md"}

// response
{
  "vault": "personal",
  "old_path": "inbox/raft-consensus.md",
  "new_path": "notes/raft-consensus.md",
  "backlink_rewrites": ["MOCs/distributed.md", "notes/etcd-internals.md"],
  "moc_appended": "MOCs/distributed.md"
}
```

**Notes:**
- The rewrite uses the `brain_backlinks` resolver and scans every note in the note's vault, not just the target tier. A stem link takes the new stem and a path link the new path. When `new_title` changes the title, a link by the old title takes the new stem and keeps the old title as display text. A link by an alias keeps resolving through the alias.
- Links inside code spans and fenced code blocks are left alone.
- A heading, block or display text on a rewritten link is kept.
- The note keeps its id, or gets one when it has none.
- There is no `vault` input. Use a `vault:path` ref when a bare path is in more than one vault. A note in a shared vault fails with a read-only error.
- The MOC bullet is appended only if `moc` is set and the MOC file exists.

### `brain_doctor`

Run the checks `brain doctor` runs and return the same report. Read-only: it creates no directory and leaves no file behind. Call it when another tool errors or comes back empty, since a client without a shell cannot run `brain doctor`.

**Input:** none.

**Output:** `ok` (false when any check failed) and `checks[] {name, status, detail, fix}`. `name` is `config`, `vault`, `index-dir`, `index`, `ids` (two notes holding one note id) or `ollama`; `status` is `ok`, `fail` or `skip`, where `skip` means an earlier failure left the check nothing to examine; `fix` names the next step for a failure. A failing check is a result, never a tool error.

## Zoekt query syntax

`brain_search` and `brain_count` use [zoekt](https://github.com/sourcegraph/zoekt) query syntax:

| Syntax | Meaning |
|---|---|
| `foo` | Literal substring |
| `"foo bar"` | Quoted phrase |
| `foo bar` | AND (space-separated) |
| `foo\|bar` | OR |
| `-foo` | NOT |
| `file:\.md$` | File path regex (vault-relative) |
| `lang:markdown` | Language filter |
| `case:yes` | Case-sensitive match |

`repo:` is **not applicable**: each vault has its own index, and a query runs against every one of them. Use the dedicated `vault` input to scope to one vault, the `tier` input to scope to a tier (it expands to `file:^<tier>/`), or pass `file:` directly for finer control over paths.

Run `brain_query_validate` to parse-check complex queries before invoking search.

## Indexing

Indexes are per vault. Each vault's zoekt index lives at `index/<vault>/` in the state directory (resolved via `brain.IndexDir(<vault>)`), and its semantic index at `semantic/<vault>/`. Before a literal query (`brain_search`, `brain_count`, and the literal side of `brain_hybrid_search`), each vault the query covers is checked:

1. Compute a fingerprint of the vault (the set, sizes and mtimes of its markdown files).
2. Compare to the stored state in that vault's index directory.
3. If stale (or no index exists), re-index that vault silently. Progress is not written to stderr, which would corrupt the MCP JSON-RPC framing.
4. Persist the new fingerprint and file list.

A markdown change in one vault makes only that vault's index stale on the next search. A build holds that vault's index lock, so the CLI and the MCP server never build the same index at once.

The semantic index syncs incrementally. Its `manifest.json` records the embed model, the chunk sizes and a content hash per note. A sync re-embeds only the notes whose hash changed and purges the chunks of deleted notes; a store made with another model or chunk size is rebuilt. `brain_semantic_search` and `brain_hybrid_search` sync a stale semantic index before they query. When Ollama does not answer, the stale index stays as it was and the result carries `warnings[]` saying so. `brain_hybrid_search` then fuses the literal hits alone and adds a warning. `brain_semantic_search` still needs Ollama to embed the query, so with Ollama down it returns an error.

To rebuild manually (e.g. after a config change):

```bash
brain index            # refresh if stale
brain index --force    # unconditional rebuild
```

## Smoke testing

To verify `brain mcp` speaks JSON-RPC correctly without involving Claude Code:

```bash
( printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}\n{"jsonrpc":"2.0","method":"notifications/initialized"}\n{"jsonrpc":"2.0","id":2,"method":"tools/list"}\n'; sleep 1 ) | brain mcp
```

Expected: one `initialize` response followed by a `tools/list` response listing all fourteen `brain_*` tools with their input and output schemas. The `sleep 1` keeps stdin open long enough for the server to flush responses before exit.
