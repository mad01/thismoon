# operating srtspec

srtspec turns a sandbox access manifest into the srt
(anthropics/sandbox-runtime) settings JSON the kernel enforces, and shows
the policy a process actually launches under once srt-run merges the
machine-local layers. The manifest is the source of truth; the JSON is
derived and never edited by hand.

## how it runs

There is no serve process, no store and no MCP server. Every command reads
files and exits:

- `{{.Bin}} render <manifest> [-o out.json]` renders one manifest. A
  manifest is the spec.access block of a component's service-info.yaml, or a
  standalone access.yaml in a recipe directory.
- `{{.Bin}} check <manifest> <settings.json>` exits 1 with a diff when the
  file drifted. Key order and list order do not count as drift.
- `{{.Bin}} generate [root]` renders every manifest in a repo to its
  recipe's `<label>.srt.json`; `make sandbox-settings` runs it.
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
2. `{{.Bin}} check <manifest> <settings.json>` before editing either
3. `{{.Bin}} docs` for this page
