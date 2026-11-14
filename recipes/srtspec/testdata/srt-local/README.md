# srt expansion layer fixtures

Shared by the srtspec `launch` tests (what `srtspec run` accepts at launch)
and the `layers` tests (what `srtspec explain` reports), so the launcher and
the explainer cannot drift apart on what the expansion layer may do.
`base.json` is the settings file each case lays its layer over. A case named
`ok-*.local.json` must launch; a case named `refuse-*.local.json` must make
`srtspec run` exit 1 without starting srt. Add a case here, not in either
test.
