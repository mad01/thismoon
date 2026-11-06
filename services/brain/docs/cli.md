# brain CLI guide for agents

brain searches and edits local markdown vaults (Obsidian-style notes with YAML
frontmatter). Every command reads one config file (`vault: <path>`, or a
`vaults:` list): `--config`, else `$BRAIN_CONFIG`, else `config.yaml` in
`$XDG_CONFIG_HOME/brain` or `~/.config/brain`. Indexes live in
`$XDG_STATE_HOME/brain` or `~/.local/state/brain`, one set per vault.
Run `brain doctor` first when anything fails. `brain docs lifecycle` covers
statuses, revocation, `brain check` and freshness, `brain docs sharing` git
vaults, sync and shared vaults, and `brain docs serve` the web view's API.

## Vaults and note refs

A config can name several vaults: private ones you write, and shared (team)
ones that are read-only here: their notes change through a pull request,
which `promote --to` and `edit --pr` open. brain commits each change to a git
vault. Search, count, list and both vector searches cover every vault, and
every hit names its `vault`. A command that takes a note accepts:

- `vault:path`, such as `team:notes/topic.md`;
- `id:<ulid>`, the note's id, which survives a rename or promote;
- a bare vault-relative path, `notes/topic.md` (not `topic.md`), when exactly
  one vault has it. Found in two vaults it fails as `ambiguous` and lists the
  qualified candidates.

`write` goes to the default vault and gives the note an id (a Universally
Unique Lexicographically Sortable Identifier, ULID, in `id`) that edit and
promote never change. With several vaults, text output prints `vault:path`.

## Tiers

| Tier | Holds | Default type / status |
|---|---|---|
| `inbox` | raw captures | `note` / `raw` |
| `notes` | validated knowledge | `note` / `validated` |
| `outputs` | finished artifacts, date-prefixed names | `output` / `validated` |
| `MOCs` | maps of content (MOC): curated link lists | `moc` / `validated` |

## Global flags

- `-o, --output text|json`: text (default) or one JSON document on stdout.
  JSON shapes match the brain MCP tools. `check` and `similar` take `github`.
- `--config <path>`: the config file (env `BRAIN_CONFIG`); a bad one exits 2.
- `--vault <name>` (repeatable): restrict to these vaults; `write` takes one
  and targets it instead of the default vault.

## Commands

