# operating srtspec

srtspec turns a sandbox access manifest into the srt
(anthropics/sandbox-runtime) settings JSON the kernel enforces, and shows
the policy a process actually launches under once srt-run merges the
machine-local layers. The manifest is the source of truth; the JSON is
derived and never edited by hand.

## how it runs

There is no serve process, no store and no MCP server. Every command reads
files and exits, except run, which ends by replacing itself with srt:

- `{{.Bin}} render <manifest> [-o out.json]` renders one manifest. A
  manifest is the spec.access block of a component's service-info.yaml, or a
  standalone access.yaml in a recipe directory.
- `{{.Bin}} check <manifest> <settings.json>` exits 1 with a diff when the
  file drifted. Key order and list order do not count as drift.
- `{{.Bin}} generate [root]` renders every manifest in a repo to its
  recipe's `<label>.srt.json`; `make sandbox-settings` runs it.
- `{{.Bin}} verify [root]` checks every render, orphan and expansion layer
  under a repo without writing, one line per check; `make sandbox-verify`
  runs it in CI and a consuming repo's test calls it directly.
- `{{.Bin}} run <settings> [--] <command...>` is what `~/code/bin/srt-run`
  execs for every sandboxed service and MCP server. It merges
  `~/.config/srt/<label>.local.json` and SRT_RUN_ALLOW_DOMAINS into
  `~/.config/srt/merged/<label>.json`, picks bun or node, prints
  `srt-run: <label> runtime <bun|node>` to stderr, and execs srt with the
  command as `env SRT_RUN=<label> <command...>`.
- `{{.Bin}} explain <label>` reads `~/.config/srt/<label>.json`, then
  `~/.config/srt/<label>.local.json`, then SRT_RUN_ALLOW_DOMAINS, and prints
  each effective entry with the layer it came from.

## failure modes

"unknown field": the manifest is decoded strictly, so a typo in a field
name fails instead of silently dropping a deny. The fields are sandbox,
reason, label, workdir, listen, local, egress, reads, writes, tls, deny and
extra.

"needs a reason": a manifest with sandbox none or seatbelt must say why the
process is not under srt.

"a bare * allows every host": egress names hosts. A host that differs per
machine is written as the literal `via SRT_RUN_ALLOW_DOMAINS`, and the
wrapper passes the real host at launch.

"reaches ~/.config/srt": no sandbox may write the settings or the ralph
sources cache, or a confined process could rewrite its own policy.

"no manifest renders": generate found a recipes/*/*.srt.json without a
manifest. Declare spec.access (or an access.yaml), or delete the file.

"FAIL" lines from verify: each names a file and the fix. A settings file
that "drifted" or "does not exist" wants `generate`. One that "matches in
meaning but not in layout" was hand-edited, so generate again. A layer that
"srt-run would refuse" must only widen.

run says "may only widen the policy; refusing to launch": the process did
not start, and the lines under it name each problem with
`~/.config/srt/<label>.local.json`. Exit 1. "srt not found on PATH", "no bun
under" and "cannot find srt's dist/cli.js" are exit 127: install srt (and
bun) with mise, or set SRT_RUN_RUNTIME=node. Exit 2 is a usage error: a
missing command, a label that is not a bare word, or an unknown runtime.

explain says "srt-run refuses to launch": the process will not start. The
local layer may only add to allowedDomains, allowRead, allowWrite,
allowMachLookup and allowUnixSockets, and set allowLocalBinding or
enableWeakerNetworkIsolation to true. Fix or delete
`~/.config/srt/<label>.local.json`.

## version skew

Nothing runs between commands, so the only skew is a stale binary:
`{{.Bin}} version -o json` reports the installed build. Settings files are
read at spawn time; after changing one, restart the process it confines.

## first moves

1. `{{.Bin}} explain <label>` to see what a sandboxed process may reach
2. `{{.Bin}} run ~/.config/srt/<label>.json <command>` to probe a process by hand
3. `{{.Bin}} check <manifest> <settings.json>` before editing either
4. `{{.Bin}} docs` for this page
