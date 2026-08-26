# toss-bin configuration

## Where config lives

`~/.config/toss-bin/config.yaml`, YAML, optional. toss-bin runs on its
built-in deny-list alone when the file is absent; the file only adds to
that list, it can never unblock a built-in entry. It is read only when
`--safe-mode` or `--validate` runs. `$TOSS_BIN_CONFIG` overrides the path.

A malformed file logs a warning to stderr (`toss-bin: warning: ignoring
<path>: <error>`) and the run continues on built-ins alone, so a YAML typo
never breaks `rm`.

## Keys

- `protected_paths` ([]string, default `[]`): exact-match paths to add to
  the deny-list. Blocks the path itself; files inside it still pass.
- `protected_trees` ([]string, default `[]`): subtree paths to add to the
  deny-list. Blocks the path and everything under it.

Entries accept a leading `~`, which expands to the home directory; trailing
slashes are stripped; duplicates, including of built-in entries, are
harmless. Config-added trees get none of the `treeAllowList` exemptions
that built-in system trees have (like `/usr/local` under `/usr`).

## Environment variables

- `TOSS_BIN_CONFIG` (string, default unset): overrides the config file
  path in place of `~/.config/toss-bin/config.yaml`.

## Example

```yaml
# ~/.config/toss-bin/config.yaml
protected_paths:
  - /Volumes/backup
protected_trees:
  - ~/code/archive
```
