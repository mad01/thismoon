#!/usr/bin/env bash
# Run the humanizer STE check over the with-arm replies of one plugin-eval run.
#
# Usage: ste-pass.sh <results-dir | aggregate-result.json>
#
# The replies come from aggregate-result.json, where each case's llm grader
# carries the full reply as its evidence. Brief cases get the procedure
# profile; report, round-trip and context cases get the description profile.
# Fenced code blocks are dropped first: by the contract a quoted denial or
# tool error sits in one, and the STE rules apply to the worker's own words.
# A fence around the whole reply only wraps it, so its two fence lines go and
# the reply inside is checked. Nothing else is dropped. Each reply is written
# to replies/ beside the JSON so a finding can be read in place; finding line
# numbers count from the reply without its fenced blocks.
#
# Exit 0: every reply is clean. Exit 1: findings at warning level or above.
# Exit 2: bad input, a missing tool, or a humanizer error.
set -euo pipefail

die() { echo "ste-pass: $*" >&2; exit 2; }

[ $# -eq 1 ] || die "usage: ste-pass.sh <results-dir | aggregate-result.json>"
src=$1
[ -d "$src" ] && src=$src/aggregate-result.json
[ -f "$src" ] || die "no aggregate-result.json at $1"
command -v jq >/dev/null || die "jq not found"
humanizer ste --help >/dev/null 2>&1 || die "humanizer with the ste command not found on PATH"

out=$(dirname "$src")/replies
mkdir -p "$out"

profile_for() {
  case $1 in
    */brief/*) echo procedure ;;
    *) echo description ;;
  esac
}

# Print the reply without its fenced blocks. A block closes only on a bare
# fence at least as long as the one that opened it, so a ```` block can hold
# a ``` block. When the fence on the first line closes on the last line, it
# wraps the whole reply: drop those two lines and keep what is between them.
drop_fences() {
  awk '
    function ticks(s) { sub(/^[[:space:]]+/, "", s); return match(s, /^`+/) ? RLENGTH : 0 }
    function closes(s, width) { return s ~ /^[[:space:]]*`+[[:space:]]*$/ && ticks(s) >= width }
    { line[NR] = $0 }
    END {
      first = 1; last = NR
      while (first < last && line[first] ~ /^[[:space:]]*$/) first++
      while (last > first && line[last] ~ /^[[:space:]]*$/) last--
      width = ticks(line[first])
      if (width >= 3) {
        i = first + 1
        while (i < last && !closes(line[i], width)) i++
        if (i == last && closes(line[last], width)) skip[first] = skip[last] = 1
      }
      width = 0
      for (i = 1; i <= NR; i++) {
        if (i in skip) continue
        if (width) { if (closes(line[i], width)) width = 0; continue }
        if (ticks(line[i]) >= 3) { width = ticks(line[i]); continue }
        print line[i]
      }
    }
  ' "$1"
}

# One JSON line per with-arm run: case name, case dir, run index, reply.
runs() {
  jq -c '
    .cases[] as $c
    | ($c.graders | map(select(.type == "llm")) | first | .name) as $judge
    | $c.arms.with | to_entries[]
    | {name: $c.name, dir: $c.dir, run: .key,
       reply: ((.value.graders[] | select(.name == $judge) | .evidence) // "")}
  ' "$src"
}

checked=0 failed=0 skipped=0
while IFS= read -r row; do
  name=$(jq -r .name <<<"$row")
  run=$(jq -r .run <<<"$row")
  file=$out/$name-$run.md
  jq -r .reply <<<"$row" >"$file"
  if [ -z "$(tr -d '[:space:]' <"$file")" ]; then
    echo "SKIP $name#$run: no reply text in the results"
    skipped=$((skipped + 1))
    continue
  fi
  profile=$(profile_for "$(jq -r .dir <<<"$row")")
  rc=0
  report=$(drop_fences "$file" | humanizer ste --profile "$profile" --min-severity warning --json -) || rc=$?
  [ "$rc" -le 1 ] || die "humanizer ste failed on $file"
  total=$(jq '.summary.total' <<<"$report")
  checked=$((checked + 1))
  if [ "$total" -eq 0 ]; then
    echo "PASS $name#$run ($profile)"
    continue
  fi
  failed=$((failed + 1))
  echo "FAIL $name#$run ($profile): $total finding(s) in $file"
  jq -r '.findings[] | "  line \(.line): \(.rule_id) \"\(.match)\""' <<<"$report"
done < <(runs)

echo "ste-pass: $checked checked, $failed with findings, $skipped skipped"
[ "$checked" -gt 0 ] || die "no replies to check"
[ "$failed" -eq 0 ]
