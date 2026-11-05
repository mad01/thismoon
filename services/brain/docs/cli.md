# brain CLI guide for agents

brain searches and edits local markdown vaults (Obsidian-style notes with YAML
frontmatter). Every command reads one config file (`vault: <path>`, or a
`vaults:` list): `--config`, else `$BRAIN_CONFIG`, else `config.yaml` in
`$XDG_CONFIG_HOME/brain` or `~/.config/brain`. Indexes live in
`$XDG_STATE_HOME/brain` or `~/.local/state/brain`, one set per vault.
Run `brain doctor` first when anything fails.

## Vaults and note refs

A config can name several vaults: private ones you write, and shared (team)
ones that are read-only here because their notes change through a pull
request. Search, count, list, semantic-search and hybrid-search cover every
vault, and every hit names its `vault`. A command that takes a note accepts:

- `vault:path`, such as `team:notes/topic.md`;
- `id:<ulid>`, the note's id, which survives a rename or promote;
- a bare vault-relative path, `notes/topic.md` (not `topic.md`), when exactly
  one vault has it. Found in two vaults it fails as `ambiguous` and lists the
  qualified candidates.

`write` goes to the default vault and gives the note an id (a Universally
Unique Lexicographically Sortable Identifier, ULID, in the `id` field); edit
and promote never change it. With more than one vault configured, text output
prints `vault:path`.

## Tiers

| Tier | Holds | Default type / status |
|---|---|---|
| `inbox` | raw captures | `note` / `raw` |
| `notes` | validated knowledge | `note` / `validated` |
| `outputs` | finished artifacts, date-prefixed names | `output` / `validated` |
| `MOCs` | maps of content (MOC): curated link lists | `moc` / `validated` |

## Global flags

- `-o, --output text|json`: text (default) or one JSON document on stdout.
  JSON shapes match the brain MCP tools.
- `--config <path>`: the config file (env `BRAIN_CONFIG`). A file that exists
  but does not parse or names no vault fails every command with exit 2.
- `--vault <name>` (repeatable): restrict to these vaults; `write` takes one
  and targets it instead of the default vault.

## Commands

| Command | Does | Key flags |
|---|---|---|
| `search <query>` | zoekt search, one path per note | `--tier --tag --type --status -f/--file --limit -m content -C N` |
| `count <query>` | match totals | `--tier`, `--group-by` (`tier`, `tag` or `vault`) |
| `query <query>` | validate a query, print its parse | |
| `semantic-search <query>` | vector search (needs Ollama) | `--tier --tag --type --status --evidence -k` |
| `hybrid-search <query>` | literal + vector, fused by Reciprocal Rank Fusion | same plus `--k-zoekt --k-vector --rerank` |
| `list [tier]` | notes in a tier, newest first | `--tier --tag --status --type --limit` |
| `read <note>` | frontmatter and body | `--start N --end M` (body lines, 1-based) |
| `peek <note>` | backlink count, frontmatter, first lines | `--lines N` (default 20) |
| `backlinks [note]` | inbound and outbound wikilinks | `--title <title or alias>` instead of a note |
| `graph` | wikilink graph as nodes and edges | `--center <note> --depth N --tags-as-nodes` |
| `write` | create a note | `--title` (required), `--tier --body --tag --alias --type --status --source --evidence --filename --set --unset`; no `--body` reads stdin |
| `edit <note>` | change body or frontmatter | `--body` or `--append` (`-` reads stdin), `--title --status --type --source --tag --alias --evidence --set --unset --if-match <sha>` |
| `promote <note>` | move an inbox note to notes/ | `--title` (rename) `--tier` `--moc <MOC path>` |
| `index` | refresh indexes; semantic re-embeds changed notes only (queries refresh too) | `--force --literal-only` |
| `migrate --ids` | give every note without an id one; skips shared vaults unless named | `--dry-run` |
| `doctor` | check config, vaults, index, duplicate ids, Ollama; prints the paths in effect | |
| `docs` | print this guide | |

Search output is one note path per line; `-m content` prints
`path:line:col: text`. `--tag` and `--alias` on write and edit are repeatable
and replace the whole list on edit.

semantic-search and hybrid-search sync a stale semantic index first. When
Ollama does not answer, the index stays as it was and a `warning:` line on
stderr (and `warnings[]` in JSON) says so; hybrid-search then fuses the
literal hits alone.

A wikilink resolves by stem or path, then by title or alias, ignoring case
and never by prefix. A private vault's links look in that vault first, then in
the shared vaults; a shared vault's links look only in shared vaults. A link
that resolves nowhere is dangling. backlinks, peek, promote and graph all use
this one resolver; backlink lines count from the top of the file.

## Frontmatter: --set and --unset

`--set key=value` sets any frontmatter key and is repeatable. The value is
YAML, so quote it for the shell:

```sh
brain edit inbox/x.md --set owner=alex --set 'tags=[ai, research]'
brain edit inbox/x.md --set 'sources=[{url: https://example.com, note: primary}]'
brain edit inbox/x.md --unset classification
```

