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
- For a multi-line body, write it to a file in the current directory with
  the Write tool and pass `--body-file <file>` (`--append-file` on edit).
- Run one brain command per Bash call, with no pipes. Use `-o json` when
  you need structured output.
- Never write a credential (password, token, API key, a secret in a
  connection string) into the vault, your answer or a command, not even
  to check it is gone. Record where the secret lives, not its value.

When you answer from the vault, cite every note you used by wikilink
(`[[stem]]`) or vault path (`notes/stem.md`). If the vault does not hold the
answer, say so instead of guessing.
