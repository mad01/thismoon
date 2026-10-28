# why srtspec

## The problem

Every sandboxed process in the fleet runs under a hand-written srt settings
file. The JSON says what the kernel enforces, but not why: which entry is the
listening port, which read is the binary, which host is a machine's own.
Reviewers had to reverse-engineer intent from six lists. The same rules (no
bare `*` host, `~` read-denied, no write into the policy directory) were
re-checked by a shell test after the fact, when they could have been
impossible to write in the first place.

On a machine, the question flips: what does this process actually get once
the launcher adds the hosts from `SRT_RUN_ALLOW_DOMAINS` and any local
widening? Answering it meant reading two files and an environment variable
and merging them by hand. The launcher itself was a shell script with the
widening rules written a second time in jq.

## Why its own tool

The manifest (spec.access in service-info.yaml, or a recipe's access.yaml)
is a short YAML block a reviewer reads in seconds. Something has to turn it
into the JSON srt reads, check the committed JSON never drifts from it,
merge a machine's layers into it at launch, and explain the merged result
on a live machine. Those jobs share one schema and one merge, so they live
in one binary; the schema itself sits in `kit/access`, so any other Go
tool can read it too.

## Why this shape

No serve process, no store, no MCP server: every command reads files and
exits, and run ends by becoming srt. Rendering is deterministic so CI can
diff it. run and explain share one merge and run the same fixtures, so
explain cannot promise a policy the launcher would refuse.
