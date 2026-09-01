# why humanizer

## The problem

Prose drafted with agent help (docs, README updates, PR descriptions) tends
to carry recognizable AI-writing tells: inflated vocabulary, formulaic
transitions, unnaturally uniform sentence lengths, missing contractions.
Judging "does this read as machine-written" by eye is unreliable, and
asking a model to grade its own output is circular. What was missing on
this machine was a deterministic, local check that flags specific patterns
at specific positions, so a writer or an agent can fix exactly what was
flagged before text gets committed or sent.

## Why its own tool

The detection engine is not homegrown: humanizer shells out to vale, an
existing prose linter, instead of reimplementing span matching. What no
off-the-shelf setup provided is everything around it: a style pack of 52
rules covering patterns from Wikipedia's "Signs of AI writing" and
StoryScope's fiction-prose tells, each with
rationale and before/after examples, a statistical detector for
whole-sample tells that no single sentence exhibits, voice profiling and
diffing, and an MCP surface so agents can run all of it as tool calls.
That bundle needs its own test corpus and metadata validation, and no
other component in this repo deals in text analysis.

## Why this shape

Detection only, no rewriting. The tool reports findings deterministically
and leaves fixes to the calling agent or human, which keeps it testable:
the same input always yields the same findings.

Two independent paths cover different tells. Vale span rules match specific
phrasing with line and column positions; the statistical checks
(sentence-length uniformity, contraction rate, type-token ratio, em-dash
density, heading density, anaphora) run on the whole sample and are
size-gated so short
snippets don't produce noise. Neither path alone gives full coverage, so
both ship in one tool.

The style pack is embedded in the tool and extracted to a cache directory
on first use, so installing the component is enough — no separate style
download or vale configuration to manage. Everything runs offline, which
makes the MCP server cheap to sandbox: the consuming repo's registration
wrapper denies all network and nearly all home-directory access, and that
wiring stays machine-private per the two-layer recipe split
(docs/adr/0006).

## Non-goals

humanizer does not rewrite text; it flags patterns and stops. It does not
claim to prove authorship — a finding means a known tell is present, not
that a machine wrote the sentence. It never touches the network. And its
file access from MCP is deliberately narrow: `humanizer_detect_file` and
`humanizer_scan_go` read only prose and Go files under the sandbox
profile's workspace roots, with inline text via `humanizer_detect` as the
path for everything else `humanizer_detect_file` can't reach.
