# brain

The vault brain indexes and serves. These terms apply to the files under
`services/brain/`; the repo-wide terms are in the root CONTEXT.md.

## Language

**Vault**:
A directory of markdown notes brain indexes and serves, named in the brain config file (`config.md` says where it lives) by a lowercase slug. It holds the four Tier directories, and the walk skips `.git`, `.obsidian`, `.trash`, `.brain` (where the Vault policy and the Inbox archive live) and `attachments`. A vault is its own repo, kept apart from the tool. One config can name several vaults; each is a Private vault or a Shared vault.
_Avoid_: brain (that is the tool), data repo

**Private vault**:
A Vault one person writes directly: write, edit and promote change its files in place. Any vault the config does not mark `shared` is private.
_Avoid_: personal vault (a name a config may give one), local vault

**Shared vault**:
A Vault a team keeps in a shared repo, marked `shared: true` in the config. brain searches, lists and reads it like any vault, but refuses a direct write, edit or promote with `read_only`: shared notes change through a pull request.
_Avoid_: team vault (a name a config may give one), remote vault, public vault

**Default vault**:
The Private vault a write targets when no `--vault` names one: the vault marked `default: true`, else the first private vault in config order. A config of shared vaults alone has none.
_Avoid_: main vault, primary vault

**Note ref**:
How a command names one Note: `vault:path` (qualified), `id:<ulid>` (by Note id), or a bare vault-relative path that must exist in exactly one vault. A bare path found in more than one vault is `ambiguous`, never guessed.
_Avoid_: slug

**Note id**:
The Universally Unique Lexicographically Sortable Identifier (ULID) in a Note's `id` frontmatter field. A write assigns one when the note has none, `brain migrate --ids` backfills older notes, and no edit, promote or rename changes it, so it names the note when its path does not. Two notes holding one id (a copied file) is a fault `brain doctor` reports.
_Avoid_: uuid, note key, stem

**Tier**:
One of the four top-level vault directories a Note lives in: `inbox`, `notes`, `outputs` and `MOCs`. A note's tier is the first component of its vault-relative path, and it sets the default `type` and `status` a new note gets: `raw` in `inbox`, `validated` everywhere else. Not the same thing as an Evidence tier.
_Avoid_: folder, stage, category

**Note**:
One markdown file in the Vault with YAML frontmatter between `---` fences. The known fields are `title`, `aliases`, `tags`, `type`, `status`, `source`, `evidence`, `created`, `updated` and `id`; a write requires only `title`. The fields of its Status (`expires_at`, `review_by`, `status_at` and the rest) are kept with the keys brain does not model, so a note written before them still reads.
_Avoid_: document, page, entry

**Stem**:
A Note's filename without the `.md` extension, and the name a Wikilink uses for it. A write derives it from the title (lowercased, every run of other characters collapsed to one hyphen), and a note written to `outputs` gets a `YYYY-MM-DD-` prefix. A stem is unique within its tier directory, not across the vault.
_Avoid_: slug (the function that makes one), id (a frontmatter field)

**Wikilink**:
An Obsidian-style `[[stem]]` link in a note body, optionally carrying `|alias`, `#heading` or `^block`; an embed, `![[stem]]`, counts as one. It names its target by Stem, by a path ending in the target's path, or by the target's title or an alias, all without regard to case. One resolver decides every link's target, for backlinks and the graph alike. It tries stems (or paths) before titles and aliases. It looks in the linking note's own Vault first, then in the Shared vaults in config order. A link in a shared vault never resolves into a Private vault. Within a vault, a name two notes share goes to the one in `notes`, then `MOCs`, `outputs`, `inbox`.
_Avoid_: link (on its own), reference

**Dangling link**:
A Wikilink no note resolves to. Its target exists nowhere the resolver looks, or only in a vault it may not look in, such as a private note named from a shared vault. A target is never matched by prefix, so `[[team]]` does not reach `team-guide`. The graph shows one dangling node per target.
_Avoid_: broken link, orphan (a note nothing links to)

**Backlink**:
A line in another Note with a Wikilink that resolves to this note: an inbound link. `brain_backlinks` reports a note's backlinks alongside its own outbound wikilinks, each with the note it resolves to or marked dangling.
_Avoid_: mention, reference

**MOC**:
A Map of Content: a curated index note in the `MOCs` tier that lists other notes by Wikilink. Promotion can append the promoted note to one. An index written as the record of one research batch (a `research-index-` or `kickoff-` filename, or the `meta/index` tag) is refused as that target.
_Avoid_: index (on its own), hub

**Evidence tier**:
The trust grade of an empirical Note, held in the `evidence` frontmatter field. `A` is a replicated trial or meta-analysis, `B` one well-designed peer-reviewed study, `C` a preprint, survey or plausible mechanism. `D` marks a refuted or apocryphal claim kept as a do-not-cite record, and `mixed` a note that grades each claim inline. A non-empirical note leaves it empty. Reads fall back to an `evidence/<grade>` tag when the field is absent. Unrelated to Tier.
_Avoid_: confidence, rating, score