Typed keys (`title aliases tags type status source evidence created updated id`)
must have the right shape (`tags` takes a list, `title` a string); a wrong shape
is a usage error. Other keys keep the value as given. Keys brain does not model
survive write, edit and promote unchanged, in their original order. `--set` and
`--unset` apply after the tier defaults on write and after the patch flags on
edit; `updated` is always refreshed on edit.

## Exit codes

| Code | JSON `code` | Meaning | Typical fix |
|---|---|---|---|
| 0 | | success | |
| 1 | `error` | anything else (config, I/O, Ollama) | `brain doctor` |
| 2 | `usage` | bad flag, argument, path, tier or vault | `brain <cmd> --help` |
| 2 | `ambiguous` | a bare path is in more than one vault | use a candidate the message lists |
| 2 | `invalid_config` | config file does not parse or names no vault | fix the file the message names |
| 3 | `not_found` | note or title does not exist | `brain list --tier <t>`, `brain search 'f:<name>'` |
| 4 | `already_exists` | write or promote target exists, also when another writer created it first | pick another `--title` or `--filename` |
| 5 | `invalid_query` | zoekt query does not parse | `brain query '<query>'` |
| 6 | `read_only` | write, edit, promote or migrate in a shared vault | change it through a PR to its repo |
| 7 | `conflict` | `edit --if-match` sha no longer matches the note | read the note again, then edit |

Errors print one line on stderr: `brain <cmd>: <message>`. With `-o json`
the same failure also prints on stdout:

```json
{"error": {"code": "not_found", "message": "vault: note not found: notes/x.md (find it with: ...)"}}
```

`brain doctor` prints its report (`{"ok": false, "checks": [...]}`) instead of
an error envelope and exits 1 when a check fails.

## JSON shapes

| Command | Top-level keys |
|---|---|
| `search` | `output_mode`, `files[] {vault, path, evidence}` or `lines[] {vault, path, line, column, text, before, after, evidence}`, `total` |
| `count` | `total`, `groups[] {group, count}` |
| `query` | `valid`, `parsed` |
| `list` | `tier`, `total` (before `--limit`), `notes[] {vault, path, title, status, type, tags, aliases, evidence, updated}` |
| `read` | `vault`, `path`, `sha`, `frontmatter`, `body_lines[] {line, text}` |
| `peek` | `vault`, `path`, `frontmatter`, `preview[]`, `backlink_count` |
| `backlinks` | `vault`, `note_path`, `evidence`, `inbound[] {vault, path, line, excerpt, evidence}`, `outbound[] {target, vault, path, line, evidence, dangling}` |
| `graph` | `nodes[] {id, vault, path, stem, title, tier, type, status, tags, evidence, review_by, degree, dangling}`, `edges[] {source, target, kind, resolved}` |
| `write`, `edit` | `vault`, `path`; edit adds `sha`, the new content's |
| `promote` | `vault`, `old_path`, `new_path`, `backlink_rewrites[]`, `moc_appended` |
| `migrate` | `dry_run`, `changed[] {vault, path, id}`, `skipped[] {vault, path, reason}` |
| `semantic-search` | `output_mode`, `hits[] {vault, path, id, title, heading_path, score, snippet, tier, tags, type, status, evidence}`, `total`, `warnings[]` |
| `hybrid-search` | `hits[] {vault, path, id, title, heading_path, score, sources, snippet, ...}`, `total`, `warnings[]` |
| `doctor` | `ok`, `checks[] {name, status, detail, fix}`; status is `ok`, `fail` or `skip` |

`frontmatter` holds `title aliases tags type status source evidence created
updated id`, plus `extra` with every other key. `evidence` is the trust tier
(A, B, C, D or mixed), taken from the field or an `evidence/<tier>` tag.
Empty fields are omitted.

## Zoekt query tips

- Space is AND within one file: `kubernetes upgrade`. OR is `a|b` with no spaces.
- `"exact phrase"`, `-word` excludes, `case:yes` forces case.
- `f:<regex>` filters paths: `f:^notes/`, `f:alpha` (prefer `--tier` for tiers).
- Terms are regexes: escape metacharacters, `foo\.bar`, `func\(`.
- Smart case: lowercase terms match any case.
- Not sure? `brain query '<query>'` shows the parse or the error with a hint.

## Workflow: capture, verify, promote

1. Search before writing: `brain search '<topic>' -o json`, then `brain peek`
   or `brain read` the hits. Extend an existing note with `brain edit --append`
   rather than duplicating it.
2. Capture new material to the inbox:
   `brain write --title 'Topic' --source <url> < notes.md` (body from stdin).
   It lands in `inbox/` with `status: raw`.
3. Verify empirical claims against primary sources. Record the result:
   `brain edit inbox/topic.md --evidence B --append -`.
4. Promote when the note stands on its own:
   `brain promote inbox/topic.md --moc MOCs/<topic>.md`. The note moves to
   `notes/` under the slug of its title (`--title` changes it), `status`
   becomes `validated`, and `[[wikilinks]]` across the vault follow the new
   name. MOC targets must be topical maps, not `research-index-*` or
   `kickoff-*` files.
5. Finished artifacts go to outputs: `brain write --tier outputs --title '...'`.

Check links before moving or renaming: `brain backlinks <path>`.
