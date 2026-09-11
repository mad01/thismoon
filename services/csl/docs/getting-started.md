# Getting started

From a blank Mac to your first cross-repo search, then the MCP server for
Claude Code, then the optional web UI as a background service. Everything here
runs standalone, without ralph or the rest of the fleet. Allow about ten
minutes.

## Prerequisites

- macOS on Apple Silicon. Release tarballs are darwin/arm64 only.
- `git` on `PATH`. csl reads `.git/config` directly for repo names and uses
  `git` for pull and fingerprinting.
- At least one local git checkout.
- [mise](https://mise.jdx.dev) or Homebrew for the install. Building from a
  checkout instead needs Go 1.26 or newer and the Xcode Command Line Tools
  (`xcode-select --install`), because csl compiles its tree-sitter grammars
  with cgo. The prebuilt tarball has them compiled in and needs neither.
- Optional, for semantic search only: [Ollama](https://ollama.com) (step 9).

## 1. Install

With mise. Every thismoon component releases from one repository under its own
tag prefix, so mise needs a tool alias plus the `csl/` prefix:

```sh
mise tool-alias set csl github:mad01/thismoon
mise use -g "csl[version_prefix=csl/]"
```

While the repository is private, set `MISE_GITHUB_TOKEN` to a token with read
access first (`gh auth token` works when the GitHub CLI is logged in).

With Homebrew, once the repository is public:

```sh
brew tap mad01/tap
brew install mad01/tap/csl
```

From a checkout, when you want to build it yourself:

```sh
git clone https://github.com/mad01/thismoon.git
cd thismoon/services/csl
make install    # builds with version metadata, installs to ~/code/bin/csl, codesigns
```

Verify:

```sh
csl version
```

## 2. Configure

csl reads one file, `config.yaml` in `$XDG_CONFIG_HOME/csl` or `~/.config/csl`.
List the directories that contain your git checkouts, one root per line. csl
walks each one and records every directory whose immediate child is `.git`.

```yaml
dirs:
  - ~/code/src/github.com
  - ~/workspace
```

That is the whole file for lexical search. Tildes are expanded and missing
directories are skipped. Leave `index.hosts` out until you need it: it is an
allowlist of git remote hosts, and any repo it does not match, including every
checkout with no remote, drops out of the index. Step 3 shows how to check
what got picked up and what got dropped.

Without a config file csl runs, but every command tells you which file to
create and indexes nothing. A file that exists and does not parse is an error.
`csl config` prints the path in effect and the settings after defaults.

## 3. Check what csl discovered

```sh
csl repo --list
```

One line per repo, tab-separated name and path:

```
myorg/service-a	/Users/you/code/src/github.com/myorg/service-a
mad01/thismoon	/Users/you/code/src/github.com/mad01/thismoon
```

Names come from the `origin` remote URL. A checkout with no remote is named
`<parent-dir>/<repo-dir>`. `csl repo --json` adds the `remote` and `host`
fields, which is how to see what a host filter would match against.

When a repo you expected is missing, work down this list:

1. `csl config` shows the `dirs` in effect. The repo's path must sit under one
   of them. The walk skips hidden directories (any name starting with `.`) and
   stops descending once it finds `.git`, so a repo nested inside another repo
   is never found.
2. `index.hosts`, when set, drops every repo whose remote host is not listed
   and every repo with no remote. csl reads the host literally from the
   remote URL: a checkout cloned as `git@gh-work:org/repo.git` has host
   `gh-work`, not the hostname your SSH config resolves it to, so list the
   alias.
3. `hooks.post_merge.exclude` removes repos from both indexes by absolute path
   or `org/repo` name, whatever the `enabled` flag beside it says.
4. When every repo is filtered out, csl says so: "N git repos found under the
   dirs ... and every one was dropped", with a count per filter. A wrong
   `dirs` entry gives "no git repos found under the dirs" instead.

`csl repo --list --skipped` lists the dropped repos with the reason for each,
tab-separated name, path, and reason:

```
org/noremote	/Users/you/code/noremote	no remote (index.hosts is set)
org/elsewhere	/Users/you/code/elsewhere	host git.example.com not in index.hosts
org/excluded	/Users/you/code/excluded	excluded by hooks.post_merge.exclude
```

`--json --skipped` adds each repo's `remote` and `host`, which is where an SSH
alias shows up as the host.

## 4. Run your first search

```sh
csl search "func Walk"
```

The first call builds the index and prints progress; later calls answer in
hundreds of milliseconds:

```
Indexing 17 repo(s)...
  [1/17] myorg/service-a
  ...
myorg/service-a/internal/walker/walker.go
mad01/thismoon/services/csl/internal/repo/finder/walker.go
```

Nothing has to be running. Each query pings a background search server on a
Unix socket, starts one from the same binary if none answers, and opens the
index in-process if that fails too. The server keeps the index mapped in memory
and exits after ten idle minutes.

The default output is one matched file per line. For the matching lines, or a
narrower search:

```sh
csl search "func Walk" --output-mode content --context-lines 2
csl search "TODO" --repo thismoon --lang go
```

Queries are zoekt syntax, which looks like grep but is not: space-separated
terms must all appear in the same file, OR is `|` with no spaces around it, and
`f:` takes a regex rather than a glob. `csl query "<pattern>"` shows how a
query parsed when the results surprise you.

## 5. Check health

```sh
csl doctor
```

One `ok` or `FAIL` line per check: config, repos discovered, state file, index
freshness, shard integrity, the search server, and two web probes. The
repos-discovered line carries the dropped counts whenever a filter removed
something, and it is the one search check a fresh config can fail: a loaded
config that discovers no repos, with the line naming the filter responsible.
Before the first search, index-freshness notes that nothing is indexed yet and
passes. The two web probes, web-ui-reachable and web-ui-version-skew, FAIL
until step 8 starts the web UI, and doctor exits non-zero because of them;
search and the MCP server are unaffected, so on a CLI-only install read those
two lines as "not running" rather than "broken". `csl docs` prints the
operating notes behind each check.

## 6. Keep the index fresh

`csl search` re-indexes stale repos in the background after answering, so
day-to-day searches keep themselves current. To also pull every repo and
reindex whatever changed upstream:

```sh
csl sync
```

It pulls fast-forward only, skips dirty trees, non-default branches, and
detached HEADs, then reindexes the changed repos.

Two shell functions make this a habit. `repo <query>` jumps to a checkout by
name and bare `repo` opens a fuzzy picker; `repo-sync` runs the sync. The
binary prints them:

```sh
# ~/.zshrc
eval "$(csl shell-init zsh)"
```

which defines:

```sh
repo() { local d=$(csl repo "$@"); [[ -n "$d" ]] && cd "$d"; }
repo-sync() { csl sync; }
```

`bash` is accepted too. Machines provisioned through the ralph recipe in this
repo get the same two functions generated into
`~/.config/ralph/generated/generated_functions.sh`, which the source line in
ralph's managed rc-file block loads on every shell start; a test keeps the
recipe bodies and the `shell-init` output identical.

## 7. Register the MCP server

```sh
claude mcp add --scope user csl -- csl mcp
claude mcp list        # expect: csl: csl mcp - ✓ Connected
```

`--scope user` registers it for every project; without the flag Claude Code
registers csl for the current directory only. The registration stores the bare
command name, so `csl` must be on the PATH of whatever launches Claude Code. A
Claude Code started from the Dock does not run your shell's mise activation;
register the mise shim instead, which resolves the current version itself:

```sh
claude mcp add --scope user csl -- "$HOME/.local/share/mise/shims/csl" mcp
```

Add `"mcp__csl__*"` to `permissions.allow` in `~/.claude/settings.json` to
skip the per-call prompt. Then give Claude the CLAUDE.md section that tells it
when to use the tools:

```sh
csl docs --claude-md >> ~/.claude/CLAUDE.md
```

The same text sits in the [README](../README.md#add-this-to-your-claudemd) if
you would rather paste it into a project `CLAUDE.md`. Registering the server
only makes the tools available; the CLAUDE.md section is what stops Claude
from reaching for `grep` and `find` first, and it keeps the agent on lexical
search unless you enable semantic search in step 9.

If you also run [belt](../../../tools/belt/README.md), the thismoon hook set
for Claude Code, its `prefer-csl` hint closes the loop from the other side:
when the agent runs a multi-file `grep` or `find` inside a repo csl indexes,
the hook hands back the equivalent `csl_search` call with the pattern already
translated to zoekt syntax. It reads csl's shard listing from disk and
launches no csl process. The hint is on by default once belt's hooks are
registered (`hints.prefer-csl.enabled` in belt's config); the hooks block to
register is in
[`tools/belt/docs/hooks.md`](../../../tools/belt/docs/hooks.md#prefer-csl-bash)
and the worked settings example in `examples/dotfiles/recipes/claude-hooks/`.

## 8. Optional: the web UI as a background service

```sh
csl web    # http://127.0.0.1:7424
```

The web process is the only long-lived part of csl, and it does three things
the CLI and the MCP server cannot:

- It keeps the index fresh on its own. Every 15 minutes it runs the same
  pull-and-reindex pass as `csl sync` (`refresh.interval_minutes`;
  `refresh.enabled: false` turns the loop off).
- It serves the `csl_show_file` deep links the MCP server hands to the user,
  plus a `/refresh` page showing each repo's last refresh outcome.
- It gives you a browser search box with lexical, semantic, and hybrid modes.

To keep it running, register it with [t-man](../../../tools/t-man/README.md),
a launchd wrapper that installs the same way as csl (`mise tool-alias set
t-man github:mad01/thismoon`, then `mise use -g "t-man[version_prefix=t-man/]"`):

```sh
t-man add --name csl-web -- "$HOME/.local/share/mise/shims/csl" web --port 7424
t-man status csl-web
```

Use the shim path. t-man bakes the resolved command path into the launchd
plist, so the path has to be the right one at add time. A bare `csl` resolves
to a Homebrew binary first when one exists, and the path `mise which csl` prints
carries the version number, so an agent registered with either would keep
running the old build after `mise upgrade csl`. The shim points at whatever
version mise currently has: after an upgrade, `t-man restart csl-web` picks it
up, and the web-ui-version-skew check in `csl doctor` tells you when the
running process is behind the binary on PATH. A Homebrew install can use
`brew services start mad01/tap/csl` instead of t-man.

`csl web --port` moves one process only. When the UI lives somewhere else for
good, set `CSL_PORT` or `web.base_url` in the config so the MCP server and
`csl doctor` follow it.

## 9. Optional: semantic search

Everything so far is lexical: exact text matching with no extra dependencies.
Semantic search (match by meaning: "retry failed requests" finds the backoff
loop whatever it's named) needs a local [Ollama](https://ollama.com) server
with an embedding model pulled:

```sh
brew install ollama
brew services start ollama
ollama pull unclemusclez/jina-embeddings-v2-base-code:f16
```

Enable it and build the vector index:

```yaml
semantic:
  enabled: true
```

```sh
csl index --semantic-all
csl semantic "walk directories looking for git repos"
```

The first build embeds every repo and takes a while; later runs only re-embed
changed files. `csl hybrid <query>` fuses both backends. Until you finish this
step, `csl_semantic_search` answers `available=false` and `csl_hybrid_search`
degrades to lexical-only. See [semantic search](semantic.md) for how it works
and how to pick a different model.

## Next

- [CLI reference](cli.md): every subcommand and flag.
- [Configuration reference](configuration.md): every file csl reads or writes, and where.
- [MCP reference](mcp.md): the fifteen `csl_*` tools and their contracts.
- [Web UI](web.md): the browser UI, the JSON API, and the refresh loop.
- [Semantic search](semantic.md): lexical vs semantic, the vector index, embedding models.
- [Architecture](architecture.md): how the daemon, index, and MCP server fit together.
