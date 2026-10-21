# `codesign` recipe

Creates the self-signed "mad01 Local Signing" certificate in the login
keychain and trusts it for code signing. Every component Makefile in this
repo signs its installed binary with it (`docs/adr/0004`), so a rebuild keeps
the binary's code-signing identity. With an ad-hoc signature the identity
changes on every build. macOS Background Task Management (BTM) then revokes
the launchd agent's approval, and a KeepAlive service wedges in "spawn
scheduled" until `t-man restart` re-registers it.

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`),
and ralph merges the recipe with the identity `thismoon/codesign`. The hook
`working_dir` therefore names the sources cache
(`~/.config/ralph/sources/thismoon/recipes/codesign`), not a dev checkout.

## What this recipe does

- **Wave 0.** Within a wave ralph orders items alphabetically by key, so
  `builds.codesign_cert` runs before every `packages.*` build in the same
  wave and the component installs find the cert.
- **`hooks.builds.codesign_cert`** runs `bash ./setup.sh` once
  (`run = "once"`). The script is idempotent: it exits early when the
  certificate is already in the keychain.
- No packages, symlinks, or uninstall hooks.

The item key `hooks.builds.codesign_cert` is the one the consuming repo used
before the recipe moved here, so ralph's run-once record carries over. Its
freshness check hashes the hook's working dir, which changed with the move,
so the first `ralph up` after the move runs `setup.sh` once more; it finds
the cert and exits.

## Files

- `setup.sh`: creates the certificate with openssl (RSA 2048, 3650 days,
  `extendedKeyUsage=codeSigning`), imports it into
  `~/Library/Keychains/login.keychain-db` and trusts it for code signing.
- `sign.sh`: `mad01_sign <binary> [identifier]` for ad-hoc scripts, sourced
  or run directly; falls back to ad-hoc signing when the cert is absent.
  Component Makefiles do not use it. They carry the inline `resign` block
  instead, so each component stays self-contained.

## Signing a new component

Copy the `resign` target from `tools/humanizer/Makefile` and call it from
`install` with `BIN=<installed path>`. It signs with the cert and the
identifier `com.mad01.<binary>`, and falls back to ad-hoc when the cert is
missing.

Verify after install: `codesign -dvv ~/code/bin/<binary>` shows
`Identifier=com.mad01.<binary>` and `Authority=mad01 Local Signing`.

## Working with it

```bash
ralph up    # runs codesign_cert if it has not run yet

# Re-run after a keychain reset (run = "once" will not on its own)
ralph up --build=codesign_cert --force
bash ~/.config/ralph/sources/thismoon/recipes/codesign/setup.sh   # same, without ralph

# Is the cert there?
security find-identity -v -p codesigning | grep "mad01 Local Signing"

# Sign a binary by hand
~/.config/ralph/sources/thismoon/recipes/codesign/sign.sh ~/code/bin/my-tool com.mad01.my-tool
```

## Gotchas

- `run = "once"` means ralph does not re-run `setup.sh` when the cert goes
  missing later (a keychain reset, say). Run the script by hand, or
  `ralph up --build=codesign_cert --force`.
- `security add-trusted-cert` prompts for the macOS login password. There is
  no way around the prompt.
- The cert is valid for 3650 days. When it expires, delete it from the
  keychain by hand first, then run `setup.sh`.
- A missing cert makes every Makefile fall back to ad-hoc without failing,
  and services then break on the next rebuild. `codesign -dvv` showing
  `Signature=adhoc` on a `~/code/bin` binary means the fallback fired: run
  `setup.sh`, then reinstall.
- Moving a binary from ad-hoc to the cert (or to a new cert) counts as a new
  identity once: `t-man restart <service>` re-registers the agent.
