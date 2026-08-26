# catalog configuration

## Where config lives

catalog has no settings file. Its only input is the registry: a YAML file
listing the repo roots to scan for `service-info.yaml` files. There is no
environment variable in the resolution path.

Resolution order, first match wins:

1. `--registry <path>` (persistent flag, every command accepts it)
2. `~/.config/catalog/registry.yaml` (default)

A leading `~` in the flag value or in the default is expanded. A missing
registry is not fatal: `catalog web` starts with an empty catalog instead of
crash-looping, and `catalog config` reports the path as `missing`.

## Keys

The registry has one top-level key:

- `sources` ([]object, required): list of repo roots to scan. Each entry:
  - `sources[].path` (string): a repo root, absolute or `~`-prefixed. The
    scanner walks it recursively for `service-info.yaml` files, skipping
    `.git`, `node_modules`, `vendor`, `.idea`, `dist`, and `build`. A source
    that is not checked out on the current machine is skipped without
    complaint, so the same registry can list more repos than any one
    machine has.

## Flags

- `--registry <path>` (string, default `~/.config/catalog/registry.yaml`):
  overrides the registry path. Persistent, so it applies to every
  subcommand (`list`, `validate`, `web`, `config`).
- `web --port <int>` (int, default `7575`): port `catalog web` listens on.
  The server binds `127.0.0.1` only, regardless of the port chosen.

## Example

```yaml
# ~/.config/catalog/registry.yaml
sources:
  - path: ~/code/src/github.com/mad01/dotfiles
  - path: ~/code/src/github.com/mad01/thismoon
```

`catalog config` prints which file resolved and how many sources it holds
(`loaded, N sources`, `missing`, or a parse error), without dumping the
entities themselves. Use `catalog list` for that.
