#!/bin/sh
# Reconcile mods.list against Claude Code's user scope.
#
#   sh sync-mods.sh           apply: register the marketplace, then install,
#                             enable, disable or uninstall per line
#   sh sync-mods.sh --check   change nothing; print each pending action and
#                             exit 1 when there is one (ralph doctor's verify)
#
# Every change goes through the `claude plugin` CLI, which owns its own state
# files. The marketplace is a local directory, so `claude plugin list --json`
# reports each mod installed from it with `readFromFolder`. Sessions run
# that folder in the sources cache, so the pull that brought a change
# delivers it. The version bump in the mod's plugin.json marks the change:
# when the install record's `version` differs from the folder's
# `folderVersion`, the script runs `claude plugin update`. That brings the
# record in step, and the next check is clean.
#
# A line the script can't use is an error in both modes: a missing or extra
# word, an unknown state, or a mod the marketplace doesn't list. The script
# names the line, reconciles the others, and exits 1.
set -eu

marketplace=thismoon
checkout="$HOME/.config/ralph/sources/thismoon"
list="$(dirname "$0")/mods.list"
check=false
[ "${1:-}" = "--check" ] && check=true

for tool in claude jq; do
  if ! command -v "$tool" >/dev/null; then
    echo "claude-mods: skip ($tool not on PATH)"
    exit 0
  fi
done
if [ ! -f "$checkout/.claude-plugin/marketplace.json" ]; then
  echo "claude-mods: skip (no marketplace in $checkout yet)"
  exit 0
fi

pending=0
act() {
  pending=1
  if $check; then
    echo "pending: $*"
  else
    echo "+ $*"
    "$@" </dev/null
  fi
}

errors=0
bad_line() {
  errors=1
  echo "claude-mods: error: mods.list line $1: $2"
}

claude plugin marketplace list --json |
  jq -e --arg name "$marketplace" 'any(.[]; .name == $name)' >/dev/null ||
  act claude plugin marketplace add "$checkout" --scope user

installed=$(claude plugin list --json)
listed=$(jq -r '.plugins[].name' "$checkout/.claude-plugin/marketplace.json")

n=0
# The `|| [ -n "$line" ]` keeps a last line that has no newline.
while IFS= read -r line || [ -n "$line" ]; do
  n=$((n + 1))
  # "<mod> <state>"; a # starts a comment, and a blank line is skipped.
  read -r mod state extra <<EOF
${line%%#*}
EOF
  [ -n "$mod" ] || continue
  if [ -z "$state" ] || [ -n "$extra" ]; then
    bad_line "$n" "want '<mod> <state>', got '$mod${state:+ $state}${extra:+ $extra}'"
    continue
  fi
  case "$state" in
    on | off | retired) ;;
    *)
      bad_line "$n" "$mod has unknown state '$state' (want on, off or retired)"
      continue
      ;;
  esac
  # A retired mod may already be gone from the marketplace.
  if [ "$state" != retired ] && ! printf '%s\n' "$listed" | grep -qxF "$mod"; then
    bad_line "$n" "$mod is not in the $marketplace marketplace"
    continue
  fi

  id="$mod@$marketplace"
  # "<version> <folderVersion> <enabled>" for a user-scope install, else
  # empty.
  record=$(printf '%s' "$installed" | jq -r --arg id "$id" \
    '.[] | select(.id == $id and .scope == "user")
      | "\(.version) \(.folderVersion) \(.enabled)"')
  read -r version folder enabled <<EOF
$record
EOF

  case "$state" in
    on)
      if [ -z "$record" ]; then
        act claude plugin install "$id" --scope user
        continue
      fi
      [ "$enabled" = true ] || act claude plugin enable "$id" --scope user
      # A version that can't be read can't be compared, so it gets no update.
      if [ "$version" != null ] && [ "$folder" != null ] && [ "$version" != "$folder" ]; then
        act claude plugin update "$id" --scope user
      fi
      ;;
    off)
      [ "$enabled" != true ] || act claude plugin disable "$id" --scope user
      ;;
    retired)
      [ -z "$record" ] || act claude plugin uninstall "$id" --scope user
      ;;
  esac
done <"$list"

if [ "$pending" -eq 0 ] && [ "$errors" -eq 0 ]; then
  echo "claude-mods: in sync"
elif [ "$pending" -ne 0 ] && ! $check; then
  echo "claude-mods: start a new session, or run /reload-plugins, to load the change"
fi
if [ "$errors" -ne 0 ] || { [ "$pending" -ne 0 ] && $check; }; then
  exit 1
fi
