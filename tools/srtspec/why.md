# why srtspec

## The problem

Every sandboxed process in the fleet runs under a hand-written srt settings
file. The JSON says what the kernel enforces, but not why: which entry is the
listening port, which read is the binary, which host is a machine's own.
Reviewers had to reverse-engineer intent from six lists, and the same rules
(no bare `*` host, `~` read-denied, no write into the policy directory) were
re-checked by a shell test after the fact instead of being impossible to
write.

On a machine, the question flips: what does this process actually get once
srt-run adds the hosts from `SRT_RUN_ALLOW_DOMAINS` and any local widening?
Answering it meant reading two files and an environment variable and merging
them by hand.

## Why its own tool

The manifest (spec.access in service-info.yaml, or a recipe's access.yaml)
is a short YAML block a reviewer reads in seconds. Something has to turn it
into the JSON srt reads, check the committed JSON never drifts from it, and
explain the merged result on a live machine. Those three jobs share one
schema, so they live in one binary; the schema itself sits in
`kit/access`, where the catalog reads it too.

## Why this shape

No serve process, no store, no MCP server: every command reads files and
exits. Rendering is deterministic so CI can diff it. explain mirrors srt-run's
merge rules exactly and runs the same shared fixtures, so it cannot promise a
policy the launcher would refuse.
