---
id: ai-cli-readme-intro
label: likely_ai
bucket: ai
source: generated
license: generated for this eval
generator: claude-sonnet-5-5 via claude -p, 2026-10-08
words: 169
notes: README intro genre with a bold tagline and bold lead-in bullets. Prompt was Write the introduction section of a README for an open-source command-line tool called tidepool that keeps dotfiles in sync across several machines. About 150 words. Output only the README text, nothing else.
---
# tidepool

**tidepool keeps your dotfiles in sync across all your machines.**

You tweak your shell prompt on your laptop, add an alias on your work desktop, and fix an editor setting on a remote server. A week later, none of them match. tidepool fixes that.

It watches the configuration files you choose, such as `.zshrc`, `.gitconfig`, `.config/nvim`, and anything else, and keeps them consistent everywhere. Changes flow between machines like the tide: gently, predictably, and without overwriting your work. When two machines edit the same file, tidepool shows you both versions and lets you decide.

- **Simple setup:** run `tidepool init` and point it at the files you care about.
- **Safe by default:** every change is versioned, so you can roll back.
- **Machine-aware:** keep per-host overrides for settings that shouldn't be shared.
- **Works with your tools:** sync over Git, SSH, or a plain shared folder.

tidepool is a single static binary, written for people who live in the terminal, and released under an open-source license.
