# operating belt

belt is the Claude Code hook binary: guards inspect a tool call before it
runs and deny risky ones with a reason, hints inspect a tool call after it
ran and add advisory context beside the result. The two halves are separate
by design: a hint has no denial path in its interface, so it can never block
a tool call, even by mistake.

## how it runs

There is no daemon. Claude Code spawns `belt hook <event>` or
`belt hint <event>` as a fresh process for every tool call it is wired to,
feeds the payload on stdin, and reads the decision or advice on stdout —
JSON for every event except `hint prompt`, which writes plain text because
UserPromptSubmit reads stdout directly as context. The process exits 0 even
when it denies: the deny travels in the JSON, never the exit code, so a
belt bug cannot break tool calls.

Because every invocation is a fresh process, changes apply on the next tool
call: belt config edits, Claude settings deny-list edits, hook registration
changes in the Claude settings, and a newly installed binary all take effect
immediately, with no restart and no new session.

## where config lives

The belt-owned config is {{.StorePath}}: guard and hint toggles, exclude
paths, extra patterns, claude_settings, guard rules, and the internal_names
section. `$XDG_CONFIG_HOME/belt/config.yaml` wins when that variable is set,
and `--config` or `$BELT_CONFIG` relocates the file outright (a relocated
file gets no legacy TOML fallback). The config is standalone and has no
machine-profile concept: belt reads no other tool's file, and the
provisioning layer renders one config per machine class — a guard meant for
only some machines is absent or disabled in the others' files. The Bash deny
patterns are read live from the Claude settings files, so the script guard
and the permission system share one deny list; claude_settings.enabled:
false turns that read off, leaving the guard with extra_patterns only.

The config file is optional, but a broken one is not tolerated. No file at
all means the built-in defaults. A file that is present and fails to parse
or validate makes every `belt hook` invocation deny with
`belt[config]: belt cannot read its config …` — belt cannot tell "no rules
configured" from "the rules did not load", and guessing the permissive one
would silently disarm every guard. Validation covers the values that parse
and then do nothing: a `custom_guards.<name>.event` outside bash/write, and
`mode:` values other than hard/soft or set on a guard that ignores them.

## failure modes

A deny reason is prefixed `belt[<guard-id>]:`, so a block always names the
guard that fired. Treat it as a true positive first: do not retry the same
call unchanged, and do not work around the block by writing the command to a
script or switching tools. If the deny looks wrong, surface it and let the
user decide.

Known false-positive path for `write-internal-names`: exclude_paths entries
usually name the canonical checkout, and a git worktree at another path
misses the match, so a write that is fine in the canonical checkout gets
denied in the worktree. Make the edit in the canonical checkout, or have the
worktree path added to the guard's exclude_paths.

Missing config fails in two directions, deliberately. Guards and hints
default to enabled. `git-push-main` fails closed: with no config it denies
every push to main or master, and only a rendered config that disables it or
allowlists a repo opens the door. With no internal_names section in the belt
config, `write-internal-names` and `publish-internal-names` have an empty
name list and allow every write and publish: those two fail open. `belt
doctor` reports the empty-name condition
in its config-surfaces section, and flags a broken config file there too —
when it does, the guard and hint state below that line is the defaults, not
what belt is enforcing.

The rule-driven commit guards fail open across the board: `git-identity`
and `commit-guard` are no-ops without their config sections, skip repos
whose remote does not resolve, and a malformed block_hours window blocks
nothing. Soft-mode rules and broken custom guards allow with a warn event
to the events service — check there when a guard seems silent. A hard
`commit-guard` deny names its override; `belt override set <name> --reason
"..."` (10m default, `--for` sizes it, `extend` pushes it) is the escape
hatch, not rewording the commit command — set/extend archive the reason as
a warn event, a suppressed block leaves one too, and it expires on its own.

Silence from the kof-backed hints is normal when the kof service is down or
its store is empty; the doctor kof line tells those states apart.

## version skew

`belt version -o json` reports the build of the binary on PATH. An
installed binary is live on the next tool call, so skew here means the
installed binary lags the repo: a fleet rebuild can report ok while an old
binary keeps answering. Rebuild and reinstall, then compare the version
again.

## first moves

1. `belt doctor`: build metadata, which config surface loaded, guard and
   hint enablement, kof reachability, and the resolved blocked-name list
2. `belt check bash "<command>"`, or `belt check write --file <path>
   --content "<text>"`: dry-run the guards and print each verdict
3. `belt config`: every setting in effect, with resolved defaults,
   guard allowlists, and the Claude deny patterns
4. `belt override`: every override with its state — active with remaining
   time, expired, legacy untimed, or malformed (set/extend/clear to manage)
5. `belt version -o json`, when a fix does not seem to apply: confirm the
   binary is the build you expect
