# kit/internalnames

The reader for the shared internal-names files that belt
(`internal_names.include`) and suspenders (`guard.include`) each name in
their own config (docs/adr/0016). One package so the file format has one
implementation.

- `File` holds the three lists a names file may carry: `blocked_words`,
  `allowlist`, `allow_phrases`.
- `Read(path)` decodes one file strictly. An unknown key is an error naming
  it, an empty file is an empty `File`, and a missing file comes back as the
  unwrapped open error so callers can tell absence from breakage with
  `errors.Is(err, fs.ErrNotExist)`.

The lists come back as written. Each consumer applies its own derivation on
top (belt folds case and drops names under three characters; suspenders
keeps case), the same rules it applies to the lists in its own config.
