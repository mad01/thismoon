# brain

The vault brain indexes and serves. These terms apply to the files under
`services/brain/`; the repo-wide terms are in the root CONTEXT.md.

## Language

**Vault**:
A directory of markdown notes brain indexes and serves, named in the brain config file (`config.md` says where it lives) by a lowercase slug. It holds the four Tier directories, and the walk skips `.git`, `.obsidian`, `.trash` and `attachments`. A vault is its own repo, kept apart from the tool. One config can name several vaults; each is a Private vault or a Shared vault.
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
One markdown file in the Vault with YAML frontmatter between `---` fences. The known fields are `title`, `aliases`, `tags`, `type`, `status`, `source`, `evidence`, `created`, `updated` and `id`; a write requires only `title`.
_Avoid_: document, page, entry

**Stem**:
A Note's filename without the `.md` extension, and the name a Wikilink uses for it. A write derives it from the title (lowercased, every run of other characters collapsed to one hyphen), and a note written to `outputs` gets a `YYYY-MM-DD-` prefix. A stem is unique within its tier directory, not across the vault.
_Avoid_: slug (the function that makes one), id (a frontmatter field)

**Wikilink**:
An Obsidian-style `[[stem]]` link in a note body, optionally carrying `|alias`, `#heading` or `^block`. It names its target by Stem.
_Avoid_: link (on its own), reference

**Backlink**:
A line in another Note whose Wikilink names this note: an inbound link. `brain_backlinks` reports a note's backlinks alongside its own outbound wikilinks, each resolved to a path where the target stem exists.
_Avoid_: mention, reference

**MOC**:
A Map of Content: a curated index note in the `MOCs` tier that lists other notes by Wikilink. Promotion can append the promoted note to one. An index written as the record of one research batch (a `research-index-` or `kickoff-` filename, or the `meta/index` tag) is refused as that target.
_Avoid_: index (on its own), hub

**Evidence tier**:
The trust grade of an empirical Note, held in the `evidence` frontmatter field. `A` is a replicated trial or meta-analysis, `B` one well-designed peer-reviewed study, `C` a preprint, survey or plausible mechanism. `D` marks a refuted or apocryphal claim kept as a do-not-cite record, and `mixed` a note that grades each claim inline. A non-empirical note leaves it empty. Reads fall back to an `evidence/<grade>` tag when the field is absent. Unrelated to Tier.
_Avoid_: confidence, rating, score

**Promotion**:
Moving an `inbox` Note into another tier, `notes` by default, through `brain promote` or `brain_promote`. It writes the note under the target tier with status `validated` and a Stem made from its title. It then rewrites every Wikilink to the old stem across the vault and deletes the inbox file. If the rewrite fails, the new file is removed again; rewrites that already landed stay.
_Avoid_: move, publish, graduate
