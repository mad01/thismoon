# macOS arm64 as the only artifact target

Supersedes the target list in `0004-build-targets-and-signing.md` (the signing half of that ADR still stands).

Release artifacts build for darwin/arm64 only. The platform is macOS through and through — services run as launchd agents under t-man, reminder fires macOS notifications, local installs codesign — so the linux/amd64 and linux/arm64 tarballs were build surface without users. Dropping them is cheap to reverse: the release workflow's platform loop takes the target list, and adding an entry back restores the artifact.
