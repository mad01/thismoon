# ADR-0011: one configuration surface across components

**Date:** 2026-08-29
**Status:** Accepted

**Scope:** every component under `services/` and `tools/`, plus the shared
`kit/confdir`, `kit/envdefault`, and `kit/notify` packages. Decided after an
inventory of every flag, environment variable, and config file in the repo,
reviewed against current CLI and MCP practice.

## Context

Twenty components grew their configuration independently, and the inventory
found the drift that predicts: one service's port is a bare literal in a
flag default while the rest read a `facts.go` constant; one component
honors `XDG_CONFIG_HOME` and one honors `XDG_CACHE_HOME` while eighteen
hardcode `~/.config`; a config file is relocatable by flag in three
components, by environment variable in two, and not at all in two more.
`expandTilde` exists in thirteen copies across two implementations, and two
of them fall back to a cwd-relative path when the home directory cannot be
resolved. That is how a tool running under a stripped environment wrote a
`.config/` tree into whatever directory it happened to start in, and how
another read a config from `/.config/...` with a working directory of `/`.

None of this was decided. Each choice made sense where it landed, and
nothing wrote down the rule the next component should follow, so every new
component picked from the spread it found. This ADR is the missing rule.

Not every difference is drift. The parse-failure posture in particular is
deliberate and stays: a guard tool that shrugs off a broken config stops
guarding, while a service that refuses to start on one takes down a
timeline nobody was looking at.

## Decision

**Ports.** A service port comes from the 7423+ block and is declared as a
constant in the component's `facts.go`, never as a bare literal in a flag
default. The operating doc, the help text, and the running binary then
cannot disagree about which port the service uses.

**Precedence.** Flag, then environment variable, then config file, then
compiled default. The environment read happens inside the flag's default so
cobra resolves the whole chain in one pass, and `kit/envdefault` is that
read. A value that is set but unparseable warns once on stderr and falls
back: a typo in a port number is worth a word, and not worth refusing to
start over.

**Config location.** Every component with a config file accepts both
`--config` and `<NAME>_CONFIG` to relocate it. Paths resolve through
`kit/confdir`, which honors `XDG_CONFIG_HOME` when it holds an absolute
path and uses `~/.config/<component>` otherwise. A home directory that
cannot be resolved is an error. A relative path is never a fallback.

**Directory kinds.** Config, cache, and state are separate. Nothing but
configuration belongs under `~/.config/<tool>`. State honors
`XDG_STATE_HOME` and falls back to `~/.local/state/<component>`, and
`confdir.StateDir` takes a legacy directory that wins when it exists on
disk, so csl keeps its index and present keeps its page store where they
already are while a fresh install lands in the right place.

**Parse failure.** The posture is a per-component choice about what the
component is for, made once and written down in its `config.md`. Guard
tools (belt, suspenders) fail closed and loudly on a config that is present
but invalid. Services may degrade to defaults, and then must say so on
stderr. A hard startup error, which is what prs does, is equally correct.
Succeeding quietly on a config nobody can parse is the one outcome ruled
out.

**Defaults are constants.** Every user-facing default is a `facts.go`
constant. The pattern only pays when it leaks nowhere.

**Binding.** HTTP services bind loopback explicitly. Where a service needs
CORS, the allowed origins are an allowlist of loopback and `*.this`
origins, never `*`.

**MCP.** Instructions are always generated from `agentdoc.Facts`. Every
server exposes a read-only `<name>_doctor` tool, because a client with no
shell can call a tool but cannot run `<bin> doctor`, and that was the only
recovery path the instructions used to name. Every `mcp` command's help
prints `agentdoc.RegistrationSnippet`. `--base-url` is display-only: it
decorates the links tools return and does not route traffic, and its help
text has to say so, since the name reads like a client setting.

**Sharing.** Mechanical config code that would otherwise be copied per
component goes through `kit`: directory resolution and tilde expansion in
`kit/confdir`, environment defaults in `kit/envdefault`, the events archive
client in `kit/notify`. Parse-failure semantics stay per component, for the
reason above.

## Consequences

- Migration is per component and lands piecemeal. The rules bind new code
  and touched code first; a component nobody is editing keeps working. The
  inventory this ADR came from is the checklist.
- `kit` gains three packages and is a cross-repo compatibility surface, so
  their exported signatures are additive-only from here.
- Honoring XDG moves where a fresh install puts state. The legacy-directory
  argument to `confdir.StateDir` is what keeps that from being a data-loss
  event, and it is load-bearing for csl and present specifically, whose
  state has lived under `~/.config` since before the convention existed.
- Two components send `Access-Control-Allow-Origin: *` today: speak's TTS
  proxy and d-man's proxy. They are the known deviations from the binding
  rule, not exceptions to it, and the allowlist is small (loopback plus the
  `.this` suffix d-man already knows).
- Every MCP server spends one more tool on `<name>_doctor`. Tool budget is
  real, and a client that cannot shell out otherwise has no way to ask why
  a call failed.
- The config file format split stays undecided. YAML and TOML both appear
  here for historical reasons and neither is worth a migration; a new
  config file should match its neighbours rather than start a third
  convention.