| Command | Does | Key flags |
|---|---|---|
| `search <query>` | zoekt search, one path per note | `--tier --tag --type --status -f/--file --limit -m content -C N --include-revoked` |
| `count <query>` | match totals | `--tier`, `--group-by` (`tier`, `tag` or `vault`) |
| `query <query>` | validate a query, print its parse | |
| `semantic-search <query>` | vector search (needs Ollama) | `--tier --tag --type --status --evidence -k --include-revoked` |
| `hybrid-search <query>` | literal + vector, fused by Reciprocal Rank Fusion | same plus `--k-zoekt --k-vector --rerank` |
| `list [tier]` | notes in a tier, newest first | `--tier --tag --status --type --limit` |
| `vaults` | each vault: shared or private, default, git state (`remote`, `local-only`, `none`), the classifications it accepts, note count; no network | `--vault` |
| `read <note>` | frontmatter and body | `--start N --end M` (body lines, 1-based) |
| `peek <note>` | backlink count, frontmatter, first lines | `--lines N` (default 20) |
| `backlinks [note]` | inbound and outbound wikilinks | `--title <title or alias>` instead of a note |
| `graph` | wikilink graph as nodes and edges | `--center <note> --depth N --tags-as-nodes` |
| `write` | create a note, unless it likely duplicates one (exit 9) | `--title` (required), `--tier --body --body-file --tag --alias --type --status --source --evidence --distinct-from --set --unset`, `--filename` (a plain file name); no `--body` reads stdin |
| `capture` | raw inbox note with `captured_by` and `expires_at`; write's checks | `--title` (else the body's first line) `--body --body-file --source <url> --by user\|agent:<session> --tag --distinct-from` |
| `edit <note>` | change body or frontmatter; with `--pr`, a shared note's through a pull request | one of `--body --append` (`-` reads stdin) `--body-file --append-file`, plus `--title --type --source --tag --alias --evidence --set --unset --if-match <sha> --pr`, `--status` (`raw`, `validated`, `promoting` or `disputed`: revoke with the three commands below; a tombstone takes no edit) |
| `promote <note>` | move an inbox note to notes/; with `--to`, open a pull request adding a validated note to a shared vault | `--title` (rename) `--tier` `--moc <a note in MOCs/>`, or `--to <vault> --force --dry-run` (`--dry-run` runs every gate and changes nothing; it needs `--to`, else exit 2) |
| `supersede <note>` | mark it superseded; `superseded_by` stores the replacement's `id:` | `--by <note>` (required) |
| `retract <note>` | mark it retracted; the body stays under a banner | `--reason error\|source-retracted\|unverifiable`, `--note <text>` |
| `withdraw <note>` | leave a tombstone and purge the note from both indexes | |
| `flag <note>` | mark it disputed, the reason in `status_reason`; a shared note gets an issue on its repo | `--reason <text>` (required), `--dry-run` |
| `drift [notes...]` | check pinned sources: code refs in local checkouts, urls with `--online`; exits 1 on drift | `--online`, and with it `--allow-local` (reach private addresses) `--timeout D` (all fetches, default 2m) |
| `stale` | notes that need review: past `review_by`, disputed, resting on a revoked source, drifted; grouped by owner | `--drift --online --allow-local --timeout D --limit N` (the fetch flags as on drift) |
| `inbox list\|keep\|archive` | raw captures: list them, extend one's expiry, archive the expired | `--expired`, `keep <note> --days N`, `archive --expired --dry-run` |
| `check [paths...]` | check notes (all of them with no paths) against the status rules, links and vault policy | `-o text\|json\|github` |
| `similar <note>` | notes alike to one in any vault; a warning, always exits 0 | `-o text\|json\|github` |
| `sync` | pull each git vault (fast-forward only), settle promotions, push private vaults | `--vault` |
| `index` | refresh indexes; semantic re-embeds changed notes only (queries refresh too) | `--force --literal-only` |
| `migrate --ids` | give every note without an id one; skips shared vaults unless named | `--dry-run` |
| `doctor` | checks `config`, `vault`, `index-dir`, `index`, `ids` (an id on two notes), `ollama` (only with the semantic layer on), then a serve on `BRAIN_PORT` or 7427: `serve-reachable`, `version-skew`, `serve-vaults`, `serve-state-dir` (skipped when nothing listens); prints the paths in effect | |
| `serve` | read-only web view of every vault (graph, notes, search, review queues) on 127.0.0.1; API in `brain docs serve` | `--port` (else `BRAIN_PORT`, else 7427) |
| `docs [topic]` | print this guide, or `lifecycle`, `sharing` or `serve` | |

Search output is one note path per line; `-m content` prints
`path:line:col: text`. `--tag` and `--alias` on write and edit are repeatable
and replace the whole list on edit.

semantic-search and hybrid-search sync a stale semantic index first. With
Ollama down the index stays as it was, a `warning:` line on stderr (and
`warnings[]` in JSON) says so, and hybrid-search fuses the literal hits alone.

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
| 2 | `usage` | bad flag, argument, path, tier or vault; a title with a control character; a note with a NUL byte or unclosed frontmatter; a note path that is a symbolic link; an edit to a revoked status or of a tombstone | `brain <cmd> --help` |
| 2 | `ambiguous` | a bare path is in more than one vault | use a candidate the message lists |
| 2 | `invalid_config` | config file does not parse or names no vault | fix the file the message names |
| 3 | `not_found` | note or title does not exist | `brain list --tier <t>`, `brain search 'f:<name>'` |
| 4 | `already_exists` | write or promote target exists, also when another writer created it first | pick another `--title` or `--filename` |
| 5 | `invalid_query` | zoekt query does not parse | `brain query '<query>'` |
| 6 | `read_only` | write, edit, promote, migrate or a revocation in a shared vault; `promote --to` a private vault | `edit --pr`, `promote --to <shared vault>`, `flag` |
| 7 | `conflict` | `edit --if-match` sha no longer matches the note | read the note again, then edit |
| 8 | `secret_found` | the note would hold a secret; nothing was written | remove it, or mark a false positive `suspenders:ignore` |
| 9 | `possible_duplicate` | write or capture: a note with the same title or near-identical content exists; nothing was written | `brain edit` a candidate, or `--distinct-from <candidate>` |

Errors print one line on stderr: `brain <cmd>: <message>`. With `-o json`
the same failure also prints on stdout:

```json
{"error": {"code": "not_found", "message": "vault: note not found: notes/x.md (find it with: ...)"}}
```

