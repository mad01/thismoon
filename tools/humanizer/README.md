# humanizer

`humanizer` shells out to [vale](https://vale.sh) against a bundled Humanizer style pack to flag AI-writing patterns in text. It also computes quantitative voice profiles (sentence length distribution, punctuation densities, contraction rate, Flesch reading ease) and can diff two samples metric by metric. Both the CLI and the MCP server are in this binary.

## How it works

Detection is purely deterministic. The tool flags patterns; rewriting is left to the calling agent. Two independent paths cover different tells:

- Vale span rules: the embedded style pack matches specific phrasing and reports a line/column span.
- Statistical detector: whole-sample checks (sentence-length uniformity, contraction rate, type-token ratio, short-text em-dash, semicolon absence, heading density, anaphora) catch structural tells that no single sentence exhibits, each gated on a minimum sample size.

Run both for full coverage.

### Style pack

The Humanizer style pack ships embedded in the binary under `internal/rules/vale/styles/Humanizer/`. It contains 43 rules covering patterns from Wikipedia's "Signs of AI writing":

AIVocabulary, AphoristicClosure, BoldOveruse, ClosingRitualPhrases, CollaborativeArtifacts, ContractionAvoidance, CopulaAvoidance, CurlyQuotes, EmDashOveruse, EmojiDecoration, ExcessiveHedging, FalseBothSidesHedge, FalseConcession, FalseRanges, FalseVulnerability, FillerBoilerplate, FillerPhrases, FiveParagraphStructure, FormulaicChallenges, FragmentedHeader, GenericConclusion, HashtagStuffing, HyphenatedPairOveruse, InfomercialHooks, InlineHeaderList, KnowledgeCutoff, LetsConstructions, NegativeParallelism, NotabilityInflation, ParticipialTailExtended, PassiveVoice, PersuasiveAuthority, PromotionalVocab, RhetoricalTransitions, RuleOfThree, SignificanceInflation, Signposting, SuperficialIng, Sycophancy, TailingNegation, TitleCaseHeadings, UnfilledPlaceholders, VagueAttribution

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

**Statistical mode** (`--statistical`) runs size-gated whole-sample checks (sentence-length uniformity, contraction rate, lexical diversity, anaphora, heading density) rather than span-level pattern matching. Run both modes for full coverage; very short snippets return few or no statistical findings.

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

**rules list** flags:

| Flag | Description |
|---|---|
| `--category` | Filter by category: `content`, `language`, `style`, `communication` |
| `--json` | Emit the rule list as JSON |

`rules explain` prints the rule ID, name, category, severity, summary, rationale, before/after examples, and reference link.

## MCP

```sh
humanizer mcp
```

An MCP host such as Claude Code launches this; don't run it by hand in normal use. To register it once:

```sh
claude mcp add --scope user humanizer -- humanizer mcp
```

The server uses the [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) and communicates over stdio. It exposes eight tools:

| Tool | Description |
|---|---|
| `humanizer_status` | Health check: vale installed, version, cache dir, rule count |
| `humanizer_detect` | Scan text for AI-writing patterns (vale span rules) |
| `humanizer_detect_file` | Scan a file on disk (sandbox: prose files under the profile's workspace roots only) |
| `humanizer_detect_statistical` | Whole-sample statistical checks (sentence uniformity, contraction rate, TTR, anaphora) |
| `humanizer_rules_list` | List all detection rules |
| `humanizer_rules_explain` | Full metadata and examples for one rule |
| `humanizer_voice_profile` | Quantitative voice profile of a text sample |
| `humanizer_voice_diff` | Metric-by-metric delta between two samples |

The consuming repo registers the server with the MCP host; in that setup it runs under a seatbelt sandbox (see Sandbox below).

### Sandbox

The consuming repo's registration wrapper runs the MCP server under macOS `sandbox-exec` with a seatbelt profile. The sandbox applies only when the MCP host launches the server; the CLI commands (`detect`, `profile`, `rules`) run without any sandbox.

What the sandbox denies:

- All network: vale runs fully offline, so no traffic is needed.
- All `$HOME` reads, denied by default except specific paths: the install dir (the binaries), `~/.cache/humanizer` (the extracted style pack), and `.md`/`.markdown`/`.txt` files under the profile's workspace roots (for `humanizer_detect_file`).
- All `$HOME` writes, restricted to `~/.cache/humanizer` and system temp.

This means `humanizer_detect_file` works only on prose files under the workspace roots the profile grants. For anything else (e.g., a file under `~/Desktop`), pass the content as text via `humanizer_detect` instead.

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
| `internal/voice/profile.go` | Sentence stats, punctuation densities, contraction rate, Flesch ease. |
| `internal/voice/statistical.go` | Statistical AI-signal detectors (size-gated). |
| `internal/voice/diff.go` | Metric-by-metric diff between two profiles. |
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
