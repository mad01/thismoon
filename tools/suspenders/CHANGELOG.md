# Changelog

## [0.4.0](https://github.com/mad01/thismoon/compare/suspenders/v0.3.3...suspenders/v0.4.0) (2026-08-13)


### Features

* **suspenders:** add config reference command ([00bd004](https://github.com/mad01/thismoon/commit/00bd0041929c3cf8f0205b3c5f51fd9eace02d35))
* **suspenders:** add doctor command ([ded36b6](https://github.com/mad01/thismoon/commit/ded36b6c3f237ee6f1ef3109dd4d865b82915c16))

## [0.3.3](https://github.com/mad01/thismoon/compare/suspenders/v0.3.2...suspenders/v0.3.3) (2026-07-18)


### Bug Fixes

* **suspenders:** scan plain directories, not just git working trees ([#97](https://github.com/mad01/thismoon/issues/97)) ([e798074](https://github.com/mad01/thismoon/commit/e7980741a4da3bf1ab3da933190bfb7c16446673))

## [0.3.2](https://github.com/mad01/thismoon/compare/suspenders/v0.3.1...suspenders/v0.3.2) (2026-07-15)


### Bug Fixes

* **suspenders:** move the version var to internal/cli so release ldflags land ([#89](https://github.com/mad01/thismoon/issues/89)) ([aacd2b1](https://github.com/mad01/thismoon/commit/aacd2b1ee26847be5c4c4247bc101bd5d6b30781))

## [0.3.1](https://github.com/mad01/thismoon/compare/suspenders/v0.3.0...suspenders/v0.3.1) (2026-07-10)


### Bug Fixes

* **suspenders:** exempt repos matching exclude globs from the guard ([#42](https://github.com/mad01/thismoon/issues/42)) ([98f7989](https://github.com/mad01/thismoon/commit/98f798930ea43ba1462d762e138fff4aa33ba749))

## [0.3.0](https://github.com/mad01/thismoon/compare/suspenders/v0.2.1...suspenders/v0.3.0) (2026-07-10)


### Features

* **suspenders:** layer per-repo guard overrides from .suspenders.yaml ([cef7cc4](https://github.com/mad01/thismoon/commit/cef7cc40122dd9586f680858fc086c56102dd028))
* **suspenders:** per-repo guard overrides and case-insensitive guard allowlist ([#41](https://github.com/mad01/thismoon/issues/41)) ([cef7cc4](https://github.com/mad01/thismoon/commit/cef7cc40122dd9586f680858fc086c56102dd028))


### Bug Fixes

* **suspenders:** match guard allowlist case-insensitively ([cef7cc4](https://github.com/mad01/thismoon/commit/cef7cc40122dd9586f680858fc086c56102dd028))
* **suspenders:** skip tracked symlinks to directories in scan ([#40](https://github.com/mad01/thismoon/issues/40)) ([14a694f](https://github.com/mad01/thismoon/commit/14a694fca56bc0e43461d48e21eb72e329495126))

## [0.2.1](https://github.com/mad01/thismoon/compare/suspenders/v0.2.0...suspenders/v0.2.1) (2026-07-07)


### Bug Fixes

* suspenders history scan now honors .suspenders.yaml path globs ([f87ca66](https://github.com/mad01/thismoon/commit/f87ca6603de3efc86026a7548fb8ed2cf57181ba))

## [0.2.0](https://github.com/mad01/thismoon/compare/suspenders/v0.1.0...suspenders/v0.2.0) (2026-07-07)


### Features

* MAD-215 migrate suspenders into tools/suspenders ([#24](https://github.com/mad01/thismoon/issues/24)) ([6bfda01](https://github.com/mad01/thismoon/commit/6bfda01fa17f5c2cf3d7f19645a99821a3ac00e9))
