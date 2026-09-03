# Changelog

## [0.11.0](https://github.com/mad01/thismoon/compare/humanizer/v0.10.0...humanizer/v0.11.0) (2026-09-02)


### Features

* **humanizer:** add litellm judge backend for OpenAI-compatible proxies (MAD-342) ([#64](https://github.com/mad01/thismoon/issues/64)) ([18f1426](https://github.com/mad01/thismoon/commit/18f1426a3470453a01db428a9d12a7530c27ae67))

## [0.10.0](https://github.com/mad01/thismoon/compare/humanizer/v0.9.0...humanizer/v0.10.0) (2026-09-01)


### Features

* **humanizer:** add humanizer_scan_go MCP tool (MAD-336) ([#63](https://github.com/mad01/thismoon/issues/63)) ([ec488ba](https://github.com/mad01/thismoon/commit/ec488ba3e941e9bf5bba564f01adcde2cccbc873))
* **humanizer:** add scan --go for Go AST prose extraction (MAD-335) ([#60](https://github.com/mad01/thismoon/issues/60)) ([f13e7c7](https://github.com/mad01/thismoon/commit/f13e7c7343920033b889caee47c3cec36552cf39))


### Bug Fixes

* **humanizer:** calibrate judge rubric for confidence spread (MAD-348) ([#62](https://github.com/mad01/thismoon/issues/62)) ([03e931f](https://github.com/mad01/thismoon/commit/03e931f63acaa48a4de382ad8608bd50e5b551ea))
* **humanizer:** exclude fenced code and double hyphens from em-dash density (MAD-347) ([#59](https://github.com/mad01/thismoon/issues/59)) ([f211ef7](https://github.com/mad01/thismoon/commit/f211ef7c8145a6e9f8a3a34f335e7aa1dfad6996))

## [0.9.0](https://github.com/mad01/thismoon/compare/humanizer/v0.8.0...humanizer/v0.9.0) (2026-09-01)


### Features

* **humanizer:** add LLM judge with pluggable OpenRouter backend (MAD-342) ([4b72402](https://github.com/mad01/thismoon/commit/4b72402cbf3dad154cca89af07fce566af515fd3))

## [0.8.0](https://github.com/mad01/thismoon/compare/humanizer/v0.7.0...humanizer/v0.8.0) (2026-08-31)


### Features

* **humanizer:** add StoryScope narrative rubric and fiction span rules ([45f3dd8](https://github.com/mad01/thismoon/commit/45f3dd89356f870ca23a0e5b8a4b7335f35d6a7f))

## [0.7.0](https://github.com/mad01/thismoon/compare/humanizer/v0.6.2...humanizer/v0.7.0) (2026-08-28)


### Features

* **humanizer:** expand detection with 2025-era tells and em-dash density check ([3cf1e16](https://github.com/mad01/thismoon/commit/3cf1e16796330d09b618fd66c4e745a8c0487979))

## [0.6.2](https://github.com/mad01/thismoon/compare/humanizer/v0.6.1...humanizer/v0.6.2) (2026-08-27)


### Bug Fixes

* **humanizer:** make dead vale rules fire, add dash-substitute rule, align CLI json with MCP payload ([13fd610](https://github.com/mad01/thismoon/commit/13fd6103fca45c7a0a4c90e81d1e96b3fb73a46f))
* **humanizer:** make dead vale rules fire, add dash-substitute rule, align CLI json with MCP payload ([8ba8c73](https://github.com/mad01/thismoon/commit/8ba8c736b66d444e1f374170499841ec66f231c7))

## [0.6.1](https://github.com/mad01/thismoon/compare/humanizer/v0.6.0...humanizer/v0.6.1) (2026-08-25)


### Bug Fixes

* **humanizer:** structured use_instead redirect on detect_file sandbox denial (MAD-302) ([d1cb5a1](https://github.com/mad01/thismoon/commit/d1cb5a1dee5728420cafc3ca8c2a2d79bff7daa4))
* MCP tool-call reliability fixes from the 30-day session audit (MAD-300) ([bdd66e4](https://github.com/mad01/thismoon/commit/bdd66e4e34306ad3180a5c2f5ddc06141aa9fb42))

## [0.6.0](https://github.com/mad01/thismoon/compare/humanizer/v0.5.0...humanizer/v0.6.0) (2026-08-22)


### Features

* doctor contract for serve-bearing components (ADR-0009 wave 2) ([#174](https://github.com/mad01/thismoon/issues/174)) ([1e68bf4](https://github.com/mad01/thismoon/commit/1e68bf47c522c671828c2cd05fa5ea5ef20e0d87))

## [0.5.0](https://github.com/mad01/thismoon/compare/humanizer/v0.4.0...humanizer/v0.5.0) (2026-08-22)


### Features

* agent-facing operating docs across all components (ADR-0009) ([#172](https://github.com/mad01/thismoon/issues/172)) ([d52b48a](https://github.com/mad01/thismoon/commit/d52b48a5983e3c3f3ed8d1d1f30d269c326f5bee))

## [0.4.0](https://github.com/mad01/thismoon/compare/humanizer/v0.3.0...humanizer/v0.4.0) (2026-08-14)


### Features

* **humanizer:** add watermark lint/fix/rewrite and ChatGPT-artifact rules ([#130](https://github.com/mad01/thismoon/issues/130)) ([b93d5b6](https://github.com/mad01/thismoon/commit/b93d5b65d0f4cea472e14a1a0ffbcb1da7be22b9))

## [0.3.0](https://github.com/mad01/thismoon/compare/humanizer/v0.2.1...humanizer/v0.3.0) (2026-08-13)


### Features

* shared buildinfo package — commit/tag/build time in version and /version across all components ([#125](https://github.com/mad01/thismoon/issues/125)) ([3fc9af5](https://github.com/mad01/thismoon/commit/3fc9af5f3014052300da6ab55325190623867d96))

## [0.2.1](https://github.com/mad01/thismoon/compare/humanizer/v0.2.0...humanizer/v0.2.1) (2026-07-12)


### Bug Fixes

* **humanizer:** stop enumerating machine-specific sandbox roots in messages ([#68](https://github.com/mad01/thismoon/issues/68)) ([5228d9a](https://github.com/mad01/thismoon/commit/5228d9a284828bbe41d0c45defc6ad80dab34d1e))

## [0.2.0](https://github.com/mad01/thismoon/compare/humanizer/v0.1.0...humanizer/v0.2.0) (2026-07-07)


### Features

* MAD-209 migrate humanizer into tools/humanizer ([#20](https://github.com/mad01/thismoon/issues/20)) ([f3cc6fa](https://github.com/mad01/thismoon/commit/f3cc6fabee1f731ff904ddcbf45022c1c5f0227e))
