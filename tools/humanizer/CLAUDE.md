# humanizer, CLI and MCP server for AI-writing detection and voice profiling

Go CLI + MCP server. Detects AI-writing patterns by shelling out to `vale` against an embedded Humanizer style pack, produces a quantitative voice profile, and exposes both as MCP tools for use inside Claude Code.

## Module layout

```
humanizer/
  cmd/humanizer/     - entrypoint (delegates to internal/cli)
  internal/
    cli/             - cobra command tree (root, detect, judge, scan, profile, rules, narrative, lint, fix, rewrite, mcp, docs, version); build metadata from the shared buildinfo package
    goscan/          - Go source prose extraction (goscan.go walk + file filters, extract.go go/ast visitors, document.go blocks-to-document plus the line map back to source); importable so an MCP tool can wrap the same extraction
    mcpserver/       - MCP server wiring (server.go, tools_detect.go, tools_scan.go, tools_statistical.go, tools_rules.go, tools_narrative.go, tools_judge.go, tools_status.go, tools_voice.go, tools_scrub.go, tools_rewrite.go)
    rules/           - vale style pack embedding and metadata (embed.go, metadata.go, vale.go, vale/)
    narrative/       - StoryScope narrative rubric: 30 discourse-level fiction features + judge prompt (static data, no detection engine)
    backend/         - LLM provider behind the judge pass (MAD-342): env-driven selection (backend.go) + OpenRouter client (openrouter.go); vertex/anthropic planned
    judge/           - holistic whole-passage judgment: static system prompt with
                       the confidence rubric, JSON verdict contract, one-retry
                       parse (judge.go)
    voice/           - voice profiler, statistical detector, and diff logic (profile.go, statistical.go, diff.go)
    scrub/           - Layer A watermark scrub: invisible-Unicode/homoglyph carrier tables, Inspect (lint) and Clean (fix)
    rewrite/         - Layer B watermark rewrite: prompt builder, candidate selection, hardened HTTP backends (ported from watermarks-remover, MIT)
  testdata/          - fixture files for tests (ai_sample.md, human_sample.md)
  Makefile           - package path github.com/mad01/thismoon/tools/humanizer, part of the monorepo module; no go.mod of its own
```

## How it works

Two independent detection paths, both backed by the same text:

- Vale span rules (`humanizer_detect` / `humanizer detect`): the embedded style pack under `internal/rules/vale/styles/Humanizer/*.yml` flags a matched substring with line/column.
- Statistical detector (`humanizer_detect_statistical` / `humanizer detect --statistical`, `internal/voice/statistical.go`): flags whole-sample properties (sentence-length stddev, contraction rate, type-token ratio, short-text em-dash, long-text em-dash density, semicolon absence, heading density, anaphora) with no span, each gated on a minimum sample size.

Run both for full coverage. Span rules catch specific phrasing tells; the statistical detector catches structural ones that no single sentence exhibits.

### Style pack

The Humanizer style pack ships embedded in the binary under `internal/rules/vale/styles/Humanizer/`. It contains 52 rules covering patterns from Wikipedia's "Signs of AI writing" plus the fiction-prose tells surfaced by StoryScope (arXiv:2604.03136):

AIVocabulary, AphoristicClosure, AssistantArtifacts, BoldOveruse, ChatGPTArtifacts, CitationArtifacts, ClosingRitualPhrases, CollaborativeArtifacts, ContractionAvoidance, CopulaAvoidance, CurlyQuotes, DashSubstitute, EmbodiedEmotionCliche, EmDashOveruse, EmojiDecoration, ExcessiveHedging, FalseBothSidesHedge, FalseConcession, FalseRanges, FalseVulnerability, FillerBoilerplate, FillerPhrases, FiveParagraphStructure, FormulaicChallenges, FragmentedHeader, GenericConclusion, HashtagStuffing, HyphenatedPairOveruse, InfomercialHooks, InlineHeaderList, KnowledgeCutoff, LetsConstructions, NarratorMoralizing, NegativeParallelism, NotabilityInflation, ParticipialTailExtended, PassiveVoice, PersuasiveAuthority, PromotionalVocab, QuietAcceptanceEnding, RhetoricalTransitions, RuleOfThree, SignificanceInflation, Signposting, StockSensoryImagery, SuperficialIng, Sycophancy, TailingNegation, TitleCaseHeadings, UnfilledPlaceholders, UTMParameters, VagueAttribution

Rule metadata (ID, category, severity, rationale, before/after examples) is parsed from `# humanizer-*` comment headers in each YAML file and served by both the CLI and the MCP tools.

The pack extracts to `~/.cache/humanizer/vale` on first use. To override the cache location, set `HUMANIZER_CACHE_DIR` or `XDG_CACHE_HOME`.

### Adding or editing a rule

