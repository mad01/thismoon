#!/bin/bash
# Checks every srt settings file a recipe ships (recipes/*/*.srt.json) and the
# wiring around it, so each sandboxed t-man service keeps the shape in
# docs/SANDBOXING.md ("Sandboxing a t-man service"). The file name is the
# srt-run label: recipes/events/events.srt.json is ~/.config/srt/events.json.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# The literal text the recipes carry; $HOME expands when t-man add runs.
# shellcheck disable=SC2016
LEDGER_LOG='--extra-log sandbox=$HOME/.local/share/speak/logs/sandbox-denials.log'

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

shopt -s nullglob
files=("$ROOT"/recipes/*/*.srt.json)
[ "${#files[@]}" -gt 0 ] || fail "no recipes/*/*.srt.json found"

for f in "${files[@]}"; do
  dir=$(dirname "$f")
  label=$(basename "$f" .srt.json)
  rel=${f#"$ROOT"/}
  recipe="$dir/recipe.toml"

  jq -e . "$f" > /dev/null || fail "$rel: not valid JSON"

  # srt rejects a file that lacks any of the six lists, even an empty one.
  jq -e '
    (.network.allowedDomains | type) == "array" and
    (.network.deniedDomains | type) == "array" and
    ([.filesystem.denyRead, .filesystem.allowRead,
      .filesystem.allowWrite, .filesystem.denyWrite] | all(type == "array"))
  ' "$f" > /dev/null || fail "$rel: needs both domain lists and all four filesystem lists"

  # Shipped settings name only hosts that are the same on every machine, as
  # the manifest declares them (deps names api.osv.dev). A machine-specific
  # host reaches the allow-list at launch through SRT_RUN_ALLOW_DOMAINS, and
  # no file may allow every host: srt rejects a bare * or *:port anyway.
  jq -e '.network.allowedDomains | all(test("^\\*(:[0-9]+)?$") | not)' "$f" > /dev/null ||
    fail "$rel: allowedDomains must not allow every host"

  jq -e '.filesystem.denyRead | index("~")' "$f" > /dev/null ||
    fail "$rel: denyRead must start from ~"

  # No sandbox may write a settings file or the sources cache the links point
  # into, or a confined process could rewrite its own policy. The ~ is the
  # literal text srt expands, so match it unexpanded.
  # shellcheck disable=SC2088
  if jq -r '.filesystem.allowWrite[]' "$f" |
    grep -qxE '~/?|~/\.config/?|~/\.config/srt(/.*)?|~/\.config/ralph(/sources(/.*)?)?/?'; then
    fail "$rel: allowWrite covers ~/.config/srt or the ralph sources cache"
  fi

  grep -qF "source = \"$label.srt.json\"" "$recipe" ||
    fail "${recipe#"$ROOT"/}: no dotfiles item with source $label.srt.json"
  grep -qF "target = \"~/.config/srt/$label.json\"" "$recipe" ||
    fail "${recipe#"$ROOT"/}: $label.srt.json is not linked to ~/.config/srt/$label.json"

  # Something in the recipe dir launches through srt-run with that path.
  launchers=$(grep -lE "srt-run\"? \"?\\\$HOME/\\.config/srt/$label\\.json" \
    "$recipe" "$dir"/*.sh 2> /dev/null || true)
  [ -n "$launchers" ] || fail "$rel: nothing in ${dir#"$ROOT"/} execs srt-run with ~/.config/srt/$label.json"

  # Every t-man service the recipe registers reads the denial ledger, and
  # registers after the step that keeps srt-run executable.
  while IFS= read -r line; do
    case $line in
      *"$LEDGER_LOG"*) ;;
      *) fail "${recipe#"$ROOT"/}: a t-man add lacks $LEDGER_LOG" ;;
    esac
  done < <(grep -F 't-man add --name' "$recipe")
  grep -qF '"builds.speak_chmod_scripts"' "$recipe" ||
    fail "${recipe#"$ROOT"/}: the service hook must depend on builds.speak_chmod_scripts"
done

printf 'PASS: srt settings and t-man wiring for %d sandboxed services\n' "${#files[@]}"
