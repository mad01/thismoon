# prs, open-PR dashboard over local checkouts

One page answering "which open pull requests need my attention?" across every
git repo checked out on this machine. prs discovers the repos itself, polls
each repo's GitHub host (github.com and GitHub Enterprise alike, through
your existing `gh` logins), and shows the open, non-draft PRs with their
review state. Closed, merged, and draft PRs never appear.

Three surfaces over one cache:

- **Web**: `http://prs.this/` (or `http://localhost:7427/`), the PR list
  with a repo filter, an author filter, newest/oldest sorting, and a reset
  button. Every row links to the PR on GitHub.
- **CLI**: `prs list`, `prs refresh`, `prs status`.
- **MCP**: `prs_list`, `prs_refresh`, `prs_status`, `prs_doctor` for agent sessions.

## Install

```bash
make install        # builds and installs to ~/code/bin/prs
```

Run the server (or let t-man supervise it):

```bash
prs serve --port 7427
t-man add --name prs -- prs serve --port 7427
```

## Configure

`~/.config/prs/config.yaml` (see `config.md` for the full reference):

```yaml
dirs:
  - ~/code/src
exclude:
  - someorg/noisy-repo
poll_interval: 5m
```

Auth needs nothing beyond an existing `gh auth login` per host. prs mints a
token per host with `gh auth token` and keeps it in memory; nothing is
written to disk.

## How fresh is it?

The poller refreshes every `poll_interval` (default 5m), and checks
staleness every 30 seconds so a laptop waking from sleep catches up quickly.
`prs refresh` (or the Refresh MCP tool) forces a cycle right now. The page
footer shows the last poll time, and repos whose fetch failed show as error
banners instead of silently vanishing.

## Docs

- `CLAUDE.md`: development guide and module layout
- `architecture.md`: structure for someone with the source open
- `why.md`: why this exists and the boundaries it keeps
- `config.md`: every flag, env var, and config key
- `prs docs`: the embedded operating doc (runtime debugging)
