# Changelog

## [1.1.0](https://github.com/mad01/thismoon/compare/belt/v1.0.0...belt/v1.1.0) (2026-08-28)


### Features

* **belt:** internal_names.allow_phrases — sanctioned compounds pass the write guard ([a4689c8](https://github.com/mad01/thismoon/commit/a4689c880990190dfafc632ca7b5550ae7d47d48))

## [1.0.0](https://github.com/mad01/thismoon/compare/belt/v0.20.0...belt/v1.0.0) (2026-08-28)


### ⚠ BREAKING CHANGES

* **belt:** belt override set/extend without --reason now exit with an error.

### Features

* **belt:** require --reason on override set and extend ([#27](https://github.com/mad01/thismoon/issues/27)) ([fc1f936](https://github.com/mad01/thismoon/commit/fc1f9362afd11ec92de788a0f0f056d2ef2a5deb))

## [0.20.0](https://github.com/mad01/thismoon/compare/belt/v0.19.0...belt/v0.20.0) (2026-08-28)


### Features

* **belt:** harden script-deny-list against script-shaped bypasses ([1ce48ed](https://github.com/mad01/thismoon/commit/1ce48ed735e91d0dded9cf82f66b1399b3e3ba4b))

## [0.19.0](https://github.com/mad01/thismoon/compare/belt/v0.18.1...belt/v0.19.0) (2026-08-27)


### Features

* **belt:** timed guard overrides with extend and audit events ([07b8c70](https://github.com/mad01/thismoon/commit/07b8c70d704fc913dafb70d3c0043137a8ff8e37))

## [0.18.1](https://github.com/mad01/thismoon/compare/belt/v0.18.0...belt/v0.18.1) (2026-08-26)


### Bug Fixes

* **belt:** add missing humanizer-check hint to embedded config reference ([2c78bb4](https://github.com/mad01/thismoon/commit/2c78bb49e813984df9d08790ad1b6b5279f69960))

## [0.18.0](https://github.com/mad01/thismoon/compare/belt/v0.17.0...belt/v0.18.0) (2026-08-25)


### Features

* **belt:** git-identity and work-hours commit guards, custom external guards ([#12](https://github.com/mad01/thismoon/issues/12)) ([4988ce6](https://github.com/mad01/thismoon/commit/4988ce638684d0e09966276c0ca19303fe7d6f8f))

## [0.17.0](https://github.com/mad01/thismoon/compare/belt/v0.16.0...belt/v0.17.0) (2026-08-25)


### Features

* **belt:** make kof-deposit and humanizer-check nudges directive and concrete (MAD-307) ([b45f961](https://github.com/mad01/thismoon/commit/b45f961f176f511d2914ce16edab228fe93b4a92))


### Bug Fixes

* MCP tool-call reliability fixes from the 30-day session audit (MAD-300) ([bdd66e4](https://github.com/mad01/thismoon/commit/bdd66e4e34306ad3180a5c2f5ddc06141aa9fb42))

## [0.16.0](https://github.com/mad01/thismoon/compare/belt/v0.15.0...belt/v0.16.0) (2026-08-22)


### Features

* doctor contract for serve-bearing components (ADR-0009 wave 2) ([#174](https://github.com/mad01/thismoon/issues/174)) ([1e68bf4](https://github.com/mad01/thismoon/commit/1e68bf47c522c671828c2cd05fa5ea5ef20e0d87))

## [0.15.0](https://github.com/mad01/thismoon/compare/belt/v0.14.0...belt/v0.15.0) (2026-08-22)


### Features

* agent-facing operating docs across all components (ADR-0009) ([#172](https://github.com/mad01/thismoon/issues/172)) ([d52b48a](https://github.com/mad01/thismoon/commit/d52b48a5983e3c3f3ed8d1d1f30d269c326f5bee))

## [0.14.0](https://github.com/mad01/thismoon/compare/belt/v0.13.0...belt/v0.14.0) (2026-08-21)


### Features

* **belt:** profile-scoped guard allowlist (allow_repos_by_profile) ([#169](https://github.com/mad01/thismoon/issues/169)) ([22ea212](https://github.com/mad01/thismoon/commit/22ea212caa113c0d480e8727fe32b70742f32efb))

## [0.13.0](https://github.com/mad01/thismoon/compare/belt/v0.12.1...belt/v0.13.0) (2026-08-21)


### Features

* **belt:** inject the work agent-memory store when present ([#167](https://github.com/mad01/thismoon/issues/167)) ([5bc353d](https://github.com/mad01/thismoon/commit/5bc353d01c85eb7259af4cbffb9f3a0ed90e73ec))

## [0.12.1](https://github.com/mad01/thismoon/compare/belt/v0.12.0...belt/v0.12.1) (2026-08-19)


### Bug Fixes

* **keeper-of-facts:** sweep remaining pre-rename keep references ([#157](https://github.com/mad01/thismoon/issues/157)) ([11aad7b](https://github.com/mad01/thismoon/commit/11aad7b949d138dcc7eb7565f9652aa8b3f98dd0))

## [0.12.0](https://github.com/mad01/thismoon/compare/belt/v0.11.0...belt/v0.12.0) (2026-08-19)


### Features

* **belt:** add humanizer-check hint for externally published text ([#155](https://github.com/mad01/thismoon/issues/155)) ([f4b5cb4](https://github.com/mad01/thismoon/commit/f4b5cb4335cecf0cc4b261a244a28bed3bd00422))

## [0.11.0](https://github.com/mad01/thismoon/compare/belt/v0.10.0...belt/v0.11.0) (2026-08-19)


### Features

* **belt:** doctor reports kof serve reachability and store size ([#152](https://github.com/mad01/thismoon/issues/152)) ([d2dcad9](https://github.com/mad01/thismoon/commit/d2dcad99f3a91aec2e7f8f21dd6cc6a602b02b26))

## [0.10.0](https://github.com/mad01/thismoon/compare/belt/v0.9.0...belt/v0.10.0) (2026-08-18)


### Features

* **belt:** rename keep-* hint ids to kof-* ([#150](https://github.com/mad01/thismoon/issues/150)) ([6e0c8d7](https://github.com/mad01/thismoon/commit/6e0c8d75c9924ceb73eb68869d9c0799b559bb5c))

## [0.9.0](https://github.com/mad01/thismoon/compare/belt/v0.8.0...belt/v0.9.0) (2026-08-18)


### Features

* **belt:** agent-memory hint — inject the shared memory index at session start (MAD-271) ([#144](https://github.com/mad01/thismoon/issues/144)) ([269602a](https://github.com/mad01/thismoon/commit/269602a5aa1e8fe61a1cbabb2fac823ad2f67b6c))
* **belt:** deposit nudge — remind keep_assert once per session (MAD-264) ([#140](https://github.com/mad01/thismoon/issues/140)) ([58c6f3d](https://github.com/mad01/thismoon/commit/58c6f3d21ad8baeaeac3161d8f2e3c80a8dca884))
* **belt:** session-start keep consult hint (MAD-263) ([#138](https://github.com/mad01/thismoon/issues/138)) ([18324c3](https://github.com/mad01/thismoon/commit/18324c3e9b9403d1f6581519d23b5168fbd62347))
* **keeper-of-facts:** rename keep -&gt; keeper-of-facts, kof binary and kof_* tools (MAD-266) ([#141](https://github.com/mad01/thismoon/issues/141)) ([8731a62](https://github.com/mad01/thismoon/commit/8731a623ddb965f7a0a11e9fef55e8b593ca98a9))

## [0.8.0](https://github.com/mad01/thismoon/compare/belt/v0.7.0...belt/v0.8.0) (2026-08-14)


### Features

* **belt:** render the full effective config from belt config ([#132](https://github.com/mad01/thismoon/issues/132)) ([6888e01](https://github.com/mad01/thismoon/commit/6888e01cdfea5dc81d1603748cf467fc09913fe8))

## [0.7.0](https://github.com/mad01/thismoon/compare/belt/v0.6.0...belt/v0.7.0) (2026-08-14)


### Features

* **belt:** own internal_names and profiles config with repo-based names ([d53353b](https://github.com/mad01/thismoon/commit/d53353b1543e9bdfda1bbb33d9226f0260ba6b2d))

## [0.6.0](https://github.com/mad01/thismoon/compare/belt/v0.5.0...belt/v0.6.0) (2026-08-13)


### Features

* **belt:** show effective config in config command, move reference to --help ([edb67af](https://github.com/mad01/thismoon/commit/edb67afb9559fcbd8280d6bcae64f14d3e6861c8))
* shared buildinfo package — commit/tag/build time in version and /version across all components ([#125](https://github.com/mad01/thismoon/issues/125)) ([3fc9af5](https://github.com/mad01/thismoon/commit/3fc9af5f3014052300da6ab55325190623867d96))

## [0.5.0](https://github.com/mad01/thismoon/compare/belt/v0.4.0...belt/v0.5.0) (2026-08-13)


### Features

* **belt:** add config reference command ([364768f](https://github.com/mad01/thismoon/commit/364768f0f91481527ea2310d46c3632eb568de67))
* **belt:** add doctor command and YAML config ([9989e2b](https://github.com/mad01/thismoon/commit/9989e2b0ac4d34435b9ed479937d649d6568b4ef))

## [0.4.0](https://github.com/mad01/thismoon/compare/belt/v0.3.0...belt/v0.4.0) (2026-08-11)


### Features

* **belt:** add per-repo allow_repos exemption to git-push-main guard ([#122](https://github.com/mad01/thismoon/issues/122)) ([79d3380](https://github.com/mad01/thismoon/commit/79d3380d1453ba79d05e0cdbdbb4f63f904e0051))
* **belt:** allow internal names in repos matched by allow_repos ([9f966e9](https://github.com/mad01/thismoon/commit/9f966e9b68d109078284171e64f3dc85bd6c14df))

## [0.3.0](https://github.com/mad01/thismoon/compare/belt/v0.2.1...belt/v0.3.0) (2026-07-18)


### Features

* **belt:** add advisory hints alongside deny-only guards ([#101](https://github.com/mad01/thismoon/issues/101)) ([614128e](https://github.com/mad01/thismoon/commit/614128ecf0ce90b151dea85441ec71e93f313198))

## [0.2.1](https://github.com/mad01/thismoon/compare/belt/v0.2.0...belt/v0.2.1) (2026-07-15)


### Bug Fixes

* **belt:** reject unknown hook events instead of silently allowing ([#94](https://github.com/mad01/thismoon/issues/94)) ([31bc61a](https://github.com/mad01/thismoon/commit/31bc61a89a2a353352a293ddc2a44e7ec862fe98))

## [0.2.0](https://github.com/mad01/thismoon/compare/belt/v0.1.0...belt/v0.2.0) (2026-07-07)


### Features

* MAD-208 migrate belt into tools/belt ([#19](https://github.com/mad01/thismoon/issues/19)) ([f3cd868](https://github.com/mad01/thismoon/commit/f3cd868d155536b2ad381373ccca8c914842fc90))
