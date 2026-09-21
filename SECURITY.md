# Security

thismoon is a toolbox of local services and CLI tools for one macOS machine.
Everything runs on the machine that installed it and works on local data. A bug
that lets a component read, change, or run more than its own docs say it should
still counts as a security issue, and so does anything that sends local data
off the machine.

## Reporting a vulnerability

Use GitHub's private vulnerability reporting: open the Security tab of this
repository and choose "Report a vulnerability". That keeps the details out of
the public issue list until a fix has shipped.

Include the component (the directory under `services/` or `tools/`), the
output of `<binary> version`, and steps to reproduce.

## What to expect

This is a one-person project. Reports get an acknowledgement within a week.
Fixes ship as a normal release of the affected component, with a line in that
component's CHANGELOG.

## Supported versions

Only the latest release of each component gets fixes. Releases are tagged per
component as `<name>/vX.Y.Z`.
