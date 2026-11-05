# Team knowledge base

The team's shared knowledge (runbooks, decision records, incident
learnings, system facts, vendor doc and paper summaries) lives in a
markdown vault served by the `brain` CLI, which is on PATH. `brain --help`
lists the commands and `brain <command> --help` shows their flags. Read and
write the vault only through brain.

The vault has four tiers:
- `inbox/`: raw captures, status raw. New findings go here first.
- `notes/`: validated knowledge.
- `outputs/`: finished artifacts such as briefs and reports.
- `MOCs/`: maps of content that index a domain.

When you write:
- Search first. If a note on the subject exists, update it with
  `brain edit` instead of adding a second note.
- A capture of external material is `--type source` with `--source <url>`.

When you answer from the vault, cite every note you used by wikilink
(`[[stem]]`) or vault path (`notes/stem.md`). If the vault does not hold the
answer, say so instead of guessing.
