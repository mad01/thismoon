# brain CLI guide for agents

brain searches and edits a local markdown vault (Obsidian-style notes with YAML
frontmatter). Every command reads one config file (`vault: <path>`): `--config`,
else `$BRAIN_CONFIG`, else `config.yaml` in `$XDG_CONFIG_HOME/brain` or
`~/.config/brain`. Indexes live in `$XDG_STATE_HOME/brain` or
`~/.local/state/brain`.
Paths are vault-relative and include the tier directory: `notes/topic.md`, not
`topic.md`. Run `brain doctor` first when anything fails.

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

## Commands

| Command | Does | Key flags |
|---|---|---|
| `search <query>` | zoekt search, one path per note | `--tier --tag --type --status -f/--file --limit -m content -C N` |
| `count <query>` | match totals | `--tier`, `--group-by` (`tier` or `tag`) |
| `query <query>` | validate a query, print its parse | |
| `semantic-search <query>` | vector search (needs Ollama) | `--tier --tag --type --status --evidence -k` |
| `hybrid-search <query>` | literal + vector, fused by Reciprocal Rank Fusion | same plus `--k-zoekt --k-vector --rerank` |
| `list [tier]` | notes in a tier, newest first | `--tier --tag --status --type --limit` |
| `read <path>` | frontmatter and body | `--start N --end M` (body lines, 1-based) |
| `peek <path>` | backlink count, frontmatter, first lines | `--lines N` (default 20) |
| `backlinks [path]` | inbound and outbound wikilinks | `--title <title or alias>` instead of a path |
| `write` | create a note | `--title` (required), `--tier --body --tag --alias --type --status --source --evidence --filename --set --unset`; no `--body` reads stdin |
| `edit <path>` | change body or frontmatter | `--body` or `--append` (`-` reads stdin), `--title --status --type --source --tag --alias --evidence --set --unset` |
| `promote <path>` | move an inbox note to notes/ | `--title` (rename) `--tier` `--moc <MOC path>` |
| `index` | rebuild indexes (search does this when stale) | `--force --literal-only` |
| `doctor` | check config, vault, index, Ollama; prints the paths in effect | |
| `docs` | print this guide | |

Search output is one note path per line; `-m content` prints
`path:line:col: text`. `--tag` and `--alias` on write and edit are repeatable
and replace the whole list on edit.

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
| 2 | `usage` | bad flag, argument, path or tier | `brain <cmd> --help` |
| 2 | `invalid_config` | config file does not parse or names no vault | fix the file the message names |
| 3 | `not_found` | note or title does not exist | `brain list --tier <t>`, `brain search 'f:<name>'` |
| 4 | `already_exists` | write or promote target exists | pick another `--title` or `--filename` |
| 5 | `invalid_query` | zoekt query does not parse | `brain query '<query>'` |

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
| `search` | `output_mode`, `files[] {path, evidence}` or `lines[] {path, line, column, text, before, after, evidence}`, `total` |
| `count` | `total`, `groups[] {group, count}` |
| `query` | `valid`, `parsed` |
| `list` | `tier`, `total` (before `--limit`), `notes[] {path, title, status, type, tags, aliases, evidence, updated}` |
| `read` | `path`, `frontmatter`, `body_lines[] {line, text}` |
| `peek` | `path`, `frontmatter`, `preview[]`, `backlink_count` |
| `backlinks` | `note_path`, `evidence`, `inbound[] {path, line, excerpt, evidence}`, `outbound[] {target, path, line, evidence}` |
| `write`, `edit` | `path` |
| `promote` | `old_path`, `new_path`, `backlink_rewrites[]`, `moc_appended` |
| `semantic-search` | `output_mode`, `hits[] {path, title, heading_path, score, snippet, tier, tags, type, status, evidence}`, `total` |
| `hybrid-search` | `hits[] {path, title, heading_path, score, sources, snippet, ...}`, `total` |
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
