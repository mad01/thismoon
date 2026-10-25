# srt-run expansion layer fixtures

Shared by `srt-run-test.sh` (the launcher) and the srtspec `layers` tests
(what `srtspec explain` reports), so the two implementations of the
widening rules cannot drift apart. `base.json` is the settings file each
case lays its layer over. A case named `ok-*.local.json` must launch; a
case named `refuse-*.local.json` must make srt-run exit 1 without starting
srt. Add a case here, not in either test.
