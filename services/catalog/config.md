# catalog configuration

## Where config lives

catalog has no settings file. Its only input is the registry: a YAML file
listing the repo roots to scan for `service-info.yaml` files.

Resolution order, first match wins:

1. `--registry <path>` (persistent flag, every command accepts it)
2. `$CATALOG_REGISTRY`
3. `$XDG_CONFIG_HOME/catalog/registry.yaml`, when that variable holds an
   absolute path
4. `~/.config/catalog/registry.yaml` (default)

A leading `~` is expanded wherever it comes from: the flag, the environment
variable, the default, and each `sources[].path` inside the registry.
Earlier releases expanded only the default, so a `--registry ~/...` passed by
a launchd agent (which never runs a shell) reached `os.ReadFile` as a
literal `~` path and crash-looped, while the same command worked in a
terminal. A home directory that cannot be resolved is now an error before
any subcommand runs, never a path relative to the working directory.

A missing registry is not fatal: `catalog web` starts with an empty catalog
instead of crash-looping, and `catalog config` reports the path as
`missing`.

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

- `--registry <path>` (string, default `~/.config/catalog/registry.yaml`,
  env `CATALOG_REGISTRY`): overrides the registry path. Persistent, so it
  applies to every subcommand (`list`, `validate`, `web`, `config`).
- `web --port <int>` (int, default `7575`, env `CATALOG_PORT`): port
  `catalog web` listens on. The server binds `127.0.0.1` only, regardless of
  the port chosen. 7575 sits outside the platform's 7423+ service block on
  purpose and stays where it is.

## Environment variables

Each mirrors a flag above and is overridden by it when both are set:
`CATALOG_REGISTRY`, `CATALOG_PORT`. An unparseable `CATALOG_PORT` warns once
on stderr and falls back to 7575 rather than refusing to start.

To see what a given environment actually resolved to, run `catalog config`
for the registry path and `catalog doctor` for whether it reads and loads.

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