**Promotion**:
Moving an `inbox` Note into another tier, `notes` by default, through `brain promote` or `brain_promote`. It writes the note under the target tier with status `validated` and a Stem made from its title. It then points every Wikilink that resolved to the note at its new stem, or its new path for a path link, and deletes the inbox file. A link by an old title keeps that title as display text. Links inside code are left alone. If the rewrite fails, the new file is removed again; rewrites that already landed stay.
_Avoid_: move, publish, graduate

**Status**:
Where a Note stands in its lifecycle, held in the `status` field: `raw`, `validated`, `promoting`, `disputed`, `superseded`, `retracted` or `withdrawn`. An empty field means the default of the note's Tier. A write dates a raw note with `expires_at` (30 days) and a validated one with `review_by` (from its `volatility`: 90 days when `high`, 365 otherwise), and every change of status records `status_at`. `expired` is computed, never stored: a raw note past `expires_at`, or a validated note past `review_by`.
_Avoid_: state, stage, lifecycle stage

**Revoked note**:
A Note that is superseded, retracted or withdrawn. Default search hides it, `--include-revoked` (or `include_revoked`) shows it, and read and peek print a banner above it. Revoking never deletes the file, so links to it still resolve. Superseded points `superseded_by` at its replacement's Note id; retracted keeps its body with `retracted_reason` (`error`, `source-retracted` or `unverifiable`) and `retracted_at`; withdrawn leaves a Tombstone.
_Avoid_: deleted note, archived note

**Capture**:
A raw Note `brain capture` writes into `inbox/`, for an agent or a quick human note: status `raw`, `captured_by` naming who captured it (`user` or `agent:<session>`) and `expires_at` 30 days out. A capture expires unless it is promoted or kept (see Inbox archive).
_Avoid_: draft, clip, scrap

**Possible duplicate**:
An existing Note a new one likely repeats, found before a write: one in the target Vault or a Shared vault whose title or an alias equals the new title once case and punctuation are folded, or, where a semantic index exists, one at least 0.92 alike in content. Revoked notes are never one. A possible duplicate refuses the write (exit 9) until the writer names it in `distinct_from`, the list of notes the new one is distinct from although alike.
_Avoid_: duplicate (on its own: the check only says likely), conflict, collision (a taken filename, exit 4)

**Flag**:
Marking a Note `disputed` with `brain flag`, which stores the reason in `status_reason`. A flag is a doubt, not a verdict: the note stays in search under a DISPUTED banner until someone validates it again or revokes it. A Shared vault takes no direct flag; there a flag is an issue on the vault's repo.
_Avoid_: report, challenge, mark (on its own)

**Inbox archive**:
The vault's `.brain/archive/`, where `brain inbox archive --expired` moves raw Notes past their `expires_at`, each under the path it had. The walk skips `.brain`, so an archived note is out of search, Check and the link graph, yet the file and its git history stay in the vault; moving it back under `inbox/` restores it. Keeping a note (`brain inbox keep`) is the one action that extends a raw note's expiry; an edit never does.
_Avoid_: trash, deleted notes, expired folder

**Drift**:
A pinned source of a Note that no longer matches what it points at, as `brain drift` finds it: a code ref (`repo:path@commit#symbol` in `sources[].ref`) whose path moved, whose content changed since the commit, or whose symbol is gone, looked up in a local checkout under a configured code root; or an external `url` that is gone or no longer matches its `content_hash`. A source drift cannot check is unresolved, which is not drift.
_Avoid_: staleness (that is review_by), rot, breakage

**Tombstone**:
What `brain withdraw` leaves of a Note: frontmatter of only `id`, `status: withdrawn` and `status_at`, and no body. The note's chunks leave both indexes when the command returns. Its old text survives in the vault's git history.
_Avoid_: stub, placeholder

**Citation**:
A Wikilink from one Note to another, judged by the target's Status. A citation of a note that is not validated, or is expired, is worth a warning; a new citation of a retracted or withdrawn note is an error in Check.
_Avoid_: reference, source (a `sources` entry)

**Classification**:
How widely a Note may travel, held in `classification`: `public`, `internal`, `confidential` or `restricted`. The Vault policy says which values a vault accepts.
_Avoid_: sensitivity, visibility, label

**Vault policy**:
The vault's `.brain/policy.yaml`: the Classifications it accepts and the level (`error`, `warning` or `off`) of each Check rule. A vault without the file gets the lenient default, where a missing optional field is a warning.
_Avoid_: config (that is the brain config file), rules file

**Check**:
`brain check`, which judges notes against the Status rules, their Citations, unique ids and stems, the Vault policy and the secret scan. It is the gate a Shared vault's CI runs on the notes a pull request changes, and exits 1 when a finding is at error level.
_Avoid_: lint, validate (the status), doctor (which checks the install)
