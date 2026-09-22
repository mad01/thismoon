# Changelog

## [1.4.0](https://github.com/mad01/thismoon/compare/present/v1.3.0...present/v1.4.0) (2026-09-22)


### Features

* **present:** container image, Kubernetes manifests, and a kind job in CI ([32734c0](https://github.com/mad01/thismoon/commit/32734c0e546e2579f0e1be9e705e93b3cea53d77))
* **present:** Kubernetes page store with expiry sweeper ([782421a](https://github.com/mad01/thismoon/commit/782421a2aa6e51ee6133877c572a112d54a4da84))
* **present:** share a local page to the shared instance ([fab5f12](https://github.com/mad01/thismoon/commit/fab5f120abc53995af90d363a1fe66cfc21e01d1))
* **present:** shared mode with author keys and MCP over HTTP ([6226b68](https://github.com/mad01/thismoon/commit/6226b68bf18e4e321f8253ee425f44be3299de94))


### Bug Fixes

* **present:** harden shared mode after the kind validation ([29f48a6](https://github.com/mad01/thismoon/commit/29f48a6e3a42aba59c7d8c7101055db60c5888d8))

## [1.3.0](https://github.com/mad01/thismoon/compare/present/v1.2.0...present/v1.3.0) (2026-09-15)


### Features

* **present:** animate edge flow by weight and add six chart kinds ([#98](https://github.com/mad01/thismoon/issues/98)) ([d088e34](https://github.com/mad01/thismoon/commit/d088e34a7c66060274ec5e44f714026a42ca8b1e))

## [1.2.0](https://github.com/mad01/thismoon/compare/present/v1.1.1...present/v1.2.0) (2026-09-12)


### Features

* annotate MCP tools, gate csl semantic tools on config, bump go-sdk to v1.8.0-pre.2 ([82ec8c4](https://github.com/mad01/thismoon/commit/82ec8c4898885b97945f2dc08edcf6584040814c))
* **present:** annotate MCP tools with read-only and open-world hints ([1a6f81a](https://github.com/mad01/thismoon/commit/1a6f81a7a20d1185bef4e950da4e8e2f60bc2513))

## [1.1.1](https://github.com/mad01/thismoon/compare/present/v1.1.0...present/v1.1.1) (2026-08-29)


### Bug Fixes

* **present:** probe pages dir for legacy state detection ([f7d241a](https://github.com/mad01/thismoon/commit/f7d241a840f6594699b8cb012247d11cc77c6f08))

## [1.1.0](https://github.com/mad01/thismoon/compare/present/v1.0.0...present/v1.1.0) (2026-08-29)


### Features

* **present:** state-dir page store, present_doctor tool ([fd62220](https://github.com/mad01/thismoon/commit/fd62220973497bee7993d701c424de641a4fd5bf))

## [1.0.0](https://github.com/mad01/thismoon/compare/present/v0.6.0...present/v1.0.0) (2026-08-27)


### ⚠ BREAKING CHANGES

* **webkit,present:** the <wk-header> control token and targets attribute are now `fixation` and `fixation-targets`; the rendered marker attribute is `data-fixation` (run `present rerender` to migrate already-stored pages); the localStorage key is `webkit-fixation` (per-browser toggle state resets, harmless); the public export is `toFixation` (was `toBionic`) and the source file is `webkit/src/fixation.ts`.

### Features

* **webkit,present:** rename reading feature to "fixation", add a ? feature guide ([a88f586](https://github.com/mad01/thismoon/commit/a88f58634b650e3fdffe94b1be0af9e86996c047))

## [0.6.0](https://github.com/mad01/thismoon/compare/present/v0.5.1...present/v0.6.0) (2026-08-24)


### Features

* agent-facing operating docs across all components (ADR-0009) ([#172](https://github.com/mad01/thismoon/issues/172)) ([03b3db8](https://github.com/mad01/thismoon/commit/03b3db8887a744910ee4772309cf88a9fcf0ced6))
* doctor contract for serve-bearing components (ADR-0009 wave 2) ([#174](https://github.com/mad01/thismoon/issues/174)) ([1ed2b87](https://github.com/mad01/thismoon/commit/1ed2b87a4c6a97d63382d03b182f0362ed6fc9f1))
* migrate present, status, pr, deps, events from dotfiles ([6da5bc2](https://github.com/mad01/thismoon/commit/6da5bc2e982e14e31aa76eaf0e71bcf1249ba270))
* **present:** import present service from dotfiles as services/present ([16b1ab1](https://github.com/mad01/thismoon/commit/16b1ab10156d2f73ce41cf18da66975361c9ab8f))
* **recipes:** ship worklog, humanizer, golang-style, and present skills from repo-root skills/ ([#136](https://github.com/mad01/thismoon/issues/136)) ([1d58911](https://github.com/mad01/thismoon/commit/1d58911ddd34b03b137f660add00487bfe1cd8a7))
* shared buildinfo package — commit/tag/build time in version and /version across all components ([#125](https://github.com/mad01/thismoon/issues/125)) ([6b2ee8b](https://github.com/mad01/thismoon/commit/6b2ee8b29f7ff1180e7f69cc8a3cbe58a8cefe38))


### Bug Fixes

* **present:** apply fixation reading to list items ([#120](https://github.com/mad01/thismoon/issues/120)) ([89c14a1](https://github.com/mad01/thismoon/commit/89c14a1f725d5d3421f35dafc676842de5a4f77c))
* **present:** correct stale --help text, remove dead server-side render layer ([#29](https://github.com/mad01/thismoon/issues/29)) ([8abc1f7](https://github.com/mad01/thismoon/commit/8abc1f7d53763077c7da6b8b74db814ff2901fba))
* ship MCP field descriptions via the jsonschema tag ([#178](https://github.com/mad01/thismoon/issues/178)) ([d83cfd4](https://github.com/mad01/thismoon/commit/d83cfd4e05f10c4f406f010ce0d01c18fff4afe6))

## [0.5.1](https://github.com/mad01/thismoon/compare/present/v0.5.0...present/v0.5.1) (2026-08-22)


### Bug Fixes

* ship MCP field descriptions via the jsonschema tag ([#178](https://github.com/mad01/thismoon/issues/178)) ([c38161e](https://github.com/mad01/thismoon/commit/c38161ed0ecb106ae64b3b9bf02a9039c7879a21))

## [0.5.0](https://github.com/mad01/thismoon/compare/present/v0.4.0...present/v0.5.0) (2026-08-22)


### Features

* doctor contract for serve-bearing components (ADR-0009 wave 2) ([#174](https://github.com/mad01/thismoon/issues/174)) ([1e68bf4](https://github.com/mad01/thismoon/commit/1e68bf47c522c671828c2cd05fa5ea5ef20e0d87))

## [0.4.0](https://github.com/mad01/thismoon/compare/present/v0.3.0...present/v0.4.0) (2026-08-22)


### Features

* agent-facing operating docs across all components (ADR-0009) ([#172](https://github.com/mad01/thismoon/issues/172)) ([d52b48a](https://github.com/mad01/thismoon/commit/d52b48a5983e3c3f3ed8d1d1f30d269c326f5bee))

## [0.3.0](https://github.com/mad01/thismoon/compare/present/v0.2.0...present/v0.3.0) (2026-08-17)


### Features

* **recipes:** ship worklog, humanizer, golang-style, and present skills from repo-root skills/ ([#136](https://github.com/mad01/thismoon/issues/136)) ([c21beb9](https://github.com/mad01/thismoon/commit/c21beb98a289f5d1410e633c7beded856c38c6c7))

## [0.2.0](https://github.com/mad01/thismoon/compare/present/v0.1.2...present/v0.2.0) (2026-08-13)


### Features

* shared buildinfo package — commit/tag/build time in version and /version across all components ([#125](https://github.com/mad01/thismoon/issues/125)) ([3fc9af5](https://github.com/mad01/thismoon/commit/3fc9af5f3014052300da6ab55325190623867d96))

## [0.1.2](https://github.com/mad01/thismoon/compare/present/v0.1.1...present/v0.1.2) (2026-08-11)


### Bug Fixes

* **present:** apply fixation reading to list items ([#120](https://github.com/mad01/thismoon/issues/120)) ([785831a](https://github.com/mad01/thismoon/commit/785831a9d84f8d3bfa9847024be8b419c34cb215))

## [0.1.1](https://github.com/mad01/thismoon/compare/present/v0.1.0...present/v0.1.1) (2026-07-08)


### Bug Fixes

* **present:** correct stale --help text, remove dead server-side render layer ([#29](https://github.com/mad01/thismoon/issues/29)) ([36ae778](https://github.com/mad01/thismoon/commit/36ae778289933ba4552c872ef8b3974a9f8d3dcf))

## 0.1.0 (2026-07-06)


### Features

* migrate present, status, pr, deps, events from dotfiles ([29163ed](https://github.com/mad01/thismoon/commit/29163ed52a19d03cbc384c0e0187efa6724da1dd))
* **present:** import present service from dotfiles as services/present ([8e40d0b](https://github.com/mad01/thismoon/commit/8e40d0b11ceb83c3281f9191e7c023d612799015))
