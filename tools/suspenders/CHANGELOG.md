# Changelog

## [0.11.0](https://github.com/mad01/thismoon/compare/suspenders/v0.10.0...suspenders/v0.11.0) (2026-09-09)


### Features

* **belt,suspenders:** one shared internal-names file, doctor warnings, publish floor ([ac0c42d](https://github.com/mad01/thismoon/commit/ac0c42d510d7e5affaca1af163d2a7e213caa34c))
* **suspenders:** include shared names files and honor allow_phrases ([edb870d](https://github.com/mad01/thismoon/commit/edb870db15ed3d87ead8874098f0c0fb40f5604a))

## [0.10.0](https://github.com/mad01/thismoon/compare/suspenders/v0.9.0...suspenders/v0.10.0) (2026-09-01)


### Features

* **suspenders:** block OpenRouter API keys (sk-or-v1-) ([8459064](https://github.com/mad01/thismoon/commit/8459064cb188617bfab09da36b46db2fb8417da8))

## [0.9.0](https://github.com/mad01/thismoon/compare/suspenders/v0.8.0...suspenders/v0.9.0) (2026-08-29)


### Features

* **suspenders:** drop write-on-load, add config init, resolve paths via confdir ([1567746](https://github.com/mad01/thismoon/commit/1567746100ffe7356927b03e2d79acf71da5618c))

## [0.8.0](https://github.com/mad01/thismoon/compare/suspenders/v0.7.0...suspenders/v0.8.0) (2026-08-22)


### Features

* doctor contract for serve-bearing components (ADR-0009 wave 2) ([#174](https://github.com/mad01/thismoon/issues/174)) ([1e68bf4](https://github.com/mad01/thismoon/commit/1e68bf47c522c671828c2cd05fa5ea5ef20e0d87))

## [0.7.0](https://github.com/mad01/thismoon/compare/suspenders/v0.6.0...suspenders/v0.7.0) (2026-08-22)


### Features

* agent-facing operating docs across all components (ADR-0009) ([#172](https://github.com/mad01/thismoon/issues/172)) ([d52b48a](https://github.com/mad01/thismoon/commit/d52b48a5983e3c3f3ed8d1d1f30d269c326f5bee))

## [0.6.0](https://github.com/mad01/thismoon/compare/suspenders/v0.5.0...suspenders/v0.6.0) (2026-08-14)


### Features

* **suspenders:** derive org and repo blocked names as separate segments ([f1749bf](https://github.com/mad01/thismoon/commit/f1749bfc950223d6839441c3390ebf3b018fd187))


### Bug Fixes

* **suspenders:** disable background git maintenance in history tests ([4cfcb03](https://github.com/mad01/thismoon/commit/4cfcb03da277aa0395a326b73364460869df3c5b))

## [0.5.0](https://github.com/mad01/thismoon/compare/suspenders/v0.4.0...suspenders/v0.5.0) (2026-08-13)


### Features

* shared buildinfo package — commit/tag/build time in version and /version across all components ([#125](https://github.com/mad01/thismoon/issues/125)) ([3fc9af5](https://github.com/mad01/thismoon/commit/3fc9af5f3014052300da6ab55325190623867d96))
* **suspenders:** show effective config in config command, move reference to --help ([3b9f9e3](https://github.com/mad01/thismoon/commit/3b9f9e350dfefd3ba22b0a5d8ee3ebfeab37306c))

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
