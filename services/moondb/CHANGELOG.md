# Changelog

## [0.4.0](https://github.com/mad01/thismoon/compare/moondb/v0.3.0...moondb/v0.4.0) (2026-10-26)


### Features

* **moondb,belt:** name a checkout's files other sessions left uncommitted ([#311](https://github.com/mad01/thismoon/issues/311)) ([68ee61a](https://github.com/mad01/thismoon/commit/68ee61af48277c505890b5286e9922a27b9f1adf))
* **moondb:** add a session scorecard ([#312](https://github.com/mad01/thismoon/issues/312)) ([fca139b](https://github.com/mad01/thismoon/commit/fca139b5eca47d34efe7e7f9675c94c8d0889d35))


### Bug Fixes

* **moondb:** list a path's history across every checkout by default ([#306](https://github.com/mad01/thismoon/issues/306)) ([8314dbb](https://github.com/mad01/thismoon/commit/8314dbb169258144df9ea711c894ab46e8bcd7b0))

## [0.3.0](https://github.com/mad01/thismoon/compare/moondb/v0.2.0...moondb/v0.3.0) (2026-10-26)


### Features

* **moondb:** attribute session cost to repos ([#303](https://github.com/mad01/thismoon/issues/303)) ([3770891](https://github.com/mad01/thismoon/commit/3770891eeba913342c655179af10b2eb22e3a37e))
* **moondb:** show active time beside the wall span for sessions ([#302](https://github.com/mad01/thismoon/issues/302)) ([96ad51b](https://github.com/mad01/thismoon/commit/96ad51ba8540adbf6ce705a27a8874efae9c4c2d))
* **moondb:** trace subagents from their own transcripts and price them ([#298](https://github.com/mad01/thismoon/issues/298)) ([a1f654f](https://github.com/mad01/thismoon/commit/a1f654fbf6fc8f278ab711bfc9edbb060deceb48))


### Bug Fixes

* **moondb:** classify a long gap inside a turn as idle ([#305](https://github.com/mad01/thismoon/issues/305)) ([358d680](https://github.com/mad01/thismoon/commit/358d680cb057d9374101ac5e5fa7d59038b6d10b))
* **moondb:** walk the session trace in time order ([#304](https://github.com/mad01/thismoon/issues/304)) ([ef26ac3](https://github.com/mad01/thismoon/commit/ef26ac3ecfbf86b6b2287959fd7fbfcd160dee23))

## [0.2.0](https://github.com/mad01/thismoon/compare/moondb/v0.1.0...moondb/v0.2.0) (2026-10-25)


### Features

* **moondb:** session list as a landing page with stored turn totals ([#286](https://github.com/mad01/thismoon/issues/286)) ([c118ee0](https://github.com/mad01/thismoon/commit/c118ee0f6c5755e4cd0b903c4091cf6bd0c8df95))


### Bug Fixes

* **moondb:** cache only complete locations, keep symlink walks for existing paths ([#289](https://github.com/mad01/thismoon/issues/289)) ([b483a17](https://github.com/mad01/thismoon/commit/b483a174a1b29ebe011d54a425b0d562d332ce9d))
* **moondb:** skip Bash-changed files a command left clean ([#288](https://github.com/mad01/thismoon/issues/288)) ([4e96a3f](https://github.com/mad01/thismoon/commit/4e96a3fc994566369543d647cbd0db9ce3593e09))

## 0.1.0 (2026-10-25)


### Features

* **moondb:** import transcripts, derive edits from the tool input ([#279](https://github.com/mad01/thismoon/issues/279)) ([ee6fdae](https://github.com/mad01/thismoon/commit/ee6fdaeac82840374419758094e1e8097a4e8adc))
* **moondb:** record edits between commits and answer why a line changed ([#273](https://github.com/mad01/thismoon/issues/273)) ([d72e4b5](https://github.com/mad01/thismoon/commit/d72e4b569866e931a919990a6153b5323bc17e1e))
* **moondb:** snapshot worktrees at turn edges and capture Bash edits ([#281](https://github.com/mad01/thismoon/issues/281)) ([af85608](https://github.com/mad01/thismoon/commit/af85608dedd8277a73bdef4437a863d58d5b194a))
* **moondb:** trace a session's time and cost from its transcripts ([#282](https://github.com/mad01/thismoon/issues/282)) ([2e5371a](https://github.com/mad01/thismoon/commit/2e5371a28279720ae32b02968a9263faeb4216a1))
* **moondb:** web UI with session, turn and annotated file views ([#276](https://github.com/mad01/thismoon/issues/276)) ([bb2a972](https://github.com/mad01/thismoon/commit/bb2a97269705186ae69b70da496deca2a24571c1))
* **sandbox:** render srt settings from access manifests beside the code ([#278](https://github.com/mad01/thismoon/issues/278)) ([04f6c54](https://github.com/mad01/thismoon/commit/04f6c54bc5cc3ff7cb534e1bbbe2f7474190f050))