`brain doctor` and `brain check` print their report (`{"ok": false, ...}`)
instead of an error envelope and exit 1 when it fails.

## JSON shapes

| Command | Top-level keys |
|---|---|
| `search` | `output_mode`, `files[] {vault, path, evidence, status, review_by, stale}` or `lines[]` (the same plus `line, column, text, before, after`), `total` |
| `count` | `total`, `groups[] {group, count}` |
| `query` | `valid`, `parsed` |
| `list` | `tier`, `total` (before `--limit`), `notes[] {vault, path, title, status, type, tags, aliases, evidence, updated}` |
| `read` | `vault`, `path`, `sha`, `banner`, `frontmatter`, `body_lines[] {line, text}` |
| `peek` | `vault`, `path`, `banner`, `frontmatter`, `preview[]`, `backlink_count` |
| `backlinks` | `vault`, `note_path`, `evidence`, `inbound[] {vault, path, line, excerpt, evidence}`, `outbound[] {target, vault, path, line, evidence, dangling}` |
| `graph` | `nodes[] {id, vault, path, stem, title, tier, type, status, tags, evidence, review_by, degree, dangling}`, `edges[] {source, target, kind, resolved}`; `kind` is `link`, `moc` (a link out of a MOC), `supersedes`, `source` (a `sources[]` note ref) or `tag` |
| `write`, `edit` | `vault`, `path`, `warnings[]`; edit adds `sha`, the new content's |
| `supersede`, `retract`, `withdraw`, `flag` | `vault`, `path`, `sha` |
| `check` | `ok`, `checked`, `errors`, `warnings`, `findings[] {vault, path, line, rule, level, message}`, `skipped[] {path, reason}` |
| `promote` | `vault`, `old_path`, `new_path`, `backlink_rewrites[]`, `moc_appended` |
| `migrate` | `dry_run`, `changed[] {vault, path, id}`, `skipped[] {vault, path, reason}` |
| `semantic-search` | `output_mode`, `hits[] {vault, path, id, title, heading_path, score, snippet, tier, tags, type, status, review_by, stale, evidence}`, `total`, `warnings[]` |
| `hybrid-search` | `hits[] {vault, path, id, title, heading_path, score, sources, snippet, ...}`, `total`, `warnings[]` |
| `doctor` | `ok`, `checks[] {name, status, detail, fix}`; status is `ok`, `fail` or `skip` |

Freshness and sharing shapes: `brain docs lifecycle`, `brain docs sharing`.
`frontmatter` holds `title aliases tags type status source evidence created
updated id`, plus `extra` with every other key (the lifecycle fields too).
`evidence` is the trust tier (A, B, C, D or mixed), from the field or an
`evidence/<tier>` tag. Empty fields are omitted.

## Zoekt query tips

- Space is AND within one file: `kubernetes upgrade`. OR is `a|b` with no spaces.
- `"exact phrase"`, `-word` excludes, `case:yes` forces case; lowercase terms
  match any case.
- `f:<regex>` filters paths: `f:^notes/`, `f:alpha` (prefer `--tier` for tiers).
- Terms are regexes: escape metacharacters, `foo\.bar`, `func\(`.
- Not sure? `brain query '<query>'` shows the parse or the error with a hint.

## Workflow: capture, verify, promote

1. Search before writing: `brain search '<topic>' -o json`, then `brain peek`
   or `brain read` the hits. Extend an existing note with `brain edit --append`
   rather than duplicating it; write refuses a likely duplicate (exit 9).
2. Capture new material to the inbox:
   `brain capture --title 'Topic' --source <url> --by agent:<session> < notes.md`
   (body from stdin). It lands in `inbox/` with `status: raw` and expires in
   30 days unless verified or kept.
3. Verify empirical claims against primary sources. Record the result:
   `brain edit inbox/topic.md --evidence B --append -`.
4. Promote when the note stands on its own: `brain promote inbox/topic.md
   --moc MOCs/<topic>.md` moves it to `notes/` under its title's slug, sets
   `status: validated`, and points `[[wikilinks]]` at the new name. The MOC
   is a topical map in `MOCs/`, not a `research-index-*` or `kickoff-*` file.
5. Finished artifacts go to outputs: `brain write --tier outputs --title '...'`.
6. To share a validated note, `brain vaults` shows what each shared vault
   accepts, and `brain promote <note> --to <vault> --dry-run` runs every gate
   and changes nothing. Drop `--dry-run` to open the pull request.

Check links before moving or renaming: `brain backlinks <path>`.
