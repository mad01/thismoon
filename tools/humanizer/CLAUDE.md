# humanizer, CLI and MCP server for AI-writing detection and voice profiling

Go CLI + MCP server. Detects AI-writing patterns by shelling out to `vale` against an embedded Humanizer style pack, produces a quantitative voice profile, and exposes both as MCP tools for use inside Claude Code.

## Module layout

```
humanizer/
  cmd/humanizer/     - entrypoint (delegates to internal/cli)
  internal/
    cli/             - cobra command tree (root, detect, profile, rules, lint, fix, rewrite, mcp, docs, version); build metadata from the shared buildinfo package
    mcpserver/       - MCP server wiring (server.go, tools_detect.go, tools_statistical.go, tools_rules.go, tools_status.go, tools_voice.go, tools_scrub.go, tools_rewrite.go)
    rules/           - vale style pack embedding and metadata (embed.go, metadata.go, vale.go, vale/)
    voice/           - voice profiler, statistical detector, and diff logic (profile.go, statistical.go, diff.go)
    scrub/           - Layer A watermark scrub: invisible-Unicode/homoglyph carrier tables, Inspect (lint) and Clean (fix)
    rewrite/         - Layer B watermark rewrite: prompt builder, candidate selection, hardened HTTP backends (ported from watermarks-remover, MIT)
  testdata/          - fixture files for tests (ai_sample.md, human_sample.md)
  Makefile           - package path github.com/mad01/thismoon/tools/humanizer, part of the monorepo module; no go.mod of its own
```

## How it works

Two independent detection paths, both backed by the same text:

- Vale span rules (`humanizer_detect` / `humanizer detect`): the embedded style pack under `internal/rules/vale/styles/Humanizer/*.yml` flags a matched substring with line/column.
- Statistical detector (`humanizer_detect_statistical` / `humanizer detect --statistical`, `internal/voice/statistical.go`): flags whole-sample properties (sentence-length stddev, contraction rate, type-token ratio, short-text em-dash, semicolon absence, heading density, anaphora) with no span, each gated on a minimum sample size.

Run both for full coverage. Span rules catch specific phrasing tells; the statistical detector catches structural ones that no single sentence exhibits.

### Style pack

The Humanizer style pack ships embedded in the binary under `internal/rules/vale/styles/Humanizer/`. It contains 46 rules covering patterns from Wikipedia's "Signs of AI writing":

AIVocabulary, AphoristicClosure, BoldOveruse, ChatGPTArtifacts, CitationArtifacts, ClosingRitualPhrases, CollaborativeArtifacts, ContractionAvoidance, CopulaAvoidance, CurlyQuotes, EmDashOveruse, EmojiDecoration, ExcessiveHedging, FalseBothSidesHedge, FalseConcession, FalseRanges, FalseVulnerability, FillerBoilerplate, FillerPhrases, FiveParagraphStructure, FormulaicChallenges, FragmentedHeader, GenericConclusion, HashtagStuffing, HyphenatedPairOveruse, InfomercialHooks, InlineHeaderList, KnowledgeCutoff, LetsConstructions, NegativeParallelism, NotabilityInflation, ParticipialTailExtended, PassiveVoice, PersuasiveAuthority, PromotionalVocab, RhetoricalTransitions, RuleOfThree, SignificanceInflation, Signposting, SuperficialIng, Sycophancy, TailingNegation, TitleCaseHeadings, UnfilledPlaceholders, UTMParameters, VagueAttribution

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

The test suite includes metadata validation for every YAML header and concurrent-detection race checks.

## Commands

