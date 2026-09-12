# Changelog

## [0.10.0](https://github.com/mad01/thismoon/compare/worklog/v0.9.0...worklog/v0.10.0) (2026-09-12)


### Features

* annotate MCP tools, gate csl semantic tools on config, bump go-sdk to v1.8.0-pre.2 ([82ec8c4](https://github.com/mad01/thismoon/commit/82ec8c4898885b97945f2dc08edcf6584040814c))
* **worklog:** annotate MCP tools with read-only and open-world hints ([c1a8a3f](https://github.com/mad01/thismoon/commit/c1a8a3fddf1e04fbcf28d6fe2b574aa94bfa5e58))

## [0.9.0](https://github.com/mad01/thismoon/compare/worklog/v0.8.2...worklog/v0.9.0) (2026-08-29)


### Features

* **worklog:** strict config loading, neutral scan defaults, remote.url ([903c13e](https://github.com/mad01/thismoon/commit/903c13e89e25edefa366373e5b634d7387155351))

## [0.8.2](https://github.com/mad01/thismoon/compare/worklog/v0.8.1...worklog/v0.8.2) (2026-08-27)


### Bug Fixes

* **worklog:** point config help at docs, not a nonexistent doctor ([4148b56](https://github.com/mad01/thismoon/commit/4148b56578d2136e76a71ed2734a2a49fc8c06bc))

## [0.8.1](https://github.com/mad01/thismoon/compare/worklog/v0.8.0...worklog/v0.8.1) (2026-08-25)


### Bug Fixes

* MCP tool-call reliability fixes from the 30-day session audit (MAD-300) ([bdd66e4](https://github.com/mad01/thismoon/commit/bdd66e4e34306ad3180a5c2f5ddc06141aa9fb42))
* **worklog:** point worklog_show key misses at worklog_search (MAD-305) ([2cfb88e](https://github.com/mad01/thismoon/commit/2cfb88eeb4bd0f4b0dd0e185cf52d1f8f699823a))

## [0.8.0](https://github.com/mad01/thismoon/compare/worklog/v0.7.1...worklog/v0.8.0) (2026-08-24)


### Features

* agent-facing operating docs across all components (ADR-0009) ([#172](https://github.com/mad01/thismoon/issues/172)) ([03b3db8](https://github.com/mad01/thismoon/commit/03b3db8887a744910ee4772309cf88a9fcf0ced6))
* doctor contract for serve-bearing components (ADR-0009 wave 2) ([#174](https://github.com/mad01/thismoon/issues/174)) ([1ed2b87](https://github.com/mad01/thismoon/commit/1ed2b87a4c6a97d63382d03b182f0362ed6fc9f1))
* in-band hints on zero-result MCP tool responses ([#176](https://github.com/mad01/thismoon/issues/176)) ([2747c1e](https://github.com/mad01/thismoon/commit/2747c1ee2dfcd17b9c68cba63c96e8e4cb8be27a))
* MAD-210 migrate worklog into tools/worklog ([#21](https://github.com/mad01/thismoon/issues/21)) ([ddfb60b](https://github.com/mad01/thismoon/commit/ddfb60b4e3df94760012506726e374c72f4c05fa))
* shared buildinfo package — commit/tag/build time in version and /version across all components ([#125](https://github.com/mad01/thismoon/issues/125)) ([6b2ee8b](https://github.com/mad01/thismoon/commit/6b2ee8b29f7ff1180e7f69cc8a3cbe58a8cefe38))
* **worklog:** add config command ([78b16ff](https://github.com/mad01/thismoon/commit/78b16fff8003beea9ceec15a46bab8c752de9ce1))
* **worklog:** remote upstream support for the store ([#166](https://github.com/mad01/thismoon/issues/166)) ([2ae9442](https://github.com/mad01/thismoon/commit/2ae94426b74322557063e724e3fe1fb6149aa868))


### Bug Fixes

* ship MCP field descriptions via the jsonschema tag ([#178](https://github.com/mad01/thismoon/issues/178)) ([d83cfd4](https://github.com/mad01/thismoon/commit/d83cfd4e05f10c4f406f010ce0d01c18fff4afe6))
* **worklog:** make checkout roots and repo path markers configurable ([#67](https://github.com/mad01/thismoon/issues/67)) ([ec28eec](https://github.com/mad01/thismoon/commit/ec28eecbc885d43f4b6c8436191c92bb8f0aabe1))

## [0.7.1](https://github.com/mad01/thismoon/compare/worklog/v0.7.0...worklog/v0.7.1) (2026-08-22)


### Bug Fixes

* ship MCP field descriptions via the jsonschema tag ([#178](https://github.com/mad01/thismoon/issues/178)) ([c38161e](https://github.com/mad01/thismoon/commit/c38161ed0ecb106ae64b3b9bf02a9039c7879a21))

## [0.7.0](https://github.com/mad01/thismoon/compare/worklog/v0.6.0...worklog/v0.7.0) (2026-08-22)


### Features

* in-band hints on zero-result MCP tool responses ([#176](https://github.com/mad01/thismoon/issues/176)) ([f48f1b0](https://github.com/mad01/thismoon/commit/f48f1b0c9f278f081a457555f8f7716bfc229f4f))

## [0.6.0](https://github.com/mad01/thismoon/compare/worklog/v0.5.0...worklog/v0.6.0) (2026-08-22)


### Features

* doctor contract for serve-bearing components (ADR-0009 wave 2) ([#174](https://github.com/mad01/thismoon/issues/174)) ([1e68bf4](https://github.com/mad01/thismoon/commit/1e68bf47c522c671828c2cd05fa5ea5ef20e0d87))

## [0.5.0](https://github.com/mad01/thismoon/compare/worklog/v0.4.0...worklog/v0.5.0) (2026-08-22)


### Features

* agent-facing operating docs across all components (ADR-0009) ([#172](https://github.com/mad01/thismoon/issues/172)) ([d52b48a](https://github.com/mad01/thismoon/commit/d52b48a5983e3c3f3ed8d1d1f30d269c326f5bee))

## [0.4.0](https://github.com/mad01/thismoon/compare/worklog/v0.3.0...worklog/v0.4.0) (2026-08-21)


### Features

* **worklog:** remote upstream support for the store ([#166](https://github.com/mad01/thismoon/issues/166)) ([a822d90](https://github.com/mad01/thismoon/commit/a822d90146fd66c6dd26f1783d39fa696f667b6e))

## [0.3.0](https://github.com/mad01/thismoon/compare/worklog/v0.2.1...worklog/v0.3.0) (2026-08-13)


### Features

* shared buildinfo package — commit/tag/build time in version and /version across all components ([#125](https://github.com/mad01/thismoon/issues/125)) ([3fc9af5](https://github.com/mad01/thismoon/commit/3fc9af5f3014052300da6ab55325190623867d96))
* **worklog:** add config command ([a200f64](https://github.com/mad01/thismoon/commit/a200f64da2a3d1e3019c990d01c13610c83a9e98))

## [0.2.1](https://github.com/mad01/thismoon/compare/worklog/v0.2.0...worklog/v0.2.1) (2026-07-12)


### Bug Fixes

* **worklog:** make checkout roots and repo path markers configurable ([#67](https://github.com/mad01/thismoon/issues/67)) ([a7ba0d7](https://github.com/mad01/thismoon/commit/a7ba0d73ff8eac1bf298f2c7dcb16be0e51487bb))

## [0.2.0](https://github.com/mad01/thismoon/compare/worklog/v0.1.0...worklog/v0.2.0) (2026-07-07)


### Features

* MAD-210 migrate worklog into tools/worklog ([#21](https://github.com/mad01/thismoon/issues/21)) ([242e2fd](https://github.com/mad01/thismoon/commit/242e2fd07e25cbb0ddb686e5f7062c153a4d285f))
