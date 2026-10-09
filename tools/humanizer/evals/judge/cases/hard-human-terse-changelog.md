---
id: hard-human-terse-changelog
label: likely_human
bucket: hard
source: https://github.com/prometheus/prometheus/blob/v2.5.0/CHANGELOG.md sections 2.3.2 and 2.3.1, tag v2.5.0 (2018-11-06)
license: Apache-2.0 (prometheus/prometheus), quoted with attribution
generator: ""
words: 134
notes: Terse bracket-tagged changelog lines with two typos (Propogate, deference); the MAD-348 false-positive shape, short entries and no prose.
---
## 2.3.2 / 2018-07-12

* [BUGFIX] Fix various tsdb bugs #4369
* [BUGFIX] Reorder startup and shutdown to prevent panics. #4321
* [BUGFIX] Exit with non-zero code on error #4296
* [BUGFIX] discovery/kubernetes/ingress: fix scheme discovery #4329
* [BUGFIX] Fix race in zookeeper sd #4355
* [BUGFIX] Better timeout handling in promql #4291 #4300
* [BUGFIX] Propogate errors when selecting series from the tsdb #4136

## 2.3.1 / 2018-06-19

* [BUGFIX] Avoid infinite loop on duplicate NaN values. #4275
* [BUGFIX] Fix nil pointer deference when using various API endpoints #4282
* [BUGFIX] config: set target group source index during unmarshalling #4245
* [BUGFIX] discovery/file: fix logging #4178
* [BUGFIX] kubernetes_sd: fix namespace filtering #4285
* [BUGFIX] web: restore old path prefix behavior #4273
* [BUGFIX] web: remove security headers added in 2.3.0 #4259
