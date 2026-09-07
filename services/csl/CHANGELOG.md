# Changelog

## [0.18.1](https://github.com/mad01/thismoon/compare/csl/v0.18.0...csl/v0.18.1) (2026-09-07)


### Bug Fixes

* **csl:** stabilize search offset paging with a constant rank cap ([#72](https://github.com/mad01/thismoon/issues/72)) ([f3ddc8b](https://github.com/mad01/thismoon/commit/f3ddc8b6f473a84c70dd118dad65fd0a3be118b9))

## [0.18.0](https://github.com/mad01/thismoon/compare/csl/v0.17.0...csl/v0.18.0) (2026-09-07)


### Features

* **csl-web:** disable semantic and hybrid modes when semantic is off ([#69](https://github.com/mad01/thismoon/issues/69)) ([0819a31](https://github.com/mad01/thismoon/commit/0819a31ba8fc86272d421a21927ebd5189385e49))
* **csl:** scale health, refresh, and search for large repo fleets ([#71](https://github.com/mad01/thismoon/issues/71)) ([771f61c](https://github.com/mad01/thismoon/commit/771f61c4659719efb5f76b7b9f8c3066803929e3))

## [0.17.0](https://github.com/mad01/thismoon/compare/csl/v0.16.0...csl/v0.17.0) (2026-09-07)


### Features

* **csl:** resolve git worktrees in repo finder ([#66](https://github.com/mad01/thismoon/issues/66)) ([19b1247](https://github.com/mad01/thismoon/commit/19b12471f1b2a8835eb7b234a47dca2ff880f7f5))

## [0.16.0](https://github.com/mad01/thismoon/compare/csl/v0.15.0...csl/v0.16.0) (2026-08-29)


### Features

* **csl:** zero-config first run, probe-based state resolution ([779bf67](https://github.com/mad01/thismoon/commit/779bf67166fca202cf35426b6a630d672c6dcc0f))

## [0.15.0](https://github.com/mad01/thismoon/compare/csl/v0.14.3...csl/v0.15.0) (2026-08-29)


### Features

* **csl:** config overrides, state separation, effective base URL, doctor checks ([2e0531e](https://github.com/mad01/thismoon/commit/2e0531e9cd6fb388723b4c0433328bae47201f7d))

## [0.14.3](https://github.com/mad01/thismoon/compare/csl/v0.14.2...csl/v0.14.3) (2026-08-27)


### Bug Fixes

* **humanizer:** make dead vale rules fire, add dash-substitute rule, align CLI json with MCP payload ([13fd610](https://github.com/mad01/thismoon/commit/13fd6103fca45c7a0a4c90e81d1e96b3fb73a46f))

## [0.14.2](https://github.com/mad01/thismoon/compare/csl/v0.14.1...csl/v0.14.2) (2026-08-25)


### Bug Fixes

* **csl:** actionable zero-result notes and ambiguous-repo suggestion (MAD-303, MAD-304) ([c03fe72](https://github.com/mad01/thismoon/commit/c03fe72256c7f56402e873fcbb2b4aa25dbb5656))
* MCP tool-call reliability fixes from the 30-day session audit (MAD-300) ([bdd66e4](https://github.com/mad01/thismoon/commit/bdd66e4e34306ad3180a5c2f5ddc06141aa9fb42))

## [0.14.1](https://github.com/mad01/thismoon/compare/csl/v0.14.0...csl/v0.14.1) (2026-08-25)


### Bug Fixes

* **csl:** take the sync lock in the ad-hoc index writers ([#4](https://github.com/mad01/thismoon/issues/4)) ([9259562](https://github.com/mad01/thismoon/commit/9259562f77f128b807abd92d15bc093f11f15c97))

## [0.14.0](https://github.com/mad01/thismoon/compare/csl/v0.13.0...csl/v0.14.0) (2026-08-25)


### Features

* **csl:** background index refresh when the web service is running (MAD-313) ([#2](https://github.com/mad01/thismoon/issues/2)) ([fc25fd9](https://github.com/mad01/thismoon/commit/fc25fd9a2c56a96b94b21f19bad8426562f7284b))

## [0.13.0](https://github.com/mad01/thismoon/compare/csl/v0.12.0...csl/v0.13.0) (2026-08-22)


### Features

* in-band hints on zero-result MCP tool responses ([#176](https://github.com/mad01/thismoon/issues/176)) ([f48f1b0](https://github.com/mad01/thismoon/commit/f48f1b0c9f278f081a457555f8f7716bfc229f4f))

## [0.12.0](https://github.com/mad01/thismoon/compare/csl/v0.11.0...csl/v0.12.0) (2026-08-22)


### Features

* doctor contract for serve-bearing components (ADR-0009 wave 2) ([#174](https://github.com/mad01/thismoon/issues/174)) ([1e68bf4](https://github.com/mad01/thismoon/commit/1e68bf47c522c671828c2cd05fa5ea5ef20e0d87))

## [0.11.0](https://github.com/mad01/thismoon/compare/csl/v0.10.0...csl/v0.11.0) (2026-08-22)


### Features

* agent-facing operating docs across all components (ADR-0009) ([#172](https://github.com/mad01/thismoon/issues/172)) ([d52b48a](https://github.com/mad01/thismoon/commit/d52b48a5983e3c3f3ed8d1d1f30d269c326f5bee))

## [0.10.0](https://github.com/mad01/thismoon/compare/csl/v0.9.0...csl/v0.10.0) (2026-08-17)


### Features

* **csl:** fleet repo-health report and file-view portal (MAD-224) ([#134](https://github.com/mad01/thismoon/issues/134)) ([72eef81](https://github.com/mad01/thismoon/commit/72eef81b975e71500e788b172d9cdb68d81cfc76))

## [0.9.0](https://github.com/mad01/thismoon/compare/csl/v0.8.0...csl/v0.9.0) (2026-08-13)


### Features

* **csl:** add config command ([515467c](https://github.com/mad01/thismoon/commit/515467c97459a3a1779754ba6ba8d99c44700e1d))
* shared buildinfo package — commit/tag/build time in version and /version across all components ([#125](https://github.com/mad01/thismoon/issues/125)) ([3fc9af5](https://github.com/mad01/thismoon/commit/3fc9af5f3014052300da6ab55325190623867d96))

## [0.8.0](https://github.com/mad01/thismoon/compare/csl/v0.7.2...csl/v0.8.0) (2026-07-18)


### Features

* **csl:** ship repo-sync and repo shell functions via the ralph recipe ([#99](https://github.com/mad01/thismoon/issues/99)) ([3c7259c](https://github.com/mad01/thismoon/commit/3c7259c049856511e580bec7ea42774636bd3694))

## [0.7.2](https://github.com/mad01/thismoon/compare/csl/v0.7.1...csl/v0.7.2) (2026-07-15)


### Bug Fixes

* **csl:** serve the web UI when the config file is missing ([#93](https://github.com/mad01/thismoon/issues/93)) ([ff0942c](https://github.com/mad01/thismoon/commit/ff0942c7d349e35de5c4aebb4e4e3e16fc99c3f3))

## [0.7.1](https://github.com/mad01/thismoon/compare/csl/v0.7.0...csl/v0.7.1) (2026-07-15)


### Miscellaneous Chores

* **csl:** note prebuilt release tarballs in the README ([#85](https://github.com/mad01/thismoon/issues/85)) ([4ec5a8e](https://github.com/mad01/thismoon/commit/4ec5a8e4461eb44c3df94c4208448337398fd816))

## [0.7.0](https://github.com/mad01/thismoon/compare/csl/v0.6.0...csl/v0.7.0) (2026-07-12)


### Features

* **csl:** default embed model to jina-code-v2 ([#59](https://github.com/mad01/thismoon/issues/59)) ([36220da](https://github.com/mad01/thismoon/commit/36220dae39651d8db31b6a931a6dcac938c06011))

## [0.6.0](https://github.com/mad01/thismoon/compare/csl/v0.5.0...csl/v0.6.0) (2026-07-11)


### Features

* **csl:** filter build output, vendor trees, and model data from semantic indexing ([#57](https://github.com/mad01/thismoon/issues/57)) ([16ca86e](https://github.com/mad01/thismoon/commit/16ca86e172c7ac3b18e891368ddbb2e64235be8f))

## [0.5.0](https://github.com/mad01/thismoon/compare/csl/v0.4.1...csl/v0.5.0) (2026-07-11)


### Features

* **csl:** embed via ollama, drop hugot ([#54](https://github.com/mad01/thismoon/issues/54)) ([cfd53eb](https://github.com/mad01/thismoon/commit/cfd53ebebe0b0b644d1ba73893031217e9be88b5))

## [0.4.1](https://github.com/mad01/thismoon/compare/csl/v0.4.0...csl/v0.4.1) (2026-07-10)


### Bug Fixes

* **csl:** apply the exclude list to lexical index paths ([#52](https://github.com/mad01/thismoon/issues/52)) ([990501d](https://github.com/mad01/thismoon/commit/990501ddd363520fad92faca4ee45a9e8b09a8f4))

## [0.4.0](https://github.com/mad01/thismoon/compare/csl/v0.3.0...csl/v0.4.0) (2026-07-10)


### Features

* **csl:** tree-sitter chunking for hcl, bash, dockerfile, markdown, protobuf, sql, yaml ([#49](https://github.com/mad01/thismoon/issues/49)) ([3bf97a0](https://github.com/mad01/thismoon/commit/3bf97a057b5897027823b91726a8b373ccc019b0))


### Bug Fixes

* **csl:** rebuild semantic stores when the chunker version changes ([#51](https://github.com/mad01/thismoon/issues/51)) ([cf1f2aa](https://github.com/mad01/thismoon/commit/cf1f2aab742cbe68d661bdcf6d042b58103db865))

## [0.3.0](https://github.com/mad01/thismoon/compare/csl/v0.2.0...csl/v0.3.0) (2026-07-10)


### Features

* **csl:** symbol-aware Java chunking with per-method chunks ([#47](https://github.com/mad01/thismoon/issues/47)) ([7462263](https://github.com/mad01/thismoon/commit/7462263258dea7bb187c6517ea438cb08d0d337c))

## [0.2.0](https://github.com/mad01/thismoon/compare/csl/v0.1.0...csl/v0.2.0) (2026-07-07)


### Features

* migrate csl + catalog into the monorepo ([#15](https://github.com/mad01/thismoon/issues/15)) ([15d3f0a](https://github.com/mad01/thismoon/commit/15d3f0aa7878787457ee4616fd95d2dcce6c26a0))