- `humanizer detect [file]`: scan text (stdin, `-`, or a file argument) for AI-writing patterns via the vale span rules. Flags: `--min-severity` (`suggestion`|`warning`|`error`), `--rule` (repeatable rule ID filter), `--statistical` (run the statistical detector instead of vale), `--json` (same shape as `humanizer_detect`).
- `humanizer profile [file]`: compute a quantitative voice profile: word/sentence/paragraph counts, type-token ratio, sentence length (mean/stddev/p50/p90), punctuation densities per 100 words (em-dash, semicolon, colon, paren, comma, hyphenated-pair, bold), contraction rate, Flesch reading ease, top bigrams/trigrams. Flags: `--diff <file>` (also print a metric-by-metric delta against a reference sample), `--json`.
- `humanizer rules list`: list every bundled rule (ID, category, default severity, summary). Flags: `--category` (`content`|`language`|`style`|`communication`), `--json`.
- `humanizer rules explain <rule_id>`: print full metadata for one rule: ID, name, category, severity, summary, rationale, before/after examples, reference link.
- `humanizer lint [file]`: report invisible-Unicode / space-homoglyph watermark carriers (Layer A `scrub.Inspect`). Reports only; exits non-zero when any carrier is found. Flags: `--aggressive` (flag confusables), `--strip-emoji-glue` (paranoid), `--json`.
- `humanizer fix [file]`: apply the Layer A scrub (`scrub.Clean`). Cleaned text to stdout / `-o` / `--in-place` (writes `.bak`); stats to stderr. Non-intrusive by default (strip invisibles, normalize spaces); risky flags `--nfkc`, `--aggressive-homoglyphs`, `--strip-emoji-glue` alter visible characters. `--no-normalize-spaces` opts out of the default space fold.
- `humanizer rewrite [file]`: Layer B rewrite for statistical marks. `--backend print-prompt` (default, offline) returns the prompt; `ollama`/`openai-compatible` run a model. Flags: `--strength`, `--model`, `--base-url`, `--allow-remote`, `--candidates`, `--temperature`, `--no-layer-a-after`, `-o`, `--json-stats`. API key via `WATERMARKS_REWRITE_API_KEY` only.
- `humanizer docs`: print the embedded operating doc (`operating.md` rendered with the binary's own defaults): how humanizer runs, cache location, failure modes, first moves.
- `humanizer mcp`: start the MCP stdio server. An MCP host such as Claude Code launches this; don't run it by hand in normal use. Register once with `claude mcp add --scope user humanizer -- humanizer mcp`.
- `humanizer version`: print the bare version token of the running build. Flags: `-o json` for the four-key build metadata object (`version`, `commit`, `tag`, `build_time`).

## MCP tools

The `mcp` subcommand starts a stdio server (`internal/mcpserver`, built on the [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk)) exposing eleven tools:

- `humanizer_status()` → `{installed, binary, version, cache_dir, rule_count, style_pack, error?, install_hint?}`. Health check: is vale installed, its version, the cache dir, bundled rule count. Call first when `humanizer_detect` fails unexpectedly.
- `humanizer_detect(text, rules?, min_severity?)` → `{findings[], summary{total, by_severity, by_category, by_rule}, engine}`. Scans a text block with the vale span rules. Each finding carries `rule_id`, `severity`, `line`, `column`, matched text, and message.
- `humanizer_detect_file(path, rules?, min_severity?)` → same shape as `humanizer_detect`. Scans a file on disk instead of inline text; saves a read and lets vale use the real extension for format detection.
  - Sandbox-gated: only prose files (`.md`/`.markdown`/`.txt`) under the seatbelt profile's workspace roots, plus `/tmp` paths, are readable. A denied path returns an error pointing at `humanizer_detect` instead.
- `humanizer_detect_statistical(text)` → `{findings[], profile, summary, engine}`. Whole-sample statistical checks (sentence uniformity, contraction rate, TTR, semicolon absence, short-text em-dash, heading density, anaphora) that span rules can't catch. Most checks gate on a minimum sample size; 200+ words gives the most reliable verdict.
- `humanizer_rules_list(category?)` → `{rules[], total}`. Lists every rule (ID, name, category, default severity, summary), optionally filtered by category.
- `humanizer_rules_explain(rule_id)` → `{id, name, category, default_severity, summary, rationale?, before?, after?, reference?, file?}`. Full metadata for one rule.
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
- **Rewrite network backends are CLI-only.** `humanizer_rewrite` over MCP is print-prompt only, because the MCP seatbelt denies all network. The `ollama`/`openai-compatible` backends run from the (unsandboxed) CLI; they default-deny non-loopback hosts, refuse redirects (so the API-key header can't be forwarded), and read the key from `WATERMARKS_REWRITE_API_KEY` only. To let the MCP tool call a local model, the consuming repo's seatbelt would need a loopback exception — not added here.
- **Vale rule gotcha:** `existence`/`occurrence` rules wrap each token in `\b…\b` by default, so a pattern that begins or ends with a non-word char (e.g. a leading `,` plus trailing `\.`, or a trailing `?`/`#`) never matches. Set `nonword: true` on those rules. Single-quoted YAML scalars must escape inner apostrophes as `''`; plain scalars can use `'?` directly.
- **The MCP sandbox roots are the consuming repo's.** The seatbelt profile that gates `humanizer_detect_file` (and denies all network) is registered by the consuming repo's wrapper, not this code; new runtime file or network needs require a profile change there.

## See also

- Recipe: `recipes/humanizer/recipe.toml` (this repo, build/install, wave 0)
- MCP registration + sandbox wrapper: the consuming repo's companion recipe (machine-private wiring; two-layer split recorded in `docs/adr/0006`)
