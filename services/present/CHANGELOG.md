# Changelog

## [1.16.0](https://github.com/mad01/thismoon/compare/present/v1.15.0...present/v1.16.0) (2026-10-07)


### Features

* **present:** diagram block on elk and d3, frameless charts ([#203](https://github.com/mad01/thismoon/issues/203)) ([2b7b84b](https://github.com/mad01/thismoon/commit/2b7b84b04258462b8dd5d4b3f480f9661badd204))
* **present:** ribbon chart kind on d3 with per-period steps ([#202](https://github.com/mad01/thismoon/issues/202)) ([0632bad](https://github.com/mad01/thismoon/commit/0632bad9d3c0920679f8b401ece01b278ce1987e))
* **present:** shared viz layer: chart steps on deck slides, viz.js, chart kind validation, d3 ([#200](https://github.com/mad01/thismoon/issues/200)) ([69430f9](https://github.com/mad01/thismoon/commit/69430f9c2d61ee5fdae286de146c7e6246d071fe))
* **present:** slides draw no filled boxes ([#205](https://github.com/mad01/thismoon/issues/205)) ([7c6002d](https://github.com/mad01/thismoon/commit/7c6002d1f60e9339df4c4b547626a1173e9de44d))
* **present:** the skill and tool descriptions are the entry point for pages and decks ([#206](https://github.com/mad01/thismoon/issues/206)) ([c50468e](https://github.com/mad01/thismoon/commit/c50468e91637d456bd7aac5bea4813c738f860eb))


### Bug Fixes

* **present:** diagram layer gap reaches the nodes inside groups ([#204](https://github.com/mad01/thismoon/issues/204)) ([7d5f53f](https://github.com/mad01/thismoon/commit/7d5f53f8acd1ebc6956b63c495ff66cbaf969d7e))

## [1.15.0](https://github.com/mad01/thismoon/compare/present/v1.14.0...present/v1.15.0) (2026-10-02)


### Features

* **present:** remove the section id badge ([#190](https://github.com/mad01/thismoon/issues/190)) ([a8cda83](https://github.com/mad01/thismoon/commit/a8cda833ed61964886a3a56e133127920e3bf249))


### Bug Fixes

* **present,webkit:** retire the terracotta alias and lift dark semantic colours under 3:1 ([00fafd0](https://github.com/mad01/thismoon/commit/00fafd03a1306752815c12aa1d2cc7ad3dcd6131))

## [1.14.0](https://github.com/mad01/thismoon/compare/present/v1.13.0...present/v1.14.0) (2026-10-02)


### Features

* **present,webkit:** graph beside prose in columns, and an image block with local-file sources ([3aa6f41](https://github.com/mad01/thismoon/commit/3aa6f41f5af94ce71d26579ff3c434392b509814))
* **present:** image block with URL and local-file sources ([ec8896c](https://github.com/mad01/thismoon/commit/ec8896c41954d4fc404fea7993ee152f5b057557))
* **present:** the graph may sit in a column ([e32b956](https://github.com/mad01/thismoon/commit/e32b956605dbbd5916445d2513200f7d40020449))


### Bug Fixes

* **present:** a reused stored image is touched and concurrent sweeps tolerate each other ([a2c191b](https://github.com/mad01/thismoon/commit/a2c191bd5f16be070bb5fc9d30aab24539d64df6))
* **present:** image sweep keeps reused files and runs at startup ([678c14b](https://github.com/mad01/thismoon/commit/678c14bd5a352649be06a8f327711f0aff90e4df))
* **present:** stop prescribing section id badges and drop them from deck slides ([#180](https://github.com/mad01/thismoon/issues/180)) ([45feb7b](https://github.com/mad01/thismoon/commit/45feb7b13754c24c12d26b5b1d917af8d4b0c05d))

## [1.13.0](https://github.com/mad01/thismoon/compare/present/v1.12.0...present/v1.13.0) (2026-10-01)


### Features

* **present:** columns, stat, quote, and details blocks plus deck chrome fields ([fdf9071](https://github.com/mad01/thismoon/commit/fdf9071bc441c7228cc1ac98285593e298b3ed36))
* **present:** deck chrome strip with logo, progress marker, and footer ([7d3eeba](https://github.com/mad01/thismoon/commit/7d3eeba417c377552bb332cabc3e0c39237d9fd6))
* **present:** open decks with the read-aloud controls hidden ([d8abbe2](https://github.com/mad01/thismoon/commit/d8abbe2e7c57f79ad81d382b0d1a946550315fa4))
* **present:** section layout, tone, notes, and reveal, and the deck transition ([bcff794](https://github.com/mad01/thismoon/commit/bcff7945abff925b935d7b79c50e3f6b4dd24bca))
* **present:** slide layouts, notes drawer, reveal steps, and view transitions in the deck ([a0d19c7](https://github.com/mad01/thismoon/commit/a0d19c70a223674f0b0d968aa16251b32738931e))


### Bug Fixes

* **present:** a link in the speaker notes opens a new tab ([db706c1](https://github.com/mad01/thismoon/commit/db706c1885ce691d44e305eca6fc47fade72823a))
* **present:** a progress dot gives up focus after it jumps ([c19a6f8](https://github.com/mad01/thismoon/commit/c19a6f83aa21876756db2bca6774ed4495363c69))
* **present:** a stat's value is shown verbatim ([622cdbd](https://github.com/mad01/thismoon/commit/622cdbdeea3a904c805e18baf84a2d81117d8cf5))
* **present:** only the live chrome paints during a crossfade, and a skipped transition stays quiet ([cb158e5](https://github.com/mad01/thismoon/commit/cb158e5fb3b213bf20fa1babe537c7d2e557814e))
* **present:** Space on a deck bar button is that button's click again ([98e75a5](https://github.com/mad01/thismoon/commit/98e75a53e18e42f0add45bbea2128e0008aabcac))
* **present:** the chart draw-in follows the deck's transition ([c75e9af](https://github.com/mad01/thismoon/commit/c75e9af54646562a3809aea21a8fb1f41e86e009))
* **present:** the chrome island and the presenter byline belong to the deck rendition ([6ce085c](https://github.com/mad01/thismoon/commit/6ce085c5cf709fdcb3afc60330ede748c8813c25))
* **present:** the chrome stays painted through a slide transition ([1577227](https://github.com/mad01/thismoon/commit/157722701ce627e595735d4fbb705766e4248f33))
* **present:** the deck knows where it is going while a transition is pending ([99653f7](https://github.com/mad01/thismoon/commit/99653f79e9699bacf449214e5eef82d69a27fd30))

## [1.12.0](https://github.com/mad01/thismoon/compare/present/v1.11.0...present/v1.12.0) (2026-10-01)


### Features

* **present:** read graph and chart colours from the palette roles ([22f16c8](https://github.com/mad01/thismoon/commit/22f16c8a6bc85c9320c6d247c4a87cd086696ed2))

## [1.11.0](https://github.com/mad01/thismoon/compare/present/v1.10.0...present/v1.11.0) (2026-09-30)


### Features

* **present:** build the graph style in app.js, not in the stored script ([190c54e](https://github.com/mad01/thismoon/commit/190c54e4d1dd9a244a1027dfd8a118c90a6cfddb))
* **present:** re-render stored pages in the background when serve starts ([7177931](https://github.com/mad01/thismoon/commit/71779317306568a5e8b3fe2d1d1d953f38ead557))
* **present:** style graphs in app.js and re-render pages when serve starts ([09aafd3](https://github.com/mad01/thismoon/commit/09aafd3deae51617800617563fa483a3de462cad))


### Bug Fixes

* **present:** render inline markup nested inside bold ([0a1deac](https://github.com/mad01/thismoon/commit/0a1deac206d7710be8b6c10bfa3e81f30218629f))
* **present:** render inline markup nested inside bold ([b2953bd](https://github.com/mad01/thismoon/commit/b2953bd9df866fcf8a0593fbcda67eee1a30f631))


### Reverts

* **present:** drop the label-aware layout and edge routing ([#168](https://github.com/mad01/thismoon/issues/168)) ([4afff66](https://github.com/mad01/thismoon/commit/4afff660a2a1e015544ef3b3e90e04721783d1be))

## [1.10.0](https://github.com/mad01/thismoon/compare/present/v1.9.1...present/v1.10.0) (2026-09-30)


### Features

* **present:** node tones and label-aware graph layout ([#164](https://github.com/mad01/thismoon/issues/164)) ([ad13a68](https://github.com/mad01/thismoon/commit/ad13a6808aa662f3385f253b20e178921ceb8299))


### Bug Fixes

* **present:** fixation on titles, toc, and tables; softer body ink ([843e99c](https://github.com/mad01/thismoon/commit/843e99cce06423f31da356d7d3c933ad8b480527))
* **present:** fixation on titles, toc, and tables; softer body ink ([370e31d](https://github.com/mad01/thismoon/commit/370e31dde0438ce49d32177eb0b2198b95007cda))

## [1.9.1](https://github.com/mad01/thismoon/compare/present/v1.9.0...present/v1.9.1) (2026-09-30)


### Bug Fixes

* **present:** grow graph nodes to fit their labels ([3c19bfb](https://github.com/mad01/thismoon/commit/3c19bfb738110f3aefbde52a4e82e669c6f04920))

## [1.9.0](https://github.com/mad01/thismoon/compare/present/v1.8.0...present/v1.9.0) (2026-09-30)


### Features

* **present:** add slide decks beside briefs ([09f8868](https://github.com/mad01/thismoon/commit/09f8868013dd39b1a30ad900445df25300a8ce12))

## [1.8.0](https://github.com/mad01/thismoon/compare/present/v1.7.0...present/v1.8.0) (2026-09-28)


### Features

* **present:** ELK graph layouts with an engine cycle control ([#144](https://github.com/mad01/thismoon/issues/144)) ([9bb9695](https://github.com/mad01/thismoon/commit/9bb96950ee3bd3294faf1c257d4e8c7c5f98f447))

## [1.7.0](https://github.com/mad01/thismoon/compare/present/v1.6.0...present/v1.7.0) (2026-09-28)


### Features

* **present:** render fenced mermaid flowcharts as the page graph ([#141](https://github.com/mad01/thismoon/issues/141)) ([fe171f4](https://github.com/mad01/thismoon/commit/fe171f4b2c77553ad1cf819479ff7d0138f58d14))

## [1.6.0](https://github.com/mad01/thismoon/compare/present/v1.5.0...present/v1.6.0) (2026-09-24)


### Features

* present reads pages aloud through speak's prepared-document flow ([52a8011](https://github.com/mad01/thismoon/commit/52a80113c022497eb72bff90aaadbf15838f131f))
* **present:** import a markdown file as a page ([#136](https://github.com/mad01/thismoon/issues/136)) ([5a92757](https://github.com/mad01/thismoon/commit/5a92757ca602bba8416913b30488328933ce7518))
* **present:** prepare read-aloud audio through speak and make the speak URL a setting ([0d36752](https://github.com/mad01/thismoon/commit/0d367528f6c83c1033f7990e82353db1339228e7))

## [1.5.0](https://github.com/mad01/thismoon/compare/present/v1.4.1...present/v1.5.0) (2026-09-23)


### Features

* **present:** push live reload over server-sent events ([#125](https://github.com/mad01/thismoon/issues/125)) ([fb8a8cf](https://github.com/mad01/thismoon/commit/fb8a8cf0d0f435d965e8efade09d48243345c015))
* **present:** serve version polls from an informer page cache ([#123](https://github.com/mad01/thismoon/issues/123)) ([bdf1118](https://github.com/mad01/thismoon/commit/bdf1118d22c7e3569b15ae5c5bf4edf72e0776b6))

## [1.4.1](https://github.com/mad01/thismoon/compare/present/v1.4.0...present/v1.4.1) (2026-09-23)


### Bug Fixes

* **present:** leave the ⌘K picker out of the shared chrome ([51df5c5](https://github.com/mad01/thismoon/commit/51df5c528cde189314d117aeb659af1a3639f940))

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
