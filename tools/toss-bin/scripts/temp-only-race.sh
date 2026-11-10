#!/usr/bin/env bash
# Races a --temp-only run against a process that swaps a directory on the
# path for a symlink to an outside directory while the run is moving files.
#
# Fails when any round moves a file from outside the temp roots. Passes when
# no round did and at least one swap landed while a run still had files to
# move. When no swap lands mid-run, the attempt proves nothing either way:
# the script retries the whole attempt, and after the last one prints a SKIP
# line and exits 0, because a loaded machine is not a broken binary.
#
# Usage: temp-only-race.sh <toss-bin>
#   RACE_FILES (default 1000) arguments per run, RACE_ROUNDS (default 3)
#   rounds per swap kind, RACE_ATTEMPTS (default 3) attempts before the
#   inconclusive skip.
#
# Every directory is fresh scratch: the run's files under /tmp, the outside
# victims under $TMPDIR (/var/folders on macOS, outside every root), and the
# trash under a scratch home set through CFFIXED_USER_HOME, so the real
# ~/.Trash never fills up.
set -uo pipefail

TOSS="$1"
FILES=${RACE_FILES:-1000}
ROUNDS=${RACE_ROUNDS:-3}
ATTEMPTS=${RACE_ATTEMPTS:-3}

IN=$(mktemp -d /tmp/toss-bin-race.XXXXXX)
OUT=$(mktemp -d)
TRASH_HOME=$(mktemp -d)
trap '/bin/rm -rf "$IN" "$OUT" "$TRASH_HOME"' EXIT

probe=$("$TOSS" --temp-only --validate "$OUT/probe" 2>/dev/null)
if ! grep -q '^BLOCKED:' <<<"$probe"; then
    echo "FAIL: scratch directory $OUT is not outside the temp roots; set TMPDIR"
    exit 1
fi

# Waits until the run has trashed r1, then swaps. `link`: the symlinked parent
# w (pointing at real) is repointed at the outside directory in one rename.
# `real`: the real parent directory itself is replaced by such a symlink.
# The new link exists before the wait, so the swap itself is renames only.
# Leaves a marker when the last file was still waiting, so the swap landed
# while the run had work left.
swap() {
    local dir="$1" kind="$2" outside="$3"
    ln -s "$outside" "$dir/swap-link"
    while [ -e "$dir/real/r1" ]; do
        [ -e "$dir/done" ] && return
    done
    if [ "$kind" = link ]; then
        mv -h -f "$dir/swap-link" "$dir/w"
        [ -e "$dir/real/r$FILES" ] && touch "$dir/mid-run"
    else
        mv "$dir/real" "$dir/real.old" && mv "$dir/swap-link" "$dir/real"
        [ -e "$dir/real.old/r$FILES" ] && touch "$dir/mid-run"
    fi
}

# One run against one swap. Sets MOVED (outside files that moved) and
# LANDED (1 when the swap landed mid-run), then removes the round's scratch.
race_round() {
    local dir="$1" outside="$2" kind="$3"
    mkdir -p "$dir/real" "$outside"
    (cd "$dir/real" && seq -f 'r%g' 1 "$FILES" | xargs touch)
    (cd "$outside" && seq -f 'r%g' 1 "$FILES" | xargs touch)
    ln -s "$dir/real" "$dir/w"

    local parent="$dir/w"
    [ "$kind" = real ] && parent="$dir/real"
    local args=() i
    for i in $(seq 1 "$FILES"); do
        args+=("$parent/r$i")
    done

    swap "$dir" "$kind" "$outside" &
    local swapper=$!
    CFFIXED_USER_HOME="$TRASH_HOME" "$TOSS" --temp-only "${args[@]}" >/dev/null 2>&1
    touch "$dir/done"
    wait "$swapper"

    MOVED=$((FILES - $(find "$outside" -type f | wc -l)))
    LANDED=0
    [ -e "$dir/mid-run" ] && LANDED=1
    /bin/rm -rf "$dir" "$outside"
}

runs=0
for attempt in $(seq 1 "$ATTEMPTS"); do
    mid_run=0
    for kind in link real; do
        for round in $(seq 1 "$ROUNDS"); do
            race_round "$IN/$attempt-$kind-$round" "$OUT/$attempt-$kind-$round" "$kind"
            runs=$((runs + 1))
            if [ "$MOVED" -ne 0 ]; then
                echo "FAIL: a swapped parent ($kind, attempt $attempt, round $round) moved $MOVED files from outside the temp roots"
                exit 1
            fi
            mid_run=$((mid_run + LANDED))
        done
    done
    echo "race: attempt $attempt: $((2 * ROUNDS)) runs of $FILES files, $mid_run swapped mid-run, 0 outside files moved"
    if [ "$mid_run" -gt 0 ]; then
        echo "PASS: a parent swapped mid-run never moves files outside the temp roots"
        exit 0
    fi
done

echo "SKIP: no swap landed mid-run in $ATTEMPTS attempts ($runs runs, none moved an outside file); the race was not exercised, so this proves nothing"
exit 0
