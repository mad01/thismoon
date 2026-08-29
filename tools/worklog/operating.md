# operating worklog

worklog keeps resumable cross-session work state: one item per long task,
keyed by ticket id or topic slug, never by working directory. A later
session, on any machine with the store, asks "where was I" against the item
key and gets back the "Where I am" snapshot the last session wrote, plus an
append-only log and per-repo notes.

## how it runs

There is no serve process. The CLI and the MCP server are two thin frontends
over the same store code: `{{.Bin}} mcp` is a stdio process the MCP host
spawns, and every tool call reads or writes the store directly on disk,
exactly as the CLI commands do. Nothing has to stay running, and an MCP tool
call and a manual `{{.Bin}} show` always see the same state.

## where state lives

The store is a git repo at {{.StorePath}} (override with WORKLOG_DIR), one
directory per work item. CONTEXT.md holds status frontmatter, the "Where I
am" snapshot rewritten on every checkpoint, and the append-only log;
repos/<repo>.md files hold per-repo notes, created lazily. Every write is a
git commit under a synthetic identity. With remote.url set in the config
file, the store clones from it on a fresh machine and pushes after every
write; with no url it is local-only. `{{.Bin}} config` prints which config
file was read and the settings in effect.

## failure modes

MCP tools fail while the CLI works: the MCP host runs whatever binary was
registered, separately from the one on PATH. On macOS an adhoc-signed binary
whose provenance drifted (a manual copy over the installed file) is killed
on exec, which surfaces as the MCP server dying at startup. Reinstall from
the thismoon checkout with `make install` in tools/worklog (it strips xattrs
and re-signs), then restart the MCP host.

Checkpoint returns a push warning: the write and local commit succeeded;
only the push to the configured upstream failed (network, auth, or a
diverged remote). Work continues safely against the local store. Run
`{{.Bin}} sync` once the remote is reachable: it fast-forward pulls, then
pushes. Sync refuses on divergence, which means two machines wrote without
syncing; resolve that in the store repo by hand.

"where was I" finds nothing: worklog keys items by ticket or topic, not by
working directory, so being in the repo you worked in proves nothing.
`{{.Bin}} list` shows every item newest first; `{{.Bin}} search <word>`
substring-matches keys and content. Try the ticket id, then a topic word.

Every command fails with a config error: a config file that exists but
cannot be read or parsed stops worklog rather than being ignored, because it
carries the store's push remote. `{{.Bin}} config` is the exception — it
prints the problem and the defaults. Stopped pushing after an upgrade with
no error at all: the config still keys its upstream by machine profile
(remote.upstreams), which is no longer read; set remote.url instead.

Checkpoint recorded no repo: repo detection needs a real working directory.
The CLI uses its own cwd; the MCP server process runs from /, so the
worklog_checkpoint tool only detects the repo when the call passes `cwd`.
Pass `repo` explicitly to override detection.

## version skew

There is no serve process, so skew is between binaries: the MCP host keeps
running the `{{.Bin}} mcp` process it spawned, while `make install` replaces
the binary on PATH. `worklog version -o json` reports the installed build;
restart the MCP host (or the session) to pick it up.

## first moves

1. `worklog list` (confirms the binary runs and the store loads)
2. `worklog path` (prints the store root; check it exists and is a git repo)
3. `worklog search <ticket or topic word>` when the item key is unknown
4. `worklog version -o json` after an install, then restart the MCP host
5. `git -C <store root> log --oneline -5` (confirms writes are committing)
