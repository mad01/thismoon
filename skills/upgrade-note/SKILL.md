---
name: upgrade-note
description: Write or check an upgrade note for the thismoon repo, the entry in docs/UPGRADING.md that tells a user what to run on a machine that already has the tools. Use when a thismoon change needs an import, a hook entry, a state move, a leftover removed or an ordered ralph up; when asked "does this need an upgrade note"; or when reviewing an entry someone else wrote.
---

# Upgrade notes

An upgrade note tells a user what to do on a machine that already runs the
tools. `ralph up` pulls main and rebuilds, but it cannot run an import, add a
hook entry to a private settings template, or order two merges. This skill
turns one change into one entry, the same way every time: fill in the input,
decide whether an entry is needed, write the output, then check it.

## 1. Fill in the input

Collect these facts from the diff and the pull request before writing a word
of the entry. A field you cannot fill is a question to answer, not a blank to
skip.

```yaml
date: 2026-11-14           # the day the change merges
components: [srtspec, speak]   # every component or recipe the user sees change
pr: 500                    # pull request number
title: the srt-run launcher path moves to the srtspec recipe
change: >                  # what is different on the machine, and what is not
  The srt-run shim moves from the speak recipe to the srtspec recipe.
  Its path and command line are unchanged.
who: everyone              # or the condition that makes the step needed
when: any time             # order constraint, such as "before the first ralph up"
steps:                     # what a ralph user runs, in order
  - run: ralph up
  - run: delete ~/code/bin/srt-run.bak.<timestamp>
    note: the backup link points at a file that is gone
steps_without_ralph: []    # only when they differ from steps
skipped: >                 # what happens on a machine that misses the step
  a dangling backup link stays in ~/code/bin
verify:
  command: readlink ~/code/bin/srt-run
  expect: ends in recipes/srtspec/srt-run.sh
  passes_when_already_done: true     # must be true; rewrite the check if not
since: {srtspec: main only}          # per component: unreleased | main only
detectable: false          # can a doctor check see a machine that missed the step?
```

`since` at merge time is `unreleased` for a component whose release will carry
the change, and `main only` when no component release carries it, as with a
change to a recipe. A version goes in later, once the release exists
(`docs/RELEASING.md`, "Upgrade notes").

## 2. Decide whether an entry is needed

An entry is needed when a machine that only runs `ralph up`, or only installs
the new release, ends up broken, different, or carrying leftovers:

| The change | Entry |
|---|---|
| Needs a command run once: an import, a forced rebuild, a cleanup run | yes |
| Needs a hook, MCP or settings entry added, moved or removed in the consuming repo | yes |
| Moves state, or needs state cleared before the apply | yes |
| Leaves a link, an agent or a registration behind | yes |
| Needs two merges or two applies in a fixed order | yes |
| Changes output or a default that scripts or settings depend on | yes |
| Needs only a new agent session or a service restart ralph already does | no |
| Adds a feature with a safe default | no |
| Changes docs, tests or internals only | no |

When the answer is no, stop and report `needed: false` with the reason.

## 3. Write the output

The entry goes at the top of the entries in `docs/UPGRADING.md`, newest first:

```markdown
## <date> <components, comma-separated>: <title>

Change: thismoon #<pr>. <change>

Run: <who>. <steps> <skipped>

Verify: `<verify.command>` <verify.expect>.

Since: <component> unreleased.
```

When `steps_without_ralph` is set, `Run:` has two parts, "With ralph:" and
"Without ralph:".

Rules the entry must meet:

1. The heading reads `## YYYY-MM-DD components: title`, and no entry below it
   has a later date.
2. `Change:`, `Run:`, `Verify:` and `Since:` each start a line, appear once,
   and come in that order.
3. `Change:` names the pull request as `#<number>`.
4. `Since:` holds comma-separated items, each `unreleased`, `main only`, a
   component with a state (`speak unreleased`) or a component with a version
   (`belt 2.12.0`).
5. `Run:` says who needs the step when it is not everyone, and gives commands
   to paste, not a description of them.
6. `Verify:` is a command plus the output to look for, and it also passes on a
   machine where the step was already done.
7. The entry names this repo and the consuming repo's settings template only.
   Machine-private wiring stays out (`docs/adr/0006`).
8. No em dashes, short sentences, the same word for the same thing.

The root `CLAUDE.md` asks for two more things in the same pull request: the
same steps in the pull request body, and a doctor check when a missed step is
detectable.

Then report the result in this shape:

```yaml
needed: true
reason: the backup link is left behind and a consuming hook may name the old build
file: docs/UPGRADING.md
entry: |
  ## 2026-11-14 srtspec, speak: the srt-run launcher path moves to the srtspec recipe
  ...
pr_body: the same steps, in one or two lines
doctor_check: exists | added | not detectable
checked: go test ./kit/upgradenotes/ passes
```

## 4. Check it

The form is checked by a test that reads the real page:

```bash
go test ./kit/upgradenotes/
```

It fails with the line and the rule when an entry breaks rules 1 to 4. CI runs
it on every pull request. Rules 5 to 8 need a reader: run the `Run` and
`Verify` lines on a machine or in a scratch home, and confirm `Verify` fails
before the step and passes after it.

`kit/upgradenotes` holds the data structure the test uses: `Parse` returns the
entries (`Date`, `Components`, `Title`, `Change`, `Run`, `Verify`, `Since`,
`Line`), and `Check` returns the problems. Entries dated before the form
(`FormDate`) keep their shape and are checked only for a dated heading, a
`Change` with a pull request, and a `Verify`.

## After a release

When a component is released, replace `unreleased` with the component and its
version in every entry that names it, in a `docs:` pull request. The same test
checks the new value.

## Reviewing an entry someone else wrote

Rebuild the input from the entry. Every field in step 1 must be answerable
from the entry text alone. A field you cannot fill is the review finding:
"the entry does not say who needs this", "Verify gives no expected output".
