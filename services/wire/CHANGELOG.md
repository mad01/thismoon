# Changelog

## [0.9.0](https://github.com/mad01/thismoon/compare/wire/v0.8.0...wire/v0.9.0) (2026-09-12)


### Features

* annotate MCP tools, gate csl semantic tools on config, bump go-sdk to v1.8.0-pre.2 ([82ec8c4](https://github.com/mad01/thismoon/commit/82ec8c4898885b97945f2dc08edcf6584040814c))
* **wire:** annotate MCP tools with read-only and open-world hints ([9bc7a17](https://github.com/mad01/thismoon/commit/9bc7a179934d4534d7c487b2a86d7dc533d9030a))

## [0.8.0](https://github.com/mad01/thismoon/compare/wire/v0.7.0...wire/v0.8.0) (2026-08-29)


### Features

* **wire:** wire_doctor tool, XDG-aware paths ([6bc442d](https://github.com/mad01/thismoon/commit/6bc442d1295640e2607d16c4c24d1839d8c7d988))

## [0.7.0](https://github.com/mad01/thismoon/compare/wire/v0.6.1...wire/v0.7.0) (2026-08-24)


### Features

* agent-facing operating docs across all components (ADR-0009) ([#172](https://github.com/mad01/thismoon/issues/172)) ([03b3db8](https://github.com/mad01/thismoon/commit/03b3db8887a744910ee4772309cf88a9fcf0ced6))
* doctor contract for serve-bearing components (ADR-0009 wave 2) ([#174](https://github.com/mad01/thismoon/issues/174)) ([1ed2b87](https://github.com/mad01/thismoon/commit/1ed2b87a4c6a97d63382d03b182f0362ed6fc9f1))
* shared buildinfo package — commit/tag/build time in version and /version across all components ([#125](https://github.com/mad01/thismoon/issues/125)) ([6b2ee8b](https://github.com/mad01/thismoon/commit/6b2ee8b29f7ff1180e7f69cc8a3cbe58a8cefe38))
* **wire:** N-agent protocol — join/leave roster, addressed messages, note kind ([#117](https://github.com/mad01/thismoon/issues/117)) ([86eda17](https://github.com/mad01/thismoon/commit/86eda17d03375be7bffb97b78a834d70cae6a24f))
* **wire:** session-to-session message bus for agent handoffs ([#112](https://github.com/mad01/thismoon/issues/112)) ([0614472](https://github.com/mad01/thismoon/commit/06144726c9762c6bda70f6c322138a2c6f9b69be))
* **wire:** surface off-roster obligations as awaiting_reply_off_roster ([#118](https://github.com/mad01/thismoon/issues/118)) ([45d01ea](https://github.com/mad01/thismoon/commit/45d01ea632b68a0a6a7fb7a9dc272f9b5bee850d))
* **wire:** typed message protocol — kind, reply_to, reply_needed, channel conventions ([#115](https://github.com/mad01/thismoon/issues/115)) ([902685e](https://github.com/mad01/thismoon/commit/902685e8812ad27e667b2253a476eea88e66ee54))


### Bug Fixes

* ship MCP field descriptions via the jsonschema tag ([#178](https://github.com/mad01/thismoon/issues/178)) ([d83cfd4](https://github.com/mad01/thismoon/commit/d83cfd4e05f10c4f406f010ce0d01c18fff4afe6))

## [0.6.1](https://github.com/mad01/thismoon/compare/wire/v0.6.0...wire/v0.6.1) (2026-08-22)


### Bug Fixes

* ship MCP field descriptions via the jsonschema tag ([#178](https://github.com/mad01/thismoon/issues/178)) ([c38161e](https://github.com/mad01/thismoon/commit/c38161ed0ecb106ae64b3b9bf02a9039c7879a21))

## [0.6.0](https://github.com/mad01/thismoon/compare/wire/v0.5.0...wire/v0.6.0) (2026-08-22)


### Features

* doctor contract for serve-bearing components (ADR-0009 wave 2) ([#174](https://github.com/mad01/thismoon/issues/174)) ([1e68bf4](https://github.com/mad01/thismoon/commit/1e68bf47c522c671828c2cd05fa5ea5ef20e0d87))

## [0.5.0](https://github.com/mad01/thismoon/compare/wire/v0.4.0...wire/v0.5.0) (2026-08-22)


### Features

* agent-facing operating docs across all components (ADR-0009) ([#172](https://github.com/mad01/thismoon/issues/172)) ([d52b48a](https://github.com/mad01/thismoon/commit/d52b48a5983e3c3f3ed8d1d1f30d269c326f5bee))

## [0.4.0](https://github.com/mad01/thismoon/compare/wire/v0.3.0...wire/v0.4.0) (2026-08-13)


### Features

* shared buildinfo package — commit/tag/build time in version and /version across all components ([#125](https://github.com/mad01/thismoon/issues/125)) ([3fc9af5](https://github.com/mad01/thismoon/commit/3fc9af5f3014052300da6ab55325190623867d96))

## [0.3.0](https://github.com/mad01/thismoon/compare/wire/v0.2.0...wire/v0.3.0) (2026-08-01)


### Features

* **wire:** surface off-roster obligations as awaiting_reply_off_roster ([#118](https://github.com/mad01/thismoon/issues/118)) ([25333eb](https://github.com/mad01/thismoon/commit/25333eb086729fee75cf480b0e065cbc1fc4f4e4))

## [0.2.0](https://github.com/mad01/thismoon/compare/wire/v0.1.0...wire/v0.2.0) (2026-08-01)


### Features

* **wire:** N-agent protocol — join/leave roster, addressed messages, note kind ([#117](https://github.com/mad01/thismoon/issues/117)) ([bdceadc](https://github.com/mad01/thismoon/commit/bdceadcc5e816dd6ed9c21ebd2014ac53c184010))
* **wire:** typed message protocol — kind, reply_to, reply_needed, channel conventions ([#115](https://github.com/mad01/thismoon/issues/115)) ([399e302](https://github.com/mad01/thismoon/commit/399e3028412e24e796ae88c611c9b3c05bfef18b))

## 0.1.0 (2026-07-28)


### Features

* **wire:** session-to-session message bus for agent handoffs ([#112](https://github.com/mad01/thismoon/issues/112)) ([87be5e6](https://github.com/mad01/thismoon/commit/87be5e6fd5ea03edc6c859b625638f4b976aaaed))
