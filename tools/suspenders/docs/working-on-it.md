# Working on suspenders by hand

A task-oriented guide for changing suspenders yourself: building it, running it against a real secret, adding or tuning a detection rule, and debugging a false positive or false negative.

This is the hands-on companion to the [README](../README.md), which holds the reference catalog (every rule, config key, and CLI flag) and the "How it works" walkthrough. When this guide says "the rule tables," it means [README → Detection rules](../README.md#detection-rules). For why staged scanning fails the commit instead of skipping an unreadable file, see [`adr/`](adr/).

## The hands-on loop

Prerequisites: Go 1.26+, golangci-lint, and Docker (the last only for integration tests).

```sh
make build              # compile to ./suspenders
(cd ../.. && tools/suspenders/suspenders scan)   # run the fresh binary against the monorepo
make test               # unit tests (fast; no Docker)
make install            # install to ~/code/bin — the binary your hooks run
```

The fixture-path ignores live in the monorepo root `.suspenders.yaml`; `scan` resolves that file from the scan root or, when scanning a subdirectory, from the enclosing git top-level. Scanning from inside `tools/suspenders` therefore still honors the root ignores.

Keep two binaries straight, because mixing them up is the usual reason a change "doesn't take":

- `./suspenders` — what `make build` produces. Use it while iterating.
- `~/code/bin/suspenders` — what `make install` puts on your `$PATH`. The installed git hooks call the bare name `suspenders hook run <event>`, so this is the copy that actually runs on commit. After a change you want your live hooks to use, run `make install`, not just `make build`.

`make install` does more than `go install`: on macOS it strips the provenance xattr and re-signs the binary ad-hoc. Tahoe SIGKILLs a linker-signed binary once it has been replaced in place, so the re-sign keeps the installed copy runnable. It is automatic; just know that is why the target has extra steps.

### Where things live

| You want to change... | File |
|---|---|
| Content detection rules | `internal/scanner/rules.go` — `DefaultRules` |
| Filename rules (flag by name, e.g. `id_rsa`, `*.p12`) | `internal/scanner/rules.go` — `DefaultFileRules` |
| How a rule is applied to a line (the gate order) | `internal/scanner/scanner.go` — `scanContent` |
| Entropy scoring | `internal/scanner/entropy.go` — `shannonEntropy` |
| Per-repo `.suspenders.yaml` handling | `internal/scanner/ignore.go` |
| Redaction of matched secrets | `internal/scanner/result.go` — `Redact` |
| The internal-reference guard | `internal/guard/guard.go` |
| Hook script generation and install/update | `internal/hook/hook.go` |
| Repo discovery for `--all` | `internal/repo/finder.go` |
| CLI commands | `cmd/suspenders/commands/` |

### Config you will touch

- Global: `~/.config/suspenders/config.yaml`. In the dotfiles setup this path is a symlink from `recipes/suspenders/config.yaml` — edit the file in the dotfiles repo, not the symlink target.
- Per-repo: `<repo>/.suspenders.yaml` (or `.suspenders.yml`). Holds rule/path/pattern ignores, a repo-scoped allowlist, watch rules, and a `guard` section (allowlist/blocked_words appended to the global guard config).

The vocabulary for ignore vs. exclude vs. allowlist vs. safe reference is defined in [`CONTEXT.md`](../CONTEXT.md). Use those terms when you write code, tests, or commit messages.

## How a content rule fires

`scanContent` runs every applicable rule over each line, in `DefaultRules` order. A regex match becomes a reported finding only if it survives this gauntlet, checked in order:

1. Inline ignore — the line carries no `suspenders:ignore` marker. If it does, the whole line is skipped before any rule runs.
2. `SkipOverlapping` — when the rule sets this, a match whose span sits inside one an earlier rule already claimed on that line is dropped. This is how the generic catch-alls at the bottom of `DefaultRules` defer to the specific rules above them.
3. `MinEntropy` — when set above 0, the secret must score at least that many bits per character. Random base64 tokens land around 5–6; English words and `xxxxxxxx`-style placeholders land near 3 or below.
4. `Filter` — when set, `Filter(secret)` must return true. `notKnownHashOrPublicKey` is the example: it vetoes subresource-integrity hashes and SSH public-key blobs that would otherwise trip `high-entropy-string`.
5. Allowlist — the raw match is not a globally or per-repo allowlisted exact value.
6. Per-repo ignore — `.suspenders.yaml` does not drop the finding by rule ID, path, or pattern.

One detail decides how you write a pattern: **capture group 1 is the secret.** If your `Pattern` has a `(...)` group, the entropy gate, the filter, and redaction all operate on that group, not the whole match. Wrap the high-entropy token in a group and leave the keyword or prefix outside it — that way `MinEntropy` scores the token, not the literal `password=` text dragging the average down.

## Add a detection rule

1. Add the rule to `DefaultRules` in `internal/scanner/rules.go`. Place specific rules above the generic catch-alls (`url-basic-auth`, `high-entropy-string`) at the end of the slice, so `SkipOverlapping` lets the broad rules defer to yours:

   ```go
   {
       ID:          "acme-api-key",
       Description: "Acme API key",
       Pattern:     regexp.MustCompile(`(acme_[A-Za-z0-9]{32})`),
       Severity:    "high", // high | medium | low
   },
   ```

2. Test the regex. Add a positive and a negative case to `TestDefaultRulesDetection` in `internal/scanner/scanner_test.go` (or `TestNewRulePatterns` in `scanner_v2_test.go`). These cases assert `rule.Pattern.MatchString` and nothing else, so they cover the regex but not the entropy or filter gates:

   ```go
   {name: "acme-api-key matches", ruleID: "acme-api-key",
    input: "acme_" + strings.Repeat("a", 32), wantMatch: true},
   {name: "acme-api-key ignores a variable name", ruleID: "acme-api-key",
    input: "var acmeKey string", wantMatch: false},
   ```

3. If the rule uses `MinEntropy`, `Filter`, or `ExcludeFiles`, the regex test is not enough — those gates only run during a real scan. Add an end-to-end case to `TestEntropyGating` in `scanner_v2_test.go`. Its `scan` helper writes content to a temp file and calls `ScanFile`, which exercises the full gate order:

   ```go
   t.Run("acme key is flagged", func(t *testing.T) {
       findings := scan(t, "config.go", `key := "acme_aB3kQ...32 real chars..."`+"\n")
       found := false
       for _, f := range findings {
           if f.Rule.ID == "acme-api-key" {
               found = true
           }
       }
       if !found {
           t.Errorf("expected acme-api-key finding, got %+v", findings)
       }
   })
   ```

4. Run it against a real sample. `suspenders scan <path>` enumerates files with `git ls-files`, so a loose file in `/tmp` is invisible — it has to be tracked or staged in a git repo. Use a throwaway repo and the staged path the hook actually takes:

   ```sh
   make build
   tmp=$(mktemp -d) && cd "$tmp" && git init -q
   printf 'token = "acme_aB3kQ...32 real chars..."\n' > leak.txt
   git add leak.txt
   ~/code/bin/suspenders scan --staged   # reads the index, same as pre-commit
   ```

   For a quick check without staging, commit the file and run `suspenders scan` (no flag), which scans tracked files via `ScanDir`.

5. Update the docs. Add the rule to the right table in [README → Detection rules](../README.md#detection-rules), and bump the rule count in three places if it changed: the README intro and Features list, and `CLAUDE.md`'s opening line. The counts are stated as prose, so they do not update themselves.

## Tune an existing rule

Same loop, narrower edit. Change the `Pattern`, `Severity`, `MinEntropy`, or `Filter` on the entry in `DefaultRules`, then re-run its cases in `TestDefaultRulesDetection` and `TestEntropyGating`. If you loosen a pattern, add a negative case that pins the thing it must still not match; if you tighten it, add a positive case for the real token it must still catch. The test is the contract — write the case that would have failed before your change.

## Debug a false positive

A false positive is a finding on something that is not a secret. Walk it down in this order:

1. Reproduce it in isolation. Copy the offending line into a temp file and scan it, so you see the rule ID and severity without the noise of a full repo:

   ```sh
   printf '%s\n' 'the exact line from the diff' > /tmp/fp.txt
   cd "$(mktemp -d)" && git init -q && cp /tmp/fp.txt . && git add fp.txt
   ~/code/bin/suspenders scan --staged
   ```

   The output names the rule: `[severity] Description (rule-id)`.

2. Pick the right fix for the blast radius:
   - One line, one repo → add `// suspenders:ignore` to the line.
   - A known-safe value that recurs → add an `allowlist` entry (global config or `.suspenders.yaml`), optionally path-scoped.
   - A whole file class (tests, fixtures) → add a `paths` ignore to the repo's `.suspenders.yaml`.
   - A rule that is wrong for this repo but fine elsewhere → add a `rules` ignore in `.suspenders.yaml` (the rule still runs, its findings are dropped here).
   - A rule that should never run anywhere → `scan.exclude_rules` in the global config removes it entirely.

3. If the rule itself is too broad, fix the pattern. Common causes: the capture group is missing so entropy scores the wrong span; `MinEntropy` is too low for a generic assignment rule; the rule needs a `Filter` to veto a known non-secret shape (follow `notKnownHashOrPublicKey`); or it needs an `ExcludeFiles` entry for a file full of high-entropy hashes (lockfiles already on `high-entropy-string`). Pin the fix with a negative test case before you commit.

## Debug a false negative

A false negative is a real secret the scan missed. The gate order is the checklist — the match died at one of these steps:

1. Is the file even scanned? `ScanDir` and `ScanStaged` only see git-tracked or staged files. Binary files (a null byte in the first 512 bytes) and files over 1 MB are skipped. A secret in an untracked file will not be caught until it is staged.
2. Does the regex match at all? Test it directly: `grep -nE 'your-pattern' file`, or add a `wantMatch: true` case and run `go test ./internal/scanner -run TestDefaultRulesDetection`. If the regex misses, the rest is moot.
3. Did a gate drop it? In `scanContent` order: an `ExcludeFiles` glob removed the rule for this filename; `SkipOverlapping` deferred to another rule that then got filtered; `MinEntropy` rejected the token as too low-entropy; a `Filter` vetoed it; or an allowlist / `.suspenders.yaml` ignore is suppressing it. Add a print or a focused test through `ScanFile` to see which.
4. Is it a brand-new secret shape? If no rule covers it, that is an "add a detection rule" task, not a bug. See above.

## Working on the hooks

The hook scripts are thin dispatchers that call `suspenders hook run <event>`; the real logic is in `internal/hook/hook.go` (generation, checksum, install/update/uninstall) and the per-event order is in the `hook run` command. To test hook changes without touching your real repos:

```sh
make install                         # the hook calls the installed binary, so install first
cd "$(mktemp -d)" && git init -q
suspenders hook install              # write the hook into this throwaway repo
suspenders hook status               # installed | outdated | foreign | not-installed
git commit --allow-empty -m test     # exercise the pre-commit path
```

The Docker integration suite covers install, chaining a foreign hook, blocking a secret, the csl takeover, update, and uninstall. Run it with `make test-integration` (each scenario is a standalone script under `tests/integration/`). Reach for it when a change touches hook generation, the backup/chain logic, or the csl-takeover path, since those are hard to exercise from a unit test.

## Before you push

```sh
make test               # unit tests
make lint               # golangci-lint
make test-integration   # Docker; run when hook behaviour changed
```

`make test` and `make lint` are the floor for any change. Add `make test-integration` when you touched hook generation or install. This repo's own `.suspenders.yaml` suppresses findings from its fixture-bearing files (the tests, `rules.go`, the README) — keep those fixtures real example tokens, do not swap them for dummies, or the rule tests lose their teeth.
