---
id: ai-minor-release-announcement
label: likely_ai
bucket: ai
split: test
source: generated
license: generated for this eval
generator: openai/gpt-5.5 via OpenRouter chat completions, 2026-10-09
words: 199
notes: Minor-release announcement with an exclamation opener, promotional adjectives, a thanks paragraph and a values-statement close. Prompt was Write a release announcement for version 3.2.0 of an open-source Rust command-line JSON formatter called jfmt. About 200 words. Output only the announcement, nothing else.
---
jfmt 3.2.0 is now available!

This release brings a smoother, faster experience to the open-source Rust command-line JSON formatter many of you use in scripts, editors, and CI pipelines. The headline feature is improved streaming support for large JSON inputs, reducing peak memory usage and making `jfmt` more reliable when working with logs, exports, and generated data files.

We’ve also added a new `--check` mode for CI workflows. It verifies whether input is already formatted and exits with a non-zero status when changes would be required, without rewriting output. Pair it with `--diff` to see exactly what needs updating.

Other improvements include clearer parse error messages with line and column context, better handling of stdin/stdout edge cases, and several formatting consistency fixes around nested arrays and objects. Shell completion generation has also been expanded, making setup easier for Bash, Zsh, Fish, and PowerShell users.

As always, you can install or upgrade with:

```sh
cargo install jfmt
```

Thanks to everyone who opened issues, tested prereleases, contributed patches, and shared feedback. jfmt remains focused on being small, fast, predictable, and easy to integrate wherever JSON needs to be readable.

Full changelog and source code are available in the project repository.