1. Copy an existing `.yml` from `internal/rules/vale/styles/Humanizer/` as a template.
2. Edit the rule, keeping the `# humanizer-*` comment block intact (used by `rules explain`).
3. Run the tests: the metadata tests parse every YAML and catch malformed headers.
4. Check that `vale` picks it up: `humanizer detect --rule Humanizer.YourRule testdata/ai_sample.md`
5. Build and install: `make build && make install`

## Build / install / test

```bash
make build    # produces ./humanizer binary
make install  # build + cp to ~/code/bin/humanizer + adhoc codesign
make test     # go test ./...
make tidy     # go mod tidy
make resign   # re-apply adhoc signature without rebuilding (macOS only)
```

Binary installs to `~/code/bin/humanizer`. There is no config file; the only tunable is the vale pack cache directory (`HUMANIZER_CACHE_DIR`, falling back to `XDG_CACHE_HOME`, default `~/.cache/humanizer/vale`).

The test suite includes metadata validation for every YAML header, a gate asserting every rule fires on its own before-example (skipped when vale is not installed), and concurrent-detection race checks.

## Commands

- `humanizer detect [file]`: scan text (stdin, `-`, or a file argument) for AI-writing patterns via the vale span rules. Flags: `--min-severity` (`suggestion`|`warning`|`error`), `--rule` (repeatable rule ID filter), `--statistical` (run the statistical detector instead of vale), `--json` (same shape as `humanizer_detect`).
- `humanizer judge [file]`: whole-passage LLM verdict (`likely_ai`|`likely_human`|`mixed` with confidence, signals, summary) through the configured backend — `OPENROUTER_API_KEY` → OpenRouter on `anthropic/claude-haiku-4.5`. Flags: `--backend` (force one), `--model` (or `HUMANIZER_MODEL`), `--json`. Fails without a configured backend; every other command stays offline.
- `humanizer scan [path]`: extract the prose from Go source (doc comments, cobra `Use`/`Short`/`Long`/`Example`, MCP tool `Description` strings, the message arguments of `fmt.Errorf`/`errors.New`/`http.Error`, and `jsonschema` tag text) and print it as `file:line`-tagged blocks that pipe into `humanizer detect`. Flags: `--go` (required, selects the Go extractor), `--detect` (run the vale span rules, findings reported against the Go source line), `--holistic` (LLM judge per file; skipped with a note when no backend is configured), `--kind` (repeatable: `doc`|`cobra`|`mcp`|`error`|`schema`). A directory is walked recursively, skipping dot-directories, `vendor`, `node_modules`, `testdata`, and generated files; test files are included.
- `humanizer profile [file]`: compute a quantitative voice profile: word/sentence/paragraph counts, type-token ratio, sentence length (mean/stddev/p50/p90), punctuation densities per 100 words (em-dash, semicolon, colon, paren, comma, hyphenated-pair, bold), contraction rate, Flesch reading ease, top bigrams/trigrams. Flags: `--diff <file>` (also print a metric-by-metric delta against a reference sample), `--json`.
- `humanizer rules list`: list every bundled rule (ID, category, default severity, summary). Flags: `--category` (`content`|`language`|`style`|`communication`), `--json`.
- `humanizer rules explain <rule_id>`: print full metadata for one rule: ID, name, category, severity, summary, rationale, before/after examples, reference link.
- `humanizer narrative`: print the StoryScope narrative rubric — 30 discourse-level features separating human from AI fiction. Flags: `--prompt` (only the LLM judge prompt), `--json` (features, themes, prompt, source). Serving only; scoring a passage is the calling agent's job.
- `humanizer lint [file]`: report invisible-Unicode / space-homoglyph watermark carriers (Layer A `scrub.Inspect`). Reports only; exits non-zero when any carrier is found. Flags: `--aggressive` (flag confusables), `--strip-emoji-glue` (paranoid), `--json`.
- `humanizer fix [file]`: apply the Layer A scrub (`scrub.Clean`). Cleaned text to stdout / `-o` / `--in-place` (writes `.bak`); stats to stderr. Non-intrusive by default (strip invisibles, normalize spaces); risky flags `--nfkc`, `--aggressive-homoglyphs`, `--strip-emoji-glue` alter visible characters. `--no-normalize-spaces` opts out of the default space fold.
- `humanizer rewrite [file]`: Layer B rewrite for statistical marks. `--backend print-prompt` (default, offline) returns the prompt; `ollama`/`openai-compatible` run a model. Flags: `--strength`, `--model`, `--base-url`, `--allow-remote`, `--candidates`, `--temperature`, `--no-layer-a-after`, `-o`, `--json-stats`. API key via `WATERMARKS_REWRITE_API_KEY` only.
- `humanizer docs`: print the embedded operating doc (`operating.md` rendered with the binary's own defaults): how humanizer runs, cache location, failure modes, first moves.
- `humanizer mcp`: start the MCP stdio server. An MCP host such as Claude Code launches this; don't run it by hand in normal use. Standalone installs register it once with `claude mcp add --scope user humanizer -- humanizer mcp`; fleet machines get it from the consuming repo's MCP-registration recipe instead (docs/adr/0006).
- `humanizer version`: print the bare version token of the running build. Flags: `-o json` for the four-key build metadata object (`version`, `commit`, `tag`, `build_time`).

## MCP tools

The `mcp` subcommand starts a stdio server (`internal/mcpserver`, built on the [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk)) exposing fourteen tools:

- `humanizer_status()` → `{installed, binary, version, cache_dir, rule_count, style_pack, error?, install_hint?}`. Health check: is vale installed, its version, the cache dir, bundled rule count. Call first when `humanizer_detect` fails unexpectedly.
- `humanizer_detect(text, rules?, min_severity?)` → `{findings[], summary{total, by_severity, by_category, by_rule}, engine}`. Scans a text block with the vale span rules. Each finding carries `rule_id`, `severity`, `line`, `column`, matched text, and message.
- `humanizer_detect_file(path, rules?, min_severity?)` → same shape as `humanizer_detect`. Scans a file on disk instead of inline text; saves a read and lets vale use the real extension for format detection.
  - Sandbox-gated: only prose files (`.md`/`.markdown`/`.txt`) and Go sources (`.go`) under the seatbelt profile's workspace roots, plus `/tmp` paths, are readable. A denied path returns an error pointing at `humanizer_detect` instead. Still a prose tool — use `humanizer_scan_go` to extract and scan Go source.
- `humanizer_scan_go(path, detect?, recursive?)` → `{files[]{path, blocks[]{file, line, kind, label, text}, detection?}, total_files}`. Extracts prose from Go source (doc comments, cobra `Use`/`Short`/`Long`/`Example`, MCP tool `Description` strings, `fmt.Errorf`/`errors.New`/`http.Error` messages, `jsonschema` tag text) and, when `detect` is true (the default), runs the vale span rules over each file's prose. Each file's `detection` field is a `humanizer_detect`-shaped result (`findings[]`, `summary`, `engine`), with finding lines mapped back to the Go source line they came from. `recursive` (default true) walks a directory into subdirectories; false scans only its direct children. Wraps the same `internal/goscan` extraction the `scan --go` CLI command uses.
  - Sandbox-gated like `humanizer_detect_file`, for `.go` files. No MCP fallback for a denied path (extraction needs real file access); run `humanizer scan --go` instead, which is unsandboxed.
- `humanizer_detect_statistical(text)` → `{findings[], profile, summary, engine}`. Whole-sample statistical checks (sentence uniformity, contraction rate, TTR, semicolon absence, short-text em-dash, long-text em-dash density, heading density, anaphora) that span rules can't catch. Most checks gate on a minimum sample size; 200+ words gives the most reliable verdict.
- `humanizer_rules_list(category?)` → `{rules[], total}`. Lists every rule (ID, name, category, default severity, summary), optionally filtered by category.
- `humanizer_rules_explain(rule_id)` → `{id, name, category, default_severity, summary, rationale?, before?, after?, reference?, file?}`. Full metadata for one rule.
- `humanizer_narrative_rubric()` → `{features[], total, themes[], prompt, source}` (the `narrative.Rubric` payload, shared with `humanizer narrative --json`). The StoryScope narrative rubric: 30 discourse-level features (thematic over-explanation, plot linearity, embodied emotion, intertextual reference) that separate human from AI fiction — 84.8% macro-F1 for this core set in the paper, and the narrative signal survives style editing. Serves data only — feed `prompt` plus the passage to an LLM judge (the skill's narrative pass) for fiction and story-shaped prose.
- `humanizer_judge(text, backend?, model?)` → `{verdict, confidence, signals[], summary, backend, model}`. Holistic whole-passage AI/human judgment through the LLM backend (`internal/backend`): auto-detects from env (`OPENROUTER_API_KEY` → openrouter, default model `anthropic/claude-haiku-4.5`; `HUMANIZER_MODEL` overrides). The one networked tool — needs egress plus the key in the server env; under a denying sandbox it errors and the caller falls back to the `humanizer judge` CLI.
- `humanizer_voice_profile(text)` → the voice `Profile` (same metrics as `humanizer profile`). 500+ words gives the most reliable metrics.
- `humanizer_voice_diff(draft, sample)` → `{draft_profile, sample_profile, diff}`. Profiles both texts and returns a metric-by-metric delta sorted by magnitude. Use when the user supplies their own writing as a voice reference.
- `humanizer_lint(text, aggressive?, strip_emoji_glue?)` → the `scrub.Report` (hits with codepoint, kind, confidence, count, sample offsets). Offline, deterministic. Reports invisible-Unicode / space-homoglyph carriers without changing anything.
- `humanizer_fix(text, normalize_spaces?, nfkc?, aggressive_homoglyphs?, strip_emoji_glue?)` → `{cleaned_text, stats}`. Offline, deterministic. `normalize_spaces` defaults true; the other three are the risky, visibly-altering transforms. The caller writes the result.
- `humanizer_rewrite(text, strength?, lang?, original_lang?)` → `{prompt, info}`. Builds a Layer B rewrite prompt (print-prompt only over MCP, so it stays offline). Network backends live in the `humanizer rewrite` CLI, not here.

## Gotchas

- **Wave 0 builder.** Must build before the consuming repo's MCP registration recipe (wave 1) registers it.
- **Runtime debugging lives in `operating.md`** (embedded in the binary, printed by `humanizer docs`): vale-on-PATH, cache location and safety of deleting it, codesign kills, MCP sandbox limits, version skew. Keep those facts there, not here.
- **Version probe convention.** `humanizer version -o json` returns the shared four-key build metadata object (`version`, `commit`, `tag`, `build_time`, every key present and `""` when unknown) from `github.com/mad01/thismoon/buildinfo`, injected by `buildinfo.mk` at link time. Plain `humanizer version` stays a bare token — ralph and status parse it as one. The same `buildinfo.Get().Version` is what the MCP initialize handshake advertises as `serverInfo.version`.
- **Two detection paths, run both.** Vale span rules flag a matched substring with line/column; the statistical detector flags whole-sample properties with no span. Neither alone gives full coverage.
- **Watermark scrub is a third, separate path.** `lint`/`fix` (`internal/scrub`) work on code points, not prose patterns — deterministic and offline, sharing one classifier so a report and its fix never disagree. Load-bearing invisibles (emoji ZWJ/VS after an emoji base, script joiners inside complex scripts, flag tag chars, orthographic Arabic/Syriac Cf) are preserved by default; `strip_emoji_glue` is the paranoid override.
- **humanizer_judge is the one networked MCP tool.** Backend selection is env-driven (`HUMANIZER_BACKEND` override, else the first configured provider; only OpenRouter is implemented — vertex/anthropic are MAD-342 follow-ups). The consuming repo's seatbelt must allow outbound TLS and inject `OPENROUTER_API_KEY` (dotfiles ADR-0020), or the tool errors while everything else stays offline.
- **Judge confidence has two scales.** The rubric asks the model for an integer 0-100 and for the arithmetic behind it (`confidence_basis`), because a free-floating "0.0 to 1.0" ask made haiku answer 0.92 for every input, slop and terse changelogs alike (MAD-348). `parse` normalizes to the 0-1 float the CLI and MCP publish and drops the basis, so the `{verdict, confidence, signals[], summary}` contract is unchanged. Values above 1 are read as percentages, at or below 1 as fractions, which keeps a model that ignores the instruction parseable. When editing the prompt, re-measure the spread across a slop file and a few real docs; the bands and the worked examples are load-bearing, and the test asserts they are still there.
- **Rewrite network backends are CLI-only.** `humanizer_rewrite` over MCP is print-prompt only, by design: the seatbelt's TLS allowance exists for the judge's backend call, not for driving rewrite models from inside the sandbox. The `ollama`/`openai-compatible` backends run from the (unsandboxed) CLI; they default-deny non-loopback hosts, refuse redirects (so the API-key header can't be forwarded), and read the key from `WATERMARKS_REWRITE_API_KEY` only. To let the MCP tool call a local model, the consuming repo's seatbelt would need a loopback exception — not added here.
- **Vale rule gotcha:** `existence`/`occurrence` rules wrap each token in `\b…\b` by default, so a pattern that begins or ends with a non-word char (e.g. a leading `,` plus trailing `\.`, or a trailing `?`/`#`) never matches. Set `nonword: true` on those rules. Single-quoted YAML scalars must escape inner apostrophes as `''`; plain scalars can use `'?` directly. Two more traps: an `occurrence` rule with `max: 0` is disabled (the zero value reads as unset — use `existence` for zero-tolerance), and the default text scope strips markdown markup and splits blocks, so tokens targeting `**`/bullet syntax or spanning blank lines only match under `scope: raw` (stripped inline-code spans also leave `**`-like residue in text scope, which is what made the old BoldOveruse fire on bold-free table rows).
- **The MCP sandbox roots are the consuming repo's.** The seatbelt profile that gates `humanizer_detect_file` (and denies all network) is registered by the consuming repo's wrapper, not this code; new runtime file or network needs require a profile change there.

## See also

- Recipe: `recipes/humanizer/recipe.toml` (this repo, build/install, wave 0)
- MCP registration + sandbox wrapper: the consuming repo's companion recipe (machine-private wiring; two-layer split recorded in `docs/adr/0006`)
