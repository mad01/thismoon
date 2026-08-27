# operating humanizer

humanizer flags AI-writing patterns in prose and computes quantitative voice
profiles. It is detection first: findings are deterministic, with rule ids
and positions, and any rewriting is left to the calling agent. There is no
serve process and no daemon. Every run, CLI or MCP, is a short-lived local
process, and everything runs offline. This doc ships embedded in the
binary; print it anytime with `humanizer docs`.

## how it runs

One binary (`{{.Bin}}`, installed to ~/code/bin) with two frontends over the
same internal functions: the CLI (`humanizer detect`, `profile`, `rules`,
`lint`, `fix`, `rewrite`) and an MCP stdio server (`humanizer mcp`) that the
MCP host spawns per session. Both call the same code in-process, so CLI and
MCP results never diverge.

Span detection is the one part that is not pure Go: it runs the `vale` prose
linter as a subprocess, resolved from PATH, against a Humanizer style pack
embedded in the binary and extracted to a cache directory on first use. The
statistical detector, voice profiler, and watermark scrub (`lint`/`fix`) are
pure Go with no subprocess and keep working when vale is missing.

## where state lives

Nothing persists between runs: humanizer reads no config file, keeps no
store, and writes no log. The one on-disk artifact is the extracted vale
style pack, default {{.StorePath}} (HUMANIZER_CACHE_DIR or XDG_CACHE_HOME
override it). It is a regenerable cache: a run rewrites any file that
drifted from the embedded copy, so deleting the directory is always safe.

## failure modes

MCP tools absent entirely: registration is machine-private wiring that lives
outside this component, so a session may simply not have the server
registered. On macOS a killed server is the other cause: taskgated kills a
signed binary whose provenance xattr no longer matches, which presents as
the stdio server dying at launch. Reinstall from the thismoon checkout
(`make install` in tools/humanizer strips the xattr and re-signs), then
restart the session.

Detect fails while other tools work: the `vale` binary is missing from
PATH. Detection is the only path that needs it. Confirm with
`humanizer_status` (MCP) or `which vale`; install with `brew install vale`
or `go install github.com/errata-ai/vale/v3/cmd/vale@latest`.

`humanizer rewrite` fails while detection works: the `ollama` and
`openai-compatible` backends are CLI-only and need a reachable model
endpoint (loopback only unless `--allow-remote`). The default print-prompt
backend and the MCP `humanizer_rewrite` tool build the prompt offline and
are unaffected.

`humanizer_detect_file` refuses a path: the MCP server runs sandboxed and
reads only .md/.markdown/.txt files under the sandbox profile's workspace
roots, plus /tmp paths. Pass the text inline via `humanizer_detect` instead.

## version skew

There is no serve process, so skew is registered-versus-installed: after an
upgrade, a live MCP host session keeps running the stdio server it spawned
from the old binary. `humanizer version -o json` reports the installed
build; the running server advertises its own build as serverInfo.version in
the MCP handshake. When they differ, restart the MCP host session.

## first moves

1. `humanizer version -o json` (binary present, and which build)
2. `humanizer_status` over MCP, or `which vale`, to confirm the detection engine
3. `echo "It is important to note that we delve deeper." | humanizer detect` end to end
4. If MCP tools are missing: check the host's server registration, then reinstall and restart the session
5. If the style pack looks corrupted: delete {{.StorePath}}; the next run re-extracts it
