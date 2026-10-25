# srtspec

Render sandbox access manifests to srt (anthropics/sandbox-runtime) settings
JSON, check committed settings for drift, and explain the effective policy a
process launches under.

## Install

```bash
make install   # builds ~/code/bin/srtspec and codesigns it
```

On a fleet machine the `srtspec` recipe does this on `ralph up`.

## Usage

```bash
srtspec render services/events/service-info.yaml        # JSON to stdout
srtspec render recipes/claude-mcp/access.yaml -o out.json
srtspec check services/events/service-info.yaml recipes/events/events.srt.json
srtspec generate .                                      # every manifest in the repo
srtspec explain events                                  # base + local + env, with layers
srtspec docs                                            # the operating doc
```

## The manifest

```yaml
spec:
  access:
    sandbox: srt                 # srt | seatbelt | none (none and seatbelt need a reason)
    workdir: ~/.local/share/kof  # the value of the workdir symbol
    listen: 7431                 # turns on allowLocalBinding
    local: [events]              # loopback peers; also turns on allowLocalBinding
    egress: [via SRT_RUN_ALLOW_DOMAINS]   # or host / *.domain / host:port
    reads: [~/code, workdir]
    writes: [workdir, temp]      # temp is /private/tmp and /private/var/folders
    tls: true                    # enableWeakerNetworkIsolation, for Go TLS
    deny: [~/code/**/.env]       # extra read denies under the reads roots
    extra: {allowAppleEvents: true}   # raw srt settings, merged last
```

The full reference, including the fleet defaults and the machine-local
expansion layer, is in the thismoon repo's docs/SANDBOXING.md under
"Declaring access".
