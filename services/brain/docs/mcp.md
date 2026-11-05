# `brain mcp` — MCP stdio server reference

`brain mcp` is a [Model Context Protocol](https://modelcontextprotocol.io) stdio server bundled into the `brain` binary. It exposes the vault (search, read, list, backlinks, write, edit, promote) as native MCP tools so Claude Code can operate on your notes without shelling out through `Bash()`.

There is no long-running server. The MCP client spawns `brain mcp` as a subprocess per session, pipes JSON-RPC 2.0 over stdin/stdout, and the process exits when stdin closes. Each tool handler is a thin adapter over the internal `search`, `vault`, and `markdown` packages that back the cobra commands.

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

The vault location comes from `vault:` in the config file `brain mcp` resolved at startup (`--config`, then `BRAIN_CONFIG`, then `config.yaml` under `$XDG_CONFIG_HOME/brain` or `~/.config/brain`). The indexes live in the state directory, `$XDG_STATE_HOME/brain` or `~/.local/state/brain`; an install that already built an index under `~/.config/brain` keeps it there. `config.md` beside the service has the full rules. The vault is your own vault repo, checked out wherever `vault:` points. Keeping it outside thismoon means the tool's history stays clean of personal-knowledge churn.

## Vault model

The vault is a single directory with four top-level tiers:

| Tier | Directory | Default `type` | Default `status` | Purpose |
|---|---|---|---|---|
| inbox | `inbox/` | `source` | `raw` | Raw captures, unprocessed |
| notes | `notes/` | `note` | `validated` | First-class knowledge |
| outputs | `outputs/` | `output` | — | Finished artifacts (filenames are date-prefixed `YYYY-MM-DD-…`) |
| MOCs | `MOCs/` | `moc` | — | Maps of Content (curated indexes) |

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
id: 01ABCDEF
---
body text with [[wikilinks]]
```

### Evidence (trust tier)

Read-side tools (`brain_search`, `brain_read`, `brain_peek`, `brain_list_tier`, `brain_backlinks`) all surface an `evidence` field so callers can reason about a note's correctness level without string-matching tags.

| Value | Meaning |
|---|---|
| `A` | Replicated RCT / meta-analysis with named effect size + CI |
| `B` | Single well-designed peer-reviewed study, reasonable N |
| `C` | Preprint, survey, mechanism-plausible, category-evidence extended |
| `D` | Refuted / apocryphal / null under blinding — kept as do-not-cite record |
| `mixed` | Composite note; inline per-claim grades in body |
| (empty) | Non-empirical note (profile, MOC, tool reference) |

The field is read with a two-step precedence: the explicit `evidence:` frontmatter field wins; otherwise an `evidence/<tier>` tag is parsed as fallback. New notes should prefer the explicit field; the tag form remains supported for legacy notes.

A `tier` input on search / count / list tools translates into a `file:^<tier>/` regex prefix against the zoekt index (e.g. `tier: "notes"` becomes `file:^notes/`). `ParseTier` accepts `inbox`, `notes`, `outputs`, and `moc`/`mocs`/`MOCs` (case-insensitive).

## Tools

Registered tools: `brain_search`, `brain_semantic_search`, `brain_hybrid_search`, `brain_count`, `brain_query_validate`, `brain_read`, `brain_peek`, `brain_list_tier`, `brain_backlinks`, `brain_write`, `brain_edit`, `brain_promote`, `brain_doctor`.

### `brain_search`

Search notes by text with zoekt query syntax, optionally filtered by tier / tag / type / status.

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

**Output:**

| Field | Type | Description |
|---|---|---|
| `output_mode` | string | Echoes the mode actually used |
| `files` | array | Unique file paths; set when `output_mode=files_with_matches` |
| `files[].path` | string | Vault-relative path |
| `files[].evidence` | string | Trust tier of the note (A/B/C/D/mixed), omitted when empty |
| `lines` | array | Matching lines; set when `output_mode=content` |
| `lines[].path` | string | Vault-relative path |
| `lines[].line` | int | 1-based line number |
| `lines[].column` | int | 1-based column |
| `lines[].text` | string | Matching line text |
| `lines[].before` | string | Context before, if `context_lines>0` |
| `lines[].after` | string | Context after, if `context_lines>0` |
| `lines[].evidence` | string | Trust tier of the matching note, omitted when empty |
| `total` | int | Number of records returned |

**Example:**

```json
// request
{"query": "kubernetes", "tier": "notes", "output_mode": "files_with_matches"}

// response
{
  "output_mode": "files_with_matches",
  "files": [
    {"path": "notes/ops/k8s-upgrades.md"},
    {"path": "notes/ops/k8s-networking.md"}
  ],
  "total": 2
}
```

**Notes:**
- The index auto-refreshes before the search if the vault fingerprint has changed since the last call. The first call after a large edit may take a second or two; subsequent calls are fast.
- Frontmatter-field filters (`tag`, `type`, `status`) are applied by the search layer against frontmatter parsed into the index, not by walking files at query time.

### `brain_count`

Count matches of a zoekt query across the vault, optionally grouped.

**Input:**

| Field | Type | Required | Description |
|---|---|---|---|
| `query` | string | yes | Zoekt query (same syntax as `brain_search`) |
| `tier` | string | no | Restrict to a single tier |
| `group_by` | string | no | `tier` or `tag`; empty for a single total |

**Output:**

| Field | Type | Description |
|---|---|---|
| `total` | int | Total match count |
| `groups` | array | Per-group counts; empty when `group_by` is unset |
| `groups[].group` | string | Tier name or tag |
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

**Notes:** `group_by` must be exactly `""`, `tier`, or `tag` — anything else errors.

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
| `path` | string | yes | Vault-relative note path |
| `start_line` | int | no | First body line, 1-based; `0`/omit for start |
| `end_line` | int | no | Last body line, 1-based inclusive; `0`/omit for end |

**Output:**

| Field | Type | Description |
|---|---|---|
| `path` | string | Vault-relative path |
| `frontmatter` | object | `title`, `aliases`, `tags`, `type`, `status`, `source`, `evidence`, `created`, `updated`, `id` (omitted when empty) |
| `body_lines` | array | `{line: int, text: string}` entries |

**Example:**

```json
// request
{"path": "notes/ops/k8s-upgrades.md", "start_line": 1, "end_line": 5}

// response
{
  "path": "notes/ops/k8s-upgrades.md",
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
| `path` | string | yes | Vault-relative note path |
| `max_body_lines` | int | no | Preview length (default `20`) |

**Output:**

| Field | Type | Description |
|---|---|---|
| `path` | string | Vault-relative path |
| `frontmatter` | object | Same shape as `brain_read` |
| `preview` | array[string] | First `max_body_lines` of the body |
| `backlink_count` | int | Count of other notes containing `[[<stem>` for this note |

**Example:**

```json
// request
{"path": "notes/ops/k8s-upgrades.md"}

// response
{
  "path": "notes/ops/k8s-upgrades.md",
  "frontmatter": {"title": "Kubernetes upgrades", "type": "note", "status": "validated"},
  "preview": ["## Context", "", "Notes from the last upgrade cycle."],
  "backlink_count": 3
}
```

**Notes:** `backlink_count` is a filesystem walk that only matches on the file stem (`[[stem`), not on title or aliases. Use `brain_backlinks` for alias-aware resolution and the actual list of linking lines. `.git`, `.obsidian`, `.trash`, and `attachments` are skipped.

### `brain_list_tier`

List notes in one tier, optionally filtered by frontmatter, sorted by `updated` desc then `path` asc.

**Input:**

| Field | Type | Required | Description |
|---|---|---|---|
| `tier` | string | yes | `inbox`, `notes`, `outputs`, or `MOCs` |
| `tag` | string | no | Filter by frontmatter tag |
| `status` | string | no | Filter by frontmatter status |
| `type` | string | no | Filter by frontmatter type |
| `limit` | int | no | Max results (default `100`) |

**Output:**

| Field | Type | Description |
|---|---|---|
| `tier` | string | Echoed tier directory |
| `total` | int | Total matching notes before `limit` truncation |
| `notes` | array | Listed notes, truncated to `limit` |
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
    {"path": "inbox/2025-01-02-article.md", "title": "Article capture", "status": "raw", "type": "source", "updated": "2025-01-02T10:00:00Z"}
  ]
}
```

**Notes:** Frontmatter parsing errors on individual files are silently skipped rather than failing the whole list. Missing tier directories return an empty result, not an error.

### `brain_backlinks`

Return inbound and outbound wikilinks for a note. Inbound matches any `[[<stem>`, `[[<title>`, or `[[<alias>` prefix; outbound resolves every `[[...]]` in the body against a vault-wide lowercase stem → path index.

**Input:**

| Field | Type | Required | Description |
|---|---|---|---|
| `path` | string | one of path/title | Vault-relative note path (preferred) |
| `title` | string | one of path/title | Note title or alias; matched case-insensitively against frontmatter.title and aliases |

**Output:**

| Field | Type | Description |
|---|---|---|
| `note_path` | string | Vault-relative path of the analyzed note |
| `evidence` | string | Trust tier of the analyzed note (A/B/C/D/mixed), omitted when empty |
| `inbound` | array | Lines in other notes that link here |
| `inbound[].path` | string | Linking note path |
| `inbound[].line` | int | 1-based line number |
| `inbound[].excerpt` | string | Matching line, trimmed to ~200 chars |
| `inbound[].evidence` | string | Trust tier of the linking note, omitted when empty |
| `outbound` | array | Wikilinks in this note |
| `outbound[].target` | string | Wikilink target as written |
| `outbound[].path` | string | Resolved vault-relative path if the target stem exists |
| `outbound[].line` | int | Line number in this note |
| `outbound[].evidence` | string | Trust tier of the resolved target note, omitted when empty |

**Example:**

```json
// request
{"path": "notes/ops/k8s-upgrades.md"}

// response
{
  "note_path": "notes/ops/k8s-upgrades.md",
  "inbound": [
    {"path": "MOCs/ops.md", "line": 12, "excerpt": "- [[k8s-upgrades]] — cluster upgrade playbook"}
  ],
  "outbound": [
    {"target": "k8s-networking", "path": "notes/ops/k8s-networking.md", "line": 18}
  ]
}
```

**Notes:**
- Inbound search scans every `.md` under the vault (skipping `.git`, `.obsidian`, `.trash`, `attachments`) and reads each matching file a second time to extract line numbers/excerpts.
- Outbound `path` is empty when the target stem doesn't correspond to any note — useful for finding dangling links.

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

**Output:**

| Field | Type | Description |
|---|---|---|
| `path` | string | Vault-relative path of the created note |

**Example:**

```json
// request
{"tier": "inbox", "title": "Article on raft consensus", "tags": ["distributed"], "source": "https://example.com/raft"}

// response
{"path": "inbox/article-on-raft-consensus.md"}
```

**Notes:**
- Errors if a note with the same filename already exists in the target tier.
- Outputs are filename-prefixed `YYYY-MM-DD-<slug>.md` automatically.
- Tier defaults: `inbox` → `type=source,status=raw`; `notes` → `type=note,status=validated`; MOCs → `type=moc`; outputs → `type=output`.

### `brain_edit`

Modify an existing note: replace the body, append to the body, and/or merge a frontmatter patch. Updates `frontmatter.updated`.

**Input:**

| Field | Type | Required | Description |
|---|---|---|---|
| `path` | string | yes | Vault-relative note path |
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
| `frontmatter_patch.id` | string | no | |

**Output:**

| Field | Type | Description |
|---|---|---|
| `path` | string | Vault-relative path of the edited note |

**Example:**

```json
// request
{"path": "inbox/article-on-raft-consensus.md", "append": "\n## Followup\nRead the original paper.", "frontmatter_patch": {"status": "validated"}}

// response
{"path": "inbox/article-on-raft-consensus.md"}
```

**Notes:** Errors if `body` and `append` are both set, or if none of `body`/`append`/`frontmatter_patch` is set.

### `brain_promote`

Promote an inbox note into another tier (default `notes`): atomic file move, status flips to `validated`, every `[[<oldstem>]]` reference across the vault is rewritten to the new filename, and optionally a bullet is appended to a MOC.

**Input:**

| Field | Type | Required | Description |
|---|---|---|---|
| `path` | string | yes | Vault-relative inbox note path |
| `new_title` | string | no | Optional new title (and re-slugged filename) |
| `target_tier` | string | no | Destination tier; defaults to `notes` |
| `moc` | string | no | MOC path (vault-relative or absolute) to append a bullet link to |

**Output:**

| Field | Type | Description |
|---|---|---|
| `old_path` | string | Vault-relative path before the move |
| `new_path` | string | Vault-relative path after the move |
| `backlink_rewrites` | array[string] | Vault-relative paths of notes whose wikilinks were updated |
| `moc_appended` | string | Vault-relative path of the MOC that had a bullet appended (empty if none) |

**Example:**

```json
// request
{"path": "inbox/raft-consensus.md", "new_title": "Raft consensus", "moc": "MOCs/distributed.md"}

// response
{
  "old_path": "inbox/raft-consensus.md",
  "new_path": "notes/raft-consensus.md",
  "backlink_rewrites": ["MOCs/distributed.md", "notes/etcd-internals.md"],
  "moc_appended": "MOCs/distributed.md"
}
```

**Notes:**
- Backlink rewriting matches by file stem (`[[<oldstem>]]` → `[[<newstem>]]`) and scans every `.md` under the vault, not just the target tier.
- The MOC bullet is appended only if `moc` is set and the MOC file exists.

### `brain_doctor`

Run the checks `brain doctor` runs and return the same report. Read-only: it creates no directory and leaves no file behind. Call it when another tool errors or comes back empty, since a client without a shell cannot run `brain doctor`.

**Input:** none.

**Output:** `ok` (false when any check failed) and `checks[] {name, status, detail, fix}`. `name` is `config`, `vault`, `index-dir`, `index` or `ollama`; `status` is `ok`, `fail` or `skip`, where `skip` means an earlier failure left the check nothing to examine; `fix` names the next step for a failure. A failing check is a result, never a tool error.

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

`repo:` is **not applicable** — the brain index is a single vault. Use the dedicated `tier` input to scope to a tier (it expands to `file:^<tier>/`), or pass `file:` directly for finer control over paths.

Run `brain_query_validate` to parse-check complex queries before invoking search.

## Indexing

The zoekt index lives at `index/` in the state directory (resolved via `brain.IndexDir()`). The MCP `brain_search` and `brain_count` handlers call `ensureIndex` before every call:

1. Compute a fingerprint of the vault (mtimes of every `.md` / `.mdx` file under the vault root).
2. Compare to the stored state in the index directory.
3. If stale (or no index exists), re-index the vault silently — progress is not written to stderr to avoid corrupting the MCP JSON-RPC framing.
4. Persist the new fingerprint and file list.

Any `.md` / `.mdx` mtime change anywhere in the vault triggers staleness on the next search. To rebuild manually (e.g. after a config change):

```bash
brain index            # refresh if stale
brain index --force    # unconditional rebuild
```

## Smoke testing

To verify `brain mcp` speaks JSON-RPC correctly without involving Claude Code:

```bash
( printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}\n{"jsonrpc":"2.0","method":"notifications/initialized"}\n{"jsonrpc":"2.0","id":2,"method":"tools/list"}\n'; sleep 1 ) | brain mcp
```

Expected: one `initialize` response followed by a `tools/list` response listing all thirteen `brain_*` tools with their input and output schemas. The `sleep 1` keeps stdin open long enough for the server to flush responses before exit.
