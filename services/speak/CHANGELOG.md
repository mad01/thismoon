# Changelog

## [0.11.1](https://github.com/mad01/thismoon/compare/speak/v0.11.0...speak/v0.11.1) (2026-09-23)


### Bug Fixes

* **speak:** retry stalled synthesis and let the page retry failed parts ([#134](https://github.com/mad01/thismoon/issues/134)) ([1343357](https://github.com/mad01/thismoon/commit/1343357f564791ddc75c29e7e03092a77a787339))

## [0.11.0](https://github.com/mad01/thismoon/compare/speak/v0.10.0...speak/v0.11.0) (2026-09-23)


### Features

* **speak:** prepare uploaded documents' audio ahead of playback ([#132](https://github.com/mad01/thismoon/issues/132)) ([54557c5](https://github.com/mad01/thismoon/commit/54557c5bdcb98e6f10a47586023728590fd90edd))

## [0.10.0](https://github.com/mad01/thismoon/compare/speak/v0.9.0...speak/v0.10.0) (2026-09-23)


### Features

* **speak:** add the Gemini API as a provider ([#130](https://github.com/mad01/thismoon/issues/130)) ([aa799f7](https://github.com/mad01/thismoon/commit/aa799f7297cce6de3a2acefb1970fc92bb8e2555))
* **speak:** choose the TTS provider from a config file ([#128](https://github.com/mad01/thismoon/issues/128)) ([703b6f2](https://github.com/mad01/thismoon/commit/703b6f274cb977b2eb95f02dd17f4a37059b602a))
* **speak:** report TTS failures with their reason on every surface ([#127](https://github.com/mad01/thismoon/issues/127)) ([2a108f2](https://github.com/mad01/thismoon/commit/2a108f2cfdb1b6bc710b8661c9b078d76ba7c8da))


### Bug Fixes

* **speak:** probe serve on 127.0.0.1 and report unreadable files ([#131](https://github.com/mad01/thismoon/issues/131)) ([801fb83](https://github.com/mad01/thismoon/commit/801fb8367b2234d6efd9c5eb9c786452cb7f3f86))

## [0.9.0](https://github.com/mad01/thismoon/compare/speak/v0.8.1...speak/v0.9.0) (2026-09-12)


### Features

* annotate MCP tools, gate csl semantic tools on config, bump go-sdk to v1.8.0-pre.2 ([82ec8c4](https://github.com/mad01/thismoon/commit/82ec8c4898885b97945f2dc08edcf6584040814c))
* **speak:** annotate MCP tools with read-only and open-world hints ([4275ae0](https://github.com/mad01/thismoon/commit/4275ae050a8c676cb5bd90a9747c7bce3620634a))

## [0.8.1](https://github.com/mad01/thismoon/compare/speak/v0.8.0...speak/v0.8.1) (2026-08-29)


### Bug Fixes

* **speak:** probe audio dir for legacy state, skip absent state dir in doctor ([d6a698c](https://github.com/mad01/thismoon/commit/d6a698caa891e95031fe8d8257202055208d16c1))

## [0.8.0](https://github.com/mad01/thismoon/compare/speak/v0.7.1...speak/v0.8.0) (2026-08-29)


### Features

* **speak:** CORS allowlist, persistent flags, reachable state dir ([743fbb7](https://github.com/mad01/thismoon/commit/743fbb771c9aca5f5bd03ea987909efbc25d55b4))

## [0.7.1](https://github.com/mad01/thismoon/compare/speak/v0.7.0...speak/v0.7.1) (2026-08-27)


### Bug Fixes

* **humanizer:** make dead vale rules fire, add dash-substitute rule, align CLI json with MCP payload ([13fd610](https://github.com/mad01/thismoon/commit/13fd6103fca45c7a0a4c90e81d1e96b3fb73a46f))

## [0.7.0](https://github.com/mad01/thismoon/compare/speak/v0.6.0...speak/v0.7.0) (2026-08-27)


### Features

* **speak:** adopt the fixation rename and the webkit feature guide ([a2917cc](https://github.com/mad01/thismoon/commit/a2917cc096e3a128a824e3cb218e136b01808cee))

## [0.6.0](https://github.com/mad01/thismoon/compare/speak/v0.5.1...speak/v0.6.0) (2026-08-24)


### Features

* agent-facing operating docs across all components (ADR-0009) ([#172](https://github.com/mad01/thismoon/issues/172)) ([03b3db8](https://github.com/mad01/thismoon/commit/03b3db8887a744910ee4772309cf88a9fcf0ced6))
* doctor contract for serve-bearing components (ADR-0009 wave 2) ([#174](https://github.com/mad01/thismoon/issues/174)) ([1ed2b87](https://github.com/mad01/thismoon/commit/1ed2b87a4c6a97d63382d03b182f0362ed6fc9f1))
* shared buildinfo package — commit/tag/build time in version and /version across all components ([#125](https://github.com/mad01/thismoon/issues/125)) ([6b2ee8b](https://github.com/mad01/thismoon/commit/6b2ee8b29f7ff1180e7f69cc8a3cbe58a8cefe38))
* **speak:** add MCP server with server-side read-aloud playback ([#33](https://github.com/mad01/thismoon/issues/33)) ([e61cb07](https://github.com/mad01/thismoon/commit/e61cb0734600d1a5a3246de27bca24278b72fd54))
* **speak:** migrate speak into services/speak + recipe ([#11](https://github.com/mad01/thismoon/issues/11)) ([36313df](https://github.com/mad01/thismoon/commit/36313dfd80518a1a321ada61b8c3aa803e58e5e1))


### Bug Fixes

* ship MCP field descriptions via the jsonschema tag ([#178](https://github.com/mad01/thismoon/issues/178)) ([d83cfd4](https://github.com/mad01/thismoon/commit/d83cfd4e05f10c4f406f010ce0d01c18fff4afe6))

## [0.5.1](https://github.com/mad01/thismoon/compare/speak/v0.5.0...speak/v0.5.1) (2026-08-22)


### Bug Fixes

* ship MCP field descriptions via the jsonschema tag ([#178](https://github.com/mad01/thismoon/issues/178)) ([c38161e](https://github.com/mad01/thismoon/commit/c38161ed0ecb106ae64b3b9bf02a9039c7879a21))

## [0.5.0](https://github.com/mad01/thismoon/compare/speak/v0.4.0...speak/v0.5.0) (2026-08-22)


### Features

* doctor contract for serve-bearing components (ADR-0009 wave 2) ([#174](https://github.com/mad01/thismoon/issues/174)) ([1e68bf4](https://github.com/mad01/thismoon/commit/1e68bf47c522c671828c2cd05fa5ea5ef20e0d87))

## [0.4.0](https://github.com/mad01/thismoon/compare/speak/v0.3.0...speak/v0.4.0) (2026-08-22)


### Features

* agent-facing operating docs across all components (ADR-0009) ([#172](https://github.com/mad01/thismoon/issues/172)) ([d52b48a](https://github.com/mad01/thismoon/commit/d52b48a5983e3c3f3ed8d1d1f30d269c326f5bee))

## [0.3.0](https://github.com/mad01/thismoon/compare/speak/v0.2.0...speak/v0.3.0) (2026-08-13)


### Features

* shared buildinfo package — commit/tag/build time in version and /version across all components ([#125](https://github.com/mad01/thismoon/issues/125)) ([3fc9af5](https://github.com/mad01/thismoon/commit/3fc9af5f3014052300da6ab55325190623867d96))

## [0.2.0](https://github.com/mad01/thismoon/compare/speak/v0.1.0...speak/v0.2.0) (2026-07-08)


### Features

* **speak:** add MCP server with server-side read-aloud playback ([#33](https://github.com/mad01/thismoon/issues/33)) ([1b8af01](https://github.com/mad01/thismoon/commit/1b8af01ac9424a6994e201e90dd5ff2b6793ad78))

## 0.1.0 (2026-07-06)


### Features

* **speak:** migrate speak into services/speak + recipe ([#11](https://github.com/mad01/thismoon/issues/11)) ([5d0afd7](https://github.com/mad01/thismoon/commit/5d0afd73b6d9995c9517035dcd2f19fd26de89e4))
