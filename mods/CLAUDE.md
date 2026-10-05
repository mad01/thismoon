# CLAUDE.md - mods

Claude Code mods: in-process plugin modules that draw bands, panes, and toasts and observe events, beside the skills, hooks, and services in this repo. The decision and its limits are in `docs/adr/0021-mods-additive-claude-ui-layer.md`; read it before adding one.

## Layout

One complete plugin per directory under `mods/<name>/`:

```
.claude-plugin/plugin.json   name, version, description, author, optional types
hooks/hooks.json             { "modules": ["./register.tsx"] }, one module
hooks/register.tsx           export const register: Register = (on, options) => { ... }
lib/*.ts                     pure helpers, no `$`, imported relatively
types/index.d.ts             the PluginState contract when the mod keeps `$.state`
test/*.test.mjs              node --test over lib/
README.md                    under 60 lines: what it shows, signal sources, dev loop, what it never does
```

A mod isn't a component: no Makefile, no release-please entry, no `name/vX.Y.Z` tag. `plugin.json` carries the version, and a merge to main is the deploy, the same as a recipe.

## Rules

- Additive only. No skill, hook, or service may require a mod. A mod reads what they already produce (events, tool results, answer text) and never changes their contract. belt remains the fail-closed guard; a mod hook fails open on a throw or a timeout.
- Every hook that does I/O or sits on a guard-shaped event carries `.catch`, which logs with `$.ui.log(text, { to: 'debug' })` and returns `next(e)`. The engine gives a hook 10 s; past it the hook is skipped and the chain goes on without it.
- Localhost probes have a 400 ms budget (`Promise.race` against `$.clock.sleep`) and stay silent when the service is down: a debug line at most, never a toast, never a blocked turn. Work that outlives a dispatch starts from `$.clock.after` or `$.clock.every` in `session.start`.
- Observe, then pass on. A `tool.call` observer calls `next(e)`, reads the result, and returns it unchanged. A `ui.render` hook on `AbovePrompt` includes `await next(e)` in its tree so a later mod's band survives.
- `claude plugin validate` reads the module statically and refuses what it cannot follow. Keep event names string literals and spell `$.noun.method` in full (never destructure `$`). Pass `$` only to functions declared at the top of the module, import relatively, and register one hook per event and matcher. Write `$.state` keys as literals matching the contract.
- State lives in `$.state`, declared under the mod's name in `types/index.d.ts` and named in `plugin.json` as `"types"`. Module variables reset on every hot reload; `$.state` survives a reload and a compaction. `/clear` puts every value back to its default (the mods test docs, https://code.claude.com/docs/en/plugins/mods/test, say a test starts the way `/clear` leaves state).
- Public repo. No employer names, internal hosts, or ticket prefixes other than `MAD-`. The belt write guard denies such writes; remove the name rather than working around it.
- Claude-only. Codex and pi never load a mod; anything they need goes in a skill.

## Naming

Directory, `plugin.json` `name`, and the `plugin` field of every `$.state` reference are the same kebab-case string. Debug lines start with `<name>:`.

## Dev loop

```bash
claude --plugin-dir mods/<name>        # from a checkout; a save hot-reloads the module
claude --debug --plugin-dir mods/<name>  # the debug log names every skipped hook and refused tree
claude plugin validate mods/<name>     # before every commit
node --test mods/<name>/test/*.test.mjs
```

On load the engine writes `.claude-plugin/types/` and a `tsconfig.json` beside the mod, both ignored by git. `claude-code/index.d.ts` there is the API for the build you run and wins over any doc; `npx -p typescript tsc -p mods/<name>` type-checks against it. `claude plugin test mods/<name>` runs `*.test.ts` files against the engine's test kit when a mod needs event-level tests.

## Marketplace

`.claude-plugin/marketplace.json` at the repo root, named `thismoon`, lists every mod with a relative `source: "./mods/<name>"`. The four planned mods are listed from the start (three as stubs), so building one of them never touches the file. A fifth mod adds its own entry; CI fails when a `mods/*` directory is missing from the list. Registering the marketplace on a machine (`claude plugin marketplace add ~/.config/ralph/sources/thismoon`) and enabling mods through `enabledPlugins` is machine-private wiring and lives in the consuming repo (ADR-0006), never here.
