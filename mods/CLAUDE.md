# CLAUDE.md - mods

Claude Code mods: in-process plugin modules that draw bands, panes, and toasts and observe events, beside the skills, hooks, and services in this repo. A mod may also carry a pi face, loaded by the pi coding agent from the same directory. The decision and its limits are in `docs/adr/0021-mods-additive-claude-ui-layer.md` and `docs/adr/0023-mods-pi-face.md`; read both before adding one.

## Layout

One complete plugin per directory under `mods/<name>/`:

```
.claude-plugin/plugin.json   name, version, description, author, optional types
hooks/hooks.json             { "modules": ["./register.tsx"] }, one module
hooks/register.tsx           export const register: Register = (on, options) => { ... }
lib/*.ts                     pure helpers, no `$`, no pi API, imported relatively by both faces
pi/index.ts                  optional pi face: export default (pi: ExtensionAPI) => { ... }
types/index.d.ts             the PluginState contract when the mod keeps `$.state`
test/*.test.mjs              node --test over lib/ and, when present, pi/ (a fake `pi` object)
README.md                    under 60 lines: what it shows, signal sources, dev loop, what it never does
```

A mod isn't a component: no Makefile, no release-please entry, no `name/vX.Y.Z` tag. `plugin.json` carries the version, and a merge to main is the deploy, the same as a recipe.

## Rules

- Additive only. No skill, hook, or service may require a mod. A mod reads what they already produce (events, tool results, answer text) and never changes their contract. belt remains the fail-closed guard layer and the only enforcement a rule may rely on; a mod hook fails open on a throw or a timeout. A mod may return a deny in two cases only: the user's own Cancel in a hold dialog, or a convenience guard. Such a guard fails open, is off by a config key, and duplicates no belt guard.
- Every hook that does I/O or sits on a guard-shaped event carries `.catch`, which logs with `$.ui.log(text, { to: 'debug' })` and returns `next(e)`. The engine gives a hook 10 s; past it the hook is skipped and the chain goes on without it.
- Localhost probes have a 400 ms budget (`Promise.race` against `$.clock.sleep`) and stay silent when the service is down: a debug line at most, never a toast, never a blocked turn. Work that outlives a dispatch starts from `$.clock.after` or `$.clock.every` in `session.start`.
- Observe, then pass on. A `tool.call` observer calls `next(e)`, reads the result, and returns it unchanged. A `ui.render` hook on `AbovePrompt` includes `await next(e)` in its tree so a later mod's band survives.
- `claude plugin validate` reads the module statically and refuses what it cannot follow. Keep event names string literals and spell `$.noun.method` in full (never destructure `$`). Pass `$` only to functions declared at the top of the module, import relatively, and register one hook per event and matcher. Write `$.state` keys as literals matching the contract.
- State lives in `$.state`, declared under the mod's name in `types/index.d.ts` and named in `plugin.json` as `"types"`. Module variables reset on every hot reload; `$.state` survives a reload and a compaction. `/clear` puts every value back to its default (the mods test docs, https://code.claude.com/docs/en/plugins/mods/test, say a test starts the way `/clear` leaves state).
- Public repo. No employer names, internal hosts, or ticket prefixes other than `MAD-`. The belt write guard denies such writes; remove the name rather than working around it.
- Two harnesses, one lib. Claude Code loads `hooks/register.tsx` through the marketplace; pi loads `pi/index.ts` through the package manifest `mods/package.json` (`pi.extensions: ./*/pi/index.ts`), declared in pi's `settings.json` by the consuming dotfiles. Each face owns its harness's API and imports `lib/` only; neither imports the other, and `lib/` imports neither API. pi has no hook chain, so a pi face observes split events (`tool_call`, `tool_result`) or the extension bus (`pi.events`, e.g. `belt:deny` from the dotfiles permission gate) instead of `next(e)`. A pi face follows the same additive rules as the Claude face: no deny except the person's Cancel, fail open, `hasUI` guard before any dialog or status. A mod may have only a Claude face. Codex never loads a mod; anything it needs goes in a skill.

## Naming

Directory, `plugin.json` `name`, and the `plugin` field of every `$.state` reference are the same kebab-case string. Debug lines start with `<name>:`.

## Dev loop

```bash
claude --plugin-dir mods/<name>        # from a checkout; a save hot-reloads the module
claude --debug --plugin-dir mods/<name>  # the debug log names every skipped hook and refused tree
claude plugin validate mods/<name>     # before every commit
node --test mods/<name>/test/*.test.mjs
pi -p --no-session -e mods "hi"           # loads every pi face from the worktree for one run; errors print to stderr
```

On load the engine writes `.claude-plugin/types/` and a `tsconfig.json` beside the mod, both ignored by git. `claude-code/index.d.ts` there is the API for the build you run and wins over any doc; `npx -p typescript tsc -p mods/<name>` type-checks against it. `claude plugin test mods/<name>` runs `*.test.ts` files against the engine's test kit when a mod needs event-level tests.

## Marketplace

`.claude-plugin/marketplace.json` at the repo root, named `thismoon`, lists every mod with a relative `source: "./mods/<name>"`. `mods/package.json` is the pi side: a local pi package whose `pi.extensions` glob finds every `pi/index.ts`, so a new pi face needs no manifest edit either. The four planned mods are listed from the start (three as stubs), so building one of them never touches the file. A fifth mod adds its own entry; CI fails when a `mods/*` directory is missing from the list. Registering the marketplace on a machine (`claude plugin marketplace add ~/.config/ralph/sources/thismoon`) and enabling mods through `enabledPlugins` is machine-private wiring and lives in the consuming repo (ADR-0006), never here.
