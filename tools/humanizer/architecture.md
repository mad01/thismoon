# humanizer architecture

## Overview

humanizer is a tool: one Go program installed to `~/code/bin/humanizer` that
detects AI-writing patterns in text and computes quantitative voice
profiles. At runtime it is a short-lived CLI (`detect`, `profile`, `rules`)
or an MCP stdio server (`humanizer mcp`) launched by an MCP host. For span
detection the tool shells out to a `vale` subprocess against an embedded
style pack rather than matching patterns itself. Everything runs offline;
the statistical detector and voice profiler are pure Go with no subprocess
at all.

## Structure

```
cmd/humanizer/       entrypoint; delegates to internal/cli
internal/cli/        cobra command tree: root, detect, profile, rules,
                     narrative, lint, fix, rewrite, mcp, docs, version
internal/rules/      vale integration: embed.go (pack embedding + cache
                     extraction), vale.go (subprocess run + JSON parsing),
                     metadata.go (rule metadata from YAML headers),
                     vale/styles/Humanizer/*.yml (52 rules)
internal/narrative/  StoryScope narrative rubric: 30 discourse-level fiction
                     features plus a rendered LLM judge prompt — static data,
                     no detection engine
internal/voice/      profile.go (Compute), statistical.go
                     (DetectStatistical), diff.go (DiffProfiles)
internal/scrub/      Layer A watermark scrub: carrier tables, Inspect (lint),
                     Clean (fix) — deterministic, offline
internal/rewrite/    Layer B watermark rewrite: prompt.go (BuildPrompt),
                     select.go (candidate scoring), backend.go (hardened
                     ollama/openai HTTP), rewrite.go (orchestration)
internal/mcpserver/  server.go plus one tools_*.go file per tool group
testdata/            ai_sample.md / human_sample.md fixtures
```

`internal/rules`, `internal/voice`, `internal/scrub`, and `internal/rewrite`
hold all the logic; `internal/cli` and `internal/mcpserver` are thin frontends
over the same functions, so CLI and MCP results never diverge.

The watermark half (`internal/scrub`, `internal/rewrite`) is a Go port of
watermarks-remover's text scripts (MIT, ported at `28eca2d`). `scrub`
classifies each rune against fixed carrier tables — one classifier drives both
`Inspect` (the lint report) and `Clean` (the fix), so they never disagree — and
is fully deterministic and offline. `rewrite` targets statistical marks the
scrub can't reach: its default backend only builds the prompt (offline), while
the `ollama`/`openai-compatible` backends make network calls and are therefore
CLI-only under the MCP seatbelt.

## Data flow

Span detection (`detect` / `humanizer_detect`): the text lands in
`rules.Detect`, which calls `EnsurePack` to extract the embedded style pack
to the cache directory if needed, writes the text to a temp file, and runs
`vale --output=JSON --config=<cache>/.vale.ini` via `runVale`.
`parseValeJSON` turns vale alerts into findings; `filterAndEnrich` applies
the `--min-severity` and `--rule` filters and attaches rule metadata.
`humanizer_detect_file` takes the same path through `rules.DetectFile` but
hands vale the on-disk file, so vale sees the real extension.

Statistical detection (`detect --statistical` /
`humanizer_detect_statistical`): `voice.DetectStatistical` runs whole-sample
checks (sentence-length uniformity, contraction rate, type-token ratio,
em-dash density, heading density, anaphora), each gated on a minimum sample
size. No vale, no
spans — findings describe the sample as a whole.

Profiling: `voice.Compute` tokenizes the text and produces the metric set
(counts, sentence-length distribution, punctuation densities, contraction
rate, Flesch reading ease, top n-grams); `voice.DiffProfiles` computes the
metric-by-metric delta for `profile --diff` and `humanizer_voice_diff`.

Rule metadata: `metadata.go` parses the `# humanizer-*` comment headers of
every embedded YAML once and serves them to `rules list` / `rules explain`
and the matching MCP tools.

Narrative rubric (`narrative` / `humanizer_narrative_rubric`): the
`narrative` package holds the 30 StoryScope core features as static data and
renders them into a judge prompt. Nothing here scores text — the features
need a reader's judgment, so the surfaces only serve the rubric and the
calling agent runs the LLM pass.

## Storage

The embedded style pack extracts to `~/.cache/humanizer/vale` on first use
(`EnsurePack`); override the location with `HUMANIZER_CACHE_DIR` or
`XDG_CACHE_HOME`. That cache is the only thing humanizer writes. Everything
else it reads (input files, temp files for vale) is transient.

## Interfaces

CLI: `detect [file]` (flags `--min-severity`, `--rule`, `--statistical`,
`--json`), `profile [file]` (`--diff`, `--json`), `rules list`
(`--category`, `--json`), `rules explain <rule_id>`, `narrative`
(`--prompt`, `--json`), `lint`, `fix`, `rewrite`, `docs`, `mcp`, and
`version [-o json]` (the bare version token, or the four-key build
metadata object shared across the repo's components). All text commands
read stdin when the file argument is omitted or `-`.

MCP: `humanizer mcp` starts a stdio server (MCP Go SDK) exposing twelve
tools: `humanizer_status`, `humanizer_detect`, `humanizer_detect_file`,
`humanizer_detect_statistical`, `humanizer_rules_list`,
`humanizer_rules_explain`, `humanizer_narrative_rubric`,
`humanizer_voice_profile`, `humanizer_voice_diff`, `humanizer_lint`,
`humanizer_fix`, `humanizer_rewrite`. Handlers call the same internal
functions as the CLI. When the consuming repo registers the server it runs under a
seatbelt sandbox: no network, and `humanizer_detect_file` reads only prose
files under the profile's workspace roots — the sandbox is the consuming
repo's wiring (docs/adr/0006), not this code.

Runtime dependency: `vale` must be on `$PATH` for span detection;
`humanizer_status` reports whether it is installed. The statistical, voice,
and rules-metadata paths work without it.
