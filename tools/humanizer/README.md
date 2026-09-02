# humanizer

`humanizer` shells out to [vale](https://vale.sh) against a bundled Humanizer style pack to flag AI-writing patterns in text. It also computes quantitative voice profiles (sentence length distribution, punctuation densities, contraction rate, Flesch reading ease) and can diff two samples metric by metric. Both the CLI and the MCP server are in this binary.

It also lints and fixes the invisible watermark carriers that get embedded in text: zero-width and format Unicode, bidi overrides, tag characters, variation selectors, and exotic space homoglyphs. `lint` reports them, `fix` scrubs them, and `rewrite` builds a prompt for the statistical (token-sampling) marks a scrub can't reach. This half is ported from [watermarks-remover](https://github.com/guillaumemeyer/watermarks-remover) (MIT, ported at `28eca2d`).

## How it works

Detection is purely deterministic. The tool flags patterns; rewriting is left to the calling agent. Two independent paths cover different tells:

- Vale span rules: the embedded style pack matches specific phrasing and reports a line/column span.
- Statistical detector: whole-sample checks (sentence-length uniformity, contraction rate, type-token ratio, short-text em-dash, long-text em-dash density, semicolon absence, heading density, anaphora) catch structural tells that no single sentence exhibits, each gated on a minimum sample size.

Run both for full coverage.

The watermark side (`lint`, `fix`, `rewrite`) is separate from the AI-writing detection above. `lint` and `fix` are deterministic and offline: they classify each code point against fixed tables and strip or normalize the carriers, preserving load-bearing invisibles. `rewrite` handles statistical marks a scrub can't reach. By default it only builds the prompt, so it too is offline; the model backends are opt-in and CLI-only.

### Style pack

The Humanizer style pack ships embedded in the binary under `internal/rules/vale/styles/Humanizer/`. It contains 52 rules covering patterns from Wikipedia's "Signs of AI writing" plus the fiction-prose tells surfaced by StoryScope ([arXiv:2604.03136](https://arxiv.org/abs/2604.03136)):

AIVocabulary, AphoristicClosure, AssistantArtifacts, BoldOveruse, ChatGPTArtifacts, CitationArtifacts, ClosingRitualPhrases, CollaborativeArtifacts, ContractionAvoidance, CopulaAvoidance, CurlyQuotes, DashSubstitute, EmbodiedEmotionCliche, EmDashOveruse, EmojiDecoration, ExcessiveHedging, FalseBothSidesHedge, FalseConcession, FalseRanges, FalseVulnerability, FillerBoilerplate, FillerPhrases, FiveParagraphStructure, FormulaicChallenges, FragmentedHeader, GenericConclusion, HashtagStuffing, HyphenatedPairOveruse, InfomercialHooks, InlineHeaderList, KnowledgeCutoff, LetsConstructions, NarratorMoralizing, NegativeParallelism, NotabilityInflation, ParticipialTailExtended, PassiveVoice, PersuasiveAuthority, PromotionalVocab, QuietAcceptanceEnding, RhetoricalTransitions, RuleOfThree, SignificanceInflation, Signposting, StockSensoryImagery, SuperficialIng, Sycophancy, TailingNegation, TitleCaseHeadings, UnfilledPlaceholders, UTMParameters, VagueAttribution

Rule metadata (ID, category, severity, rationale, before/after examples) comes from `# humanizer-*` comment headers in each YAML file, served by both the CLI (`rules explain`) and the MCP tools.

## Install

```sh
make build && make install
```

The binary installs to `~/code/bin/humanizer`. `make install` also applies an adhoc codesign signature, which macOS 15+ requires for stdio MCP binaries.

To re-sign after a manual copy:

```sh
make resign BIN=~/code/bin/humanizer
```

### Dependencies

`vale` must be on `$PATH`. Install it with:

```sh
brew install vale
```

The Humanizer style pack is embedded in the binary and extracted on first use to `~/.cache/humanizer/vale`. No separate `vale` config or style download is needed.

## Usage

### detect

Scan text for AI-writing patterns.

```sh
humanizer detect [file]
humanizer detect README.md
cat draft.md | humanizer detect
```

Reads from stdin when you omit the file argument or pass `-`.

| Flag | Description |
|---|---|
| `--min-severity` | Filter findings below this level: `suggestion`, `warning`, `error` |
| `--rule` | Restrict to this rule ID (repeatable, e.g. `--rule Humanizer.AIVocabulary`) |
| `--statistical` | Run statistical checks instead of vale span rules (sentence uniformity, contraction rate, TTR) |
| `--json` | Emit findings as JSON (same shape as the `humanizer_detect` MCP tool) |

Output groups findings by file position with a severity badge (`ERR`, `WARN`, `INFO`) and a summary line with counts per level.

Statistical mode (`--statistical`) runs size-gated whole-sample checks (sentence-length uniformity, contraction rate, lexical diversity, anaphora, heading density) rather than span-level pattern matching. Run both modes for full coverage; very short snippets return few or no statistical findings.

### judge

Send text to an LLM for a whole-passage AI/human verdict. This is the fuzzy complement to `detect`: span rules and statistics catch mechanical tells, while the judge reads the passage the way a human reader does.

```sh
humanizer judge draft.md
cat draft.md | humanizer judge --json
```

The backend comes from the environment. `HUMANIZER_BACKEND` forces one; otherwise the first configured provider wins. `LITELLM_BASE_URL` selects a LiteLLM proxy, self-hosted or a company gateway, on model `claude-haiku-4-5-20251001`, with `LITELLM_API_KEY` sent as a bearer token when set. Otherwise `OPENROUTER_API_KEY` selects OpenRouter on model `anthropic/claude-haiku-4.5`. Both make the same OpenAI-compatible chat-completions call; only the host, the auth, and the model alias differ, so a proxy that names Haiku 4.5 differently needs `HUMANIZER_MODEL`. Without a provider the command fails, and everything else in humanizer keeps working offline.

| Flag | Description |
|---|---|
| `--backend` | Force an LLM backend (`litellm`, `openrouter`); default auto-detects from env |
| `--model` | Override the backend's default model id (also settable via `HUMANIZER_MODEL`) |
| `--json` | Emit the verdict as JSON (same shape as the `humanizer_judge` MCP tool) |

The verdict is `likely_ai`, `likely_human`, or `mixed`, with a confidence score, concrete signals, and a one-line summary. Confidence is a 0-1 fraction reporting how sure the judge is of the verdict it gave, not how machine-written the text is: a terse changelog can be confidently human at 0.5 because the genre explains everything the judge could point at. The rubric makes the model derive that number from the signals it listed, so short or genre-bound passages score lower than long ones full of quotable evidence. Verdicts are advisory: treat flagged sections as rewrite targets alongside detect findings, not as ground truth.

### scan

Pull the prose out of Go source and scan it. Doc comments, cobra command fields, MCP tool descriptions, error messages, and `jsonschema` struct tags are text a reader meets in help output, tool listings, and errors, and none of it sits in a Markdown file where `detect` would find it.

```sh
humanizer scan --go internal/cli/detect.go
humanizer scan --go ./services/ | humanizer detect
humanizer scan --go --detect ./services/wire/
```

| Flag | Description |
|---|---|
| `--go` | Extract from Go source (the only extractor so far) |
| `--detect` | Run the vale span rules over the extracted prose |
| `--holistic` | Send each file's prose to the LLM judge; skipped with a note when no backend is configured |
| `--kind` | Restrict extraction to one kind: `doc`, `cobra`, `mcp`, `error`, `schema` (repeatable) |

Without `--detect` or `--holistic` the command prints the blocks, each behind a `file:line` header, so the output pipes into `humanizer detect`. A directory argument is walked recursively: dot-directories, `vendor`, `node_modules`, and `testdata` are skipped, as are generated files (a `.pb.go` name or a `DO NOT EDIT.` header), while test files are read like any other source. Under `--detect`, vale runs once per file over that file's blocks and every finding is reported against the line of Go source it came from.

### profile

Compute a quantitative voice profile of a text sample.

```sh
humanizer profile draft.md
humanizer profile draft.md --diff reference.md
cat draft.md | humanizer profile --json
```

Metrics: word/sentence/paragraph counts, type-token ratio (TTR), sentence length (mean, stddev, p50, p90), punctuation densities per 100 words (em-dash, semicolon, colon, parenthesis, comma, hyphenated-pair, bold), contraction rate, Flesch reading ease, top bigrams, top trigrams.

| Flag | Description |
|---|---|
| `--diff` | Path to a sample file; also print the metric-by-metric delta |
| `--json` | Emit the profile (and diff, if requested) as JSON |

### rules

List or explain the bundled detection rules.

```sh
# list all rules
humanizer rules list

# filter by category
humanizer rules list --category language

# show full metadata for one rule
humanizer rules explain Humanizer.EmDashOveruse
```

`rules list` flags:

| Flag | Description |
|---|---|
| `--category` | Filter by category: `content`, `language`, `style`, `communication` |
| `--json` | Emit the rule list as JSON |

`rules explain` prints the rule ID, name, category, severity, summary, rationale, before/after examples, and reference link.

### narrative

For fiction and story-shaped prose only; skip it for docs, PR descriptions, and Slack drafts. Print the StoryScope narrative rubric: 30 discourse-level features (thematic over-explanation, plot linearity, embodied emotion, intertextual reference) that separate human-written from AI-generated fiction; in the paper this narrative signal also survives span-level style editing. These features need a reader's judgment, not a regex, so the command only serves the rubric. Score a passage by running the judge prompt with the passage on stdin:

```sh
# human-readable rubric, grouped by theme
humanizer narrative

# score a story: judge prompt as the instruction, passage on stdin
cat story.md | claude -p --model sonnet "$(humanizer narrative --prompt)"
```

The judge prompt ends with an output contract (a final `VERDICT|confidence|features` line) and works best on a complete story or a 1,000+ word section.

| Flag | Description |
|---|---|
| `--prompt` | Print only the judge prompt |
| `--json` | Emit features, themes, prompt, and source as JSON (same shape as the `humanizer_narrative_rubric` MCP tool) |

### lint

Report invisible-Unicode and space-homoglyph watermark carriers. Reports only; it changes nothing.

```sh
humanizer lint draft.md
cat draft.md | humanizer lint --json
```

| Flag | Description |
|---|---|
| `--aggressive` | Also flag Cyrillic/fullwidth Latin confusable lookalikes |
| `--strip-emoji-glue` | Paranoid: also flag load-bearing invisibles (emoji glue, script joiners, flag tags, orthographic Cf) |
| `--json` | Emit the report as JSON |

Each hit reports a codepoint, kind (`strip`, `bidi`, `tag_chars`, `variation_selector`, `zwj_family`, `space`, `confusable`, `other_cf`), a confidence (`probable` for edit-carriers, `informational` for spaces), a count, and sample character offsets. Load-bearing invisibles (emoji ZWJ/variation selectors after an emoji base, script joiners inside complex scripts, flag tag characters, and orthographic Arabic/Syriac marks) are preserved and not flagged unless `--strip-emoji-glue` is set. Exits non-zero when any carrier is found.

### fix

Apply the scrub and emit the cleaned text.

```sh
humanizer fix draft.md               # cleaned text to stdout, stats to stderr
humanizer fix draft.md -o clean.md
humanizer fix draft.md --in-place    # overwrite, writing draft.md.bak first
```

| Flag | Description |
|---|---|
| `-o`, `--output` | Write cleaned text here (default: stdout) |
| `--in-place` | Overwrite the input file (writes a `.bak` backup first) |
| `--no-normalize-spaces` | Do not rewrite exotic spaces to U+0020 |
| `--aggressive-homoglyphs` | Risky: map Cyrillic/fullwidth Latin confusables to ASCII |
| `--nfkc` | Risky: apply Unicode NFKC normalization after the scrub |
| `--strip-emoji-glue` | Risky: strip load-bearing invisibles too |
| `--json` | Emit the stats summary as JSON on stderr |

The default is non-intrusive: it strips invisible/format controls and normalizes exotic spaces, neither of which changes a visible character. The three risky flags rewrite visible content, so they are opt-in.

### rewrite

Build (or run) a rewrite prompt for statistical watermarks, which the deterministic scrub can't touch.

```sh
# offline default: print the prompt for you (or an agent) to run
humanizer rewrite draft.md --strength humanize

# run a local Ollama model directly
export WATERMARKS_REWRITE_MODEL=llama3.2
humanizer rewrite draft.md --backend ollama -o draft.rewritten.md
```

| Flag | Description |
|---|---|
| `--backend` | `print-prompt` (default, offline), `ollama`, or `openai-compatible` |
| `--strength` | `paraphrase` (default), `humanize`, `code`, `backtranslate`, `structural` |
| `--model` | Model name (required for the model backends) |
| `--base-url` | Backend base URL (default `http://127.0.0.1:11434`) |
| `--allow-remote` | Allow non-loopback backend hosts (default: deny) |
| `--candidates` | Generate N candidates and keep the most lexically diverged |
| `--temperature` | Sampling temperature (default 0.9) |
| `--no-layer-a-after` | Skip the Layer A scrub on model output |
| `-o`, `--output` | Write the result here (default: stdout) |
| `--json-stats` | Emit the info block as JSON on stderr |

The default `print-prompt` backend calls no model; it returns the prompt so you or the calling agent produce the rewrite. The `ollama` and `openai-compatible` backends send text off-process: non-loopback hosts are refused unless `--allow-remote` (or `WATERMARKS_REWRITE_ALLOW_REMOTE=1`) is set, redirects are refused so the API-key header can't be forwarded to an unvalidated host, and the key is read from `WATERMARKS_REWRITE_API_KEY` only, never a flag. Prefer a rewrite model different from the suspected origin; rewriting with the origin model can re-stamp the text.

### version

Report which build is installed.

```sh
humanizer version              # bare version token
humanizer version -o json      # version, commit, tag, build_time
```

Plain output is the version and nothing else, so a probe can read the line as-is. The JSON form is the four-key build metadata object, with each key present and `""` for anything the build didn't stamp.

## MCP

```sh
humanizer mcp
```

An MCP host such as Claude Code launches this; don't run it by hand in normal use. On a standalone install, register it once:

```sh
claude mcp add --scope user humanizer -- humanizer mcp
```

On a ralph-managed machine, skip the manual command: MCP registration is machine-private wiring that ships from the consuming repo's companion recipe (`docs/adr/0006` at the repo root).

The server uses the [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) and communicates over stdio. It exposes fourteen tools:

| Tool | Description |
|---|---|
| `humanizer_status` | Health check: vale installed, version, cache dir, rule count |
| `humanizer_detect` | Scan text for AI-writing patterns (vale span rules) |
| `humanizer_detect_file` | Scan a file on disk (sandbox: prose and Go files under the profile's workspace roots only) |
| `humanizer_scan_go` | Extract prose from Go source and scan it; findings map back to Go source lines |
| `humanizer_detect_statistical` | Whole-sample statistical checks (sentence uniformity, contraction rate, TTR, anaphora) |
| `humanizer_rules_list` | List all detection rules |
| `humanizer_rules_explain` | Full metadata and examples for one rule |
| `humanizer_narrative_rubric` | StoryScope narrative rubric and judge prompt for LLM-judged fiction detection |
| `humanizer_judge` | Whole-passage LLM verdict via the configured backend (needs a key and network egress) |
| `humanizer_voice_profile` | Quantitative voice profile of a text sample |
| `humanizer_voice_diff` | Metric-by-metric delta between two samples |
| `humanizer_lint` | Report invisible-Unicode / space-homoglyph watermark carriers (offline) |
| `humanizer_fix` | Scrub those carriers; returns cleaned text + stats (offline) |
| `humanizer_rewrite` | Build a rewrite prompt for statistical watermarks (offline; network backends are CLI-only) |

The consuming repo registers the server with the MCP host; in that setup it runs under a seatbelt sandbox (see Sandbox below).

### Sandbox

The consuming repo's registration wrapper runs the MCP server under macOS `sandbox-exec` with a seatbelt profile. The sandbox applies only when the MCP host launches the server; the CLI commands (`detect`, `profile`, `rules`) run without any sandbox.

What the sandbox denies:

- Network, except outbound TLS for `humanizer_judge`'s backend call. Detection itself runs fully offline; the profile allows no listener and no plain HTTP.
- All `$HOME` reads, denied by default except specific paths: the install dir (the binaries), `~/.cache/humanizer` (the extracted style pack), and `.md`/`.markdown`/`.txt`/`.go` files under the profile's workspace roots (for `humanizer_detect_file` and `humanizer_scan_go`).
- All `$HOME` writes, restricted to `~/.cache/humanizer` and system temp.

This means `humanizer_detect_file` and `humanizer_scan_go` work only on files under the workspace roots the profile grants. For anything else (e.g., a file under `~/Desktop`), pass the content as text via `humanizer_detect` instead; `humanizer_scan_go` has no such fallback, so use the CLI's `scan --go`, which runs unsandboxed.

If you add a runtime file or network need, update the consuming repo's seatbelt profile; a code change alone isn't enough.

## Configuration

The style pack extracts to `~/.cache/humanizer/vale` on first use. To override the cache location, set `HUMANIZER_CACHE_DIR` or `XDG_CACHE_HOME`.

## Where things live

| File | What it does |
|---|---|
| `internal/rules/vale/styles/Humanizer/*.yml` | One YAML file per rule. Add or edit rules here. |
| `internal/rules/embed.go` | Embeds the `vale/` tree into the binary; extracts to `~/.cache/humanizer/vale` on first use. |
| `internal/rules/vale.go` | Runs the `vale` subprocess against extracted rules, parses JSON output. |
| `internal/rules/metadata.go` | Parses `# humanizer-*` headers from each YAML for `rules list`/`explain`. |
| `internal/goscan/` | Go source prose extraction: `Scan`/`ScanSource` return the blocks, `Render` joins one file's blocks into a document plus the line map back to source. |
| `internal/voice/profile.go` | Sentence stats, punctuation densities, contraction rate, Flesch ease. |
| `internal/voice/statistical.go` | Statistical AI-signal detectors (size-gated). |
| `internal/voice/diff.go` | Metric-by-metric diff between two profiles. |
| `internal/scrub/scrub.go` | Invisible-Unicode/homoglyph carrier tables, `Inspect` (lint) and `Clean` (fix). |
| `internal/rewrite/` | Rewrite prompt builder, candidate selection, and the hardened HTTP backends. |
| consuming repo's seatbelt profile | Sandbox for the MCP server (machine-private wiring, kept beside the MCP registration). |

Package path: `github.com/mad01/thismoon/tools/humanizer`, part of the monorepo module; it has no go.mod of its own.

## Develop

**Adding or editing a rule:**

1. Copy an existing `.yml` from `internal/rules/vale/styles/Humanizer/` as a template.
2. Edit the rule, keeping the `# humanizer-*` comment block intact (used by `rules explain`).
3. Run the tests: the metadata tests parse every YAML and catch malformed headers.
4. Check that `vale` picks it up: `humanizer detect --rule Humanizer.YourRule testdata/ai_sample.md`
5. Build and install: `make build && make install`

**Change-and-see loop:**

```sh
# Edit a rule YAML, then:
make test
humanizer detect testdata/ai_sample.md
humanizer detect --statistical testdata/ai_sample.md
humanizer rules list

# For MCP changes, also install and verify:
make install
claude mcp list
```

**Tests:**

```sh
make test
# or: go test ./...
```

The test suite includes metadata validation for every YAML header and concurrent-detection race checks.

## Docs

- [architecture](architecture.md): internal structure and data flow
- [operating](operating.md): runtime behavior, failure modes, first moves
- [why](why.md): why this component exists
- [config](config.md): configuration reference
