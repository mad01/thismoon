# suspenders

A fast, offline git secret scanner and hook orchestrator. Suspenders detects checked-in tokens, passwords, API keys, private keys, and certificates across your repositories. It installs git hooks that block secrets before they reach a remote, guards against leaking internal repository names into public repos, runs user-defined hook scripts, and can rewrite git history to remove a secret that already made it into a commit.

Named for the layer it adds: belt ([`tools/belt`](../belt/)) holds up the agent session, denying risky tool calls before anything reaches git; suspenders holds up git itself. Each tool reads only its own config, but both derive their internal-name list the same way, so the two layers agree when their configs do. belt only sees what an agent does; suspenders also catches what you type.

## Quickstart

```sh
make install                   # builds and installs to ~/code/bin/suspenders
suspenders hook install --all  # pre-commit hook in every repo under your dirs
```

Out of the box you get the defaults without a config file: secret scanning
on, repo discovery over `~/code/src` and `~/workspace`. Run `suspenders
config init` to write `~/.config/suspenders/config.yaml` when you want to
adjust `dirs` to your layout. From then on, a staged secret blocks the
commit that would carry it.

Two things the defaults don't do:

- The internal-name guard ships disabled. Turn it on by giving it a name
  source. Every repo found under `workspace_dirs` contributes its org,
  repo, and directory names to the blocked set:

  ```yaml
  guard:
    enabled: true
    workspace_dirs:
      - ~/work-checkouts
  ```

  Then `suspenders doctor` in any repo shows the derived blocked names, the
  config in effect, and whether that repo is exempt.

- Hooks are per-clone. Re-run `suspenders hook install` after cloning
  something new, or `suspenders hook install --all` to sweep every
  discovered repo. On a ralph-managed machine, a companion recipe in your
  config repo can run the sweep on every `ralph up` (the post-install wiring
  pattern; see `docs/adr/0006` and `examples/dotfiles/` at the repo root).

## When a block is wrong

A pre-commit block you disagree with has an escape ladder, narrowest first;
take the lowest rung that solves it. `suspenders doctor` in the repo shows
the state behind any decision: the config in effect, per-repo overrides,
whether the repo is exempt, and the derived blocked-name list.

1. One commit: `git commit --no-verify` skips the hook entirely. It is
   the deliberate, audited override: the block message names it, and the
   attempt that was blocked is already on the events timeline.
2. One line: a `suspenders:ignore` comment on the flagged line suppresses
   findings on that line permanently.
3. One value: a known-safe secret-shaped string goes in the `allowlist`
   (global config or the repo's `.suspenders.yaml`); a safe internal-name
   reference goes in `guard.allowlist`.
4. One repo: `.suspenders.yaml` at the repo root suppresses rules,
   paths, or patterns for that repo only; per-repo `allowlist` and `guard`
   entries append to the global config rather than replacing it.
5. One rule or check: `scan.enabled: false` or `guard.enabled: false`
   turns a whole check off; `suspenders hook uninstall [--all]` removes the
   hooks and restores whatever they backed up.

### Including and excluding repos

Three different mechanisms decide which repos the checks apply to, and they
answer different questions:

- `dirs` + `exclude` decide which repos suspenders manages at all.
  `exclude` entries are globs matched against the `org/repo` name from the
  origin remote. An excluded repo is skipped by `--all` discovery *and*
  exempt from the internal-name guard.
- `guard.workspace_dirs` membership exempts a repo from the guard by
  definition: a repo inside a workspace dir is internal, so referencing
  internal names there is fine. Exemption doesn't remove that repo's names
  from the block list other repos are checked against.
- Public vs internal is derived, never declared: there is no per-repo
  "this one is public" flag to get wrong. A repo is guarded exactly when it
  is neither inside `workspace_dirs` nor matched by `exclude`.

The session-side twin has the same shape: belt's repo scoping (allowlists by
canonical `host/owner/repo`, path excludes) is documented in
[`tools/belt/docs/hooks.md`](../belt/docs/hooks.md), "Overriding, allowing,
and disabling".

## Features

- 84 built-in detection rules covering AWS, GitHub, GitLab, Google, Slack, Stripe, OpenAI, Anthropic, HuggingFace, and dozens more
- 7 filename rules that flag sensitive files (`id_rsa`, `*.p12`, `.env`, `*.tfstate`, ...) even when the content is binary
- Shannon-entropy gating on generic rules, plus a standalone high-entropy string detector
- Staged scans read content from the git index, so the scan sees exactly what would be committed
- Inline `suspenders:ignore` comments to suppress findings on a single line
- Deep history scan: walks every commit on the public branch and flags the commit that introduced each finding, even when a later commit removed it
- History clean: rewrites git history in place to remove flagged strings and redact whole secret files, with a mandatory backup bundle, no git-filter-repo needed
- Multi-event hook orchestrator: manages pre-commit and post-merge hooks from a single config
- Internal-reference guard: blocks commits that mention internal org/repo names in public repositories
- External hook scripts: run your own commands (linters, formatters) as part of the hook pipeline
- Per-hook filtering by file pattern and repository name (glob matching)
- Custom watch patterns for organization-specific secrets
- Exact-match allowlists with optional path restrictions
- Per-repo ignore files (`.suspenders.yaml`) to suppress rules, paths, or known-safe values
- Scans only git-tracked files via `git ls-files`
- Automatic binary file detection and skipping
- Concurrent repository discovery with 32 workers
- Redacted output in all findings
- Backs up foreign hooks during install, chains to them from the generated hook, and restores them on uninstall
- Docker-based integration test suite

## How it works

### Scanning

1. Enumerates git-tracked files via `git ls-files` (untracked and `.gitignore`d files are excluded)
2. Applies filename rules: sensitive file names are flagged regardless of content
3. Skips binary files (detected by null byte in the first 512 bytes) and files larger than 1 MB
4. Matches each line against all rules (built-in + custom watch rules); every match on a line is reported, not just the first
5. Applies per-rule entropy gates and filters, then checks the global allowlist and per-repo `.suspenders.yaml`
6. Skips lines that carry a `suspenders:ignore` comment
7. Redacts every match on a line before printing, so one finding's context never exposes another finding's secret

In `--staged` mode, the file list comes from `git diff --cached --name-only --diff-filter=ACMR` and the content is read from the index (`git show :<path>`), not the working tree. The scan sees exactly what would be committed: staging a secret and then scrubbing the working copy without re-staging still gets caught. A file that can't be read fails the scan instead of being silently skipped.

### Hook orchestration

Generated hook scripts are thin dispatchers:

```sh
#!/bin/sh
# managed by suspenders - do not edit
# version: <checksum>
hook_dir=$(dirname -- "$0")
if [ -x "$hook_dir/pre-commit.backup" ]; then
  "$hook_dir/pre-commit.backup" "$@" || exit $?
fi
exec suspenders hook run pre-commit
```

A foreign hook found at install time is moved to `<event>.backup` and the generated script chains to it first, so existing hooks keep running. Hook scripts use the bare binary name (`suspenders`) rather than an absolute path, so they keep working after rebuilds or relocations as long as the binary is in `$PATH`.

The `hook run` subcommand loads config, identifies the current repo, then runs each configured check in order:

For `pre-commit`:

1. External `pre_commit` hooks (in config order, stop on first failure)
2. Built-in guard (if `guard.enabled` is true)
3. Built-in scan (if `scan.enabled` is true, which is the default)

For `post-merge`:

1. External `post_merge` hooks (in config order)

Any non-zero exit from a step blocks the git operation (for pre-commit) or logs a warning (for post-merge).

Each hook file includes a SHA-256 checksum of its body (excluding the version line). `suspenders hook update` compares this checksum to detect stale hooks without re-reading the binary.

### Repository discovery

The `--all` flag walks every directory in `config.yaml`'s `dirs` array, discovers git repositories with 32 concurrent workers, extracts the `org/repo` name from each remote origin URL, and filters out repos matching `exclude` globs.

Discovery runs through the shared [`kit/repofind`](../../kit/repofind/README.md) package, and suspenders uses it for two separate walks with two separate config lists:

- `dirs` answers "which repos do I manage": `hook install --all` installs hooks into every repo found here (minus `exclude` globs).
- `guard.workspace_dirs` answers "which names are internal": every repo found here contributes its org segment, repo segment, and checkout directory basename as three separate blocked names, never the combined `org/repo` form.

belt's `write-internal-names` guard runs the same walk over the same package from its own `internal_names` config section (same derivation, standalone configs, docs/adr/0010), which is what keeps the write-time and commit-time block lists identical when the two configs carry the same values. The block list is derived fresh on every run and never persisted: a config file enumerating internal names would itself be the leak.

## Install

```sh
make install    # builds and installs to ~/code/bin/suspenders
```

To build a local binary without installing:

```sh
make build      # outputs to ./suspenders
```

Pre-built binaries ship as `suspenders/vX.Y.Z` releases of the thismoon monorepo for Linux and macOS (amd64/arm64), with checksums and cosign signatures.

## Usage

### Scan for secrets

Scan the current directory:

```sh
suspenders scan
```

Scan a specific path:

```sh
suspenders scan /path/to/repo
```

Scan only git-staged files (used by the pre-commit hook internally):

```sh
suspenders scan --staged --fail-on-findings
```

The `--fail-on-findings` flag causes an exit code of 1 when secrets are detected.

When the guard is enabled (see [Internal-reference guard](#internal-reference-guard)), `scan` also reports blocked names found in tracked files, with file and line: internal repo names and `blocked_words` entries such as a company or brand name. A staged scan checks the staged diff instead, matching what the pre-commit hook would block.

### Manage hooks

Install hooks in the current repository:

```sh
suspenders hook install
```

Install hooks in all discovered repositories:

```sh
suspenders hook install --all
```

This writes both `pre-commit` and `post-merge` hook scripts (when post-merge hooks are configured). Each hook script calls `suspenders hook run <event>`, which dispatches to your configured external hooks and built-in checks.

Check hook status:

```sh
suspenders hook status         # current repo
suspenders hook status --all   # all discovered repos
```

Status output shows per-event status:

| Status | Meaning |
|--------|---------|
| `installed` | Hook is present and up to date |
| `outdated` | Hook exists but its checksum differs from the current version |
| `foreign` | A hook exists but was not installed by suspenders |
| `not-installed` | No hook found for this event |

Update outdated hooks:

```sh
suspenders hook update --all
```

Remove hooks:

```sh
suspenders hook uninstall --all
```

When installing over an existing non-suspenders hook, the original is backed up with a `.backup` suffix. Uninstalling restores the backup.

### Scan full git history

The staged and directory scans only see the present. A secret committed last year and deleted the next day is invisible to both, yet still sits in every clone. `history scan` walks every commit reachable from the public branch and runs the built-in checks (secret scan, plus the guard when enabled) over each commit's added lines and commit message:

```sh
suspenders history scan
suspenders history scan --branch origin/main
suspenders history scan --fail-on-findings
```

Each finding is attributed to the commit that introduced it:

```
commit a00b5424b32d "oops add key" — Ada <ada@example.com>
  [high] AWS access key ID (aws-access-key-id)
    s.env:1:19
    AWS_ACCESS_KEY_ID=AKIA************MPLE
```

Without `--branch`, the walked ref is `origin/HEAD`, falling back to `main`, `master`, then `HEAD`.

### Clean git history

`history clean` rewrites all local branches and tags in place, replacing flagged strings in file contents and commit messages with `***REDACTED***`. Files that are secrets wholesale (a committed private key, a token file) can be redacted in place: the file stays in every commit's tree, but its content becomes `***REDACTED***` everywhere.

```sh
# Collect replacements automatically from a history scan, then rewrite.
# File-name findings (a committed id_rsa, .env) become whole-file redactions:
suspenders history clean

# Or name the strings explicitly:
suspenders history clean --replace 'AKIAIOSFODNN7EXAMPLE'

# Redact whole files by path or glob (bare names match in any directory):
suspenders history clean --redact-file id_rsa --redact-file 'certs/*.p12'

# See what would change without touching anything:
suspenders history clean --dry-run --replace 'AKIAIOSFODNN7EXAMPLE'
```

The rewrite runs `git fast-export` through a stream transform into `git fast-import`, so it needs no external tools. Safety rails:

- Refuses to run on a bare repo, a detached HEAD, or a dirty working tree.
- Always writes a backup bundle to `.git/suspenders-backup-<timestamp>.bundle` first; `git clone <bundle>` restores the original history.
- Asks for confirmation with the full blast radius (refs, replacements, redacted files) unless you pass `--yes`.
- Binary blobs are never string-replaced; if one contains a flagged string you get a warning instead. A `--redact-file` glob does apply to binaries: the whole blob is redacted.

Every commit hash changes from the first affected commit onward. Afterwards you must force-push rewritten branches, and collaborators must re-clone; their old clones still hold the removed strings. Stale commit signatures are dropped, since they signed the old content.

For a step-by-step walkthrough of the rewrite (what runs, what gets edited, how to undo it), see [docs/history-clean.md](docs/history-clean.md).

### Run hooks manually

The `hook run` subcommand is what the generated hook scripts call. You can also invoke it directly for testing:

```sh
suspenders hook run pre-commit
suspenders hook run post-merge
```

### Explain the guard for a repo

`doctor` shows why the guard decides what it decides: the installed build, the global config, any per-repo overrides from `.suspenders.yaml`, whether the repo is guard-exempt, and the full blocked-name list derived from the workspace dirs and blocked words. Use it when a commit was blocked (or wasn't) and the reason isn't obvious. The list is derived fresh on every run and never written anywhere.

```sh
suspenders doctor                # current directory
suspenders doctor /path/to/repo
```

`suspenders config` prints the config file location and the settings in effect after defaults are applied. `suspenders config --help` carries the annotated reference of every setting: which key exempts a repo (`exclude`), which marks a name safe (`guard.allowlist`), and what the per-repo `.suspenders.yaml` can override.

```sh
suspenders config
suspenders config init          # write the defaults to the config file
```

### Print version

```sh
suspenders version              # bare version token
suspenders version -o json      # version, commit, tag, build_time
```

Plain output is the version and nothing else, so a probe can read the line as-is. The JSON form is the four-key build metadata object every tool in this repo reports, with each key present and `""` for anything the build didn't stamp.

## Detection rules

Suspenders ships with 84 built-in content rules validated against GitLeaks, GitHub Secret Scanning, and TruffleHog, plus 7 filename rules. Custom rules can be added via the `watch` config section.

### Cloud providers

| Rule ID | Description | Severity |
|---------|-------------|----------|
| `aws-access-key-id` | AWS access key ID (`AKIA...`) | high |
| `aws-secret-access-key` | AWS secret access key | high |
| `aws-session-token` | AWS temporary session token (`ASIA...`) | high |
| `gcp-service-account-json` | GCP service account key (JSON) | high |
| `google-api-key` | Google API key (`AIza...`) | high |
| `google-oauth-client-secret` | Google OAuth client secret | high |
| `azure-client-secret` | Azure client secret | high |
| `digitalocean-pat` | DigitalOcean personal access token (`dop_v1_`) | high |
| `cloudflare-api-token` | Cloudflare API token | high |
| `alibaba-access-key-id` | Alibaba Cloud access key ID (`LTAI...`) | high |

### Source control and CI/CD

| Rule ID | Description | Severity |
|---------|-------------|----------|
| `github-pat-classic` | GitHub token: classic PAT, OAuth, app, refresh (`gh[pousr]_`) | high |
| `github-pat-fine-grained` | GitHub fine-grained personal access token | high |
| `gitlab-pat` | GitLab personal access token (`glpat-`) | high |
| `gitlab-runner-token` | GitLab runner authentication token (`glrt-`) | high |
| `gitlab-deploy-token` | GitLab deploy token (`gldt-`) | high |
| `bitbucket-app-password` | Bitbucket app password or API token | high |
| `azure-devops-pat` | Azure DevOps personal access token | high |
| `circleci-token` | CircleCI personal API token | high |
| `atlassian-api-token` | Atlassian API token (`ATATT3...`) | high |

### Communication and messaging

| Rule ID | Description | Severity |
|---------|-------------|----------|
| `slack-bot-token` | Slack bot token (`xoxb-`) | high |
| `slack-user-token` | Slack user token (`xoxp-`) | high |
| `slack-app-token` | Slack app/workspace/service tokens (`xox[oas]-`) | high |
| `slack-app-level-token` | Slack app-level token (`xapp-`) | high |
| `slack-webhook` | Slack incoming webhook URL | high |
| `discord-bot-token` | Discord bot token | high |
| `telegram-bot-token` | Telegram bot token | high |

### AI/ML providers

| Rule ID | Description | Severity |
|---------|-------------|----------|
| `openai-api-key` | OpenAI API key legacy (`sk-...T3BlbkFJ...`) | high |
| `openai-project-key` | OpenAI project API key (`sk-proj-`) | high |
| `anthropic-api-key` | Anthropic API key (`sk-ant-`) | high |
| `openrouter-api-key` | OpenRouter API key (`sk-or-v1-`) | high |
| `huggingface-token` | HuggingFace access token (`hf_`) | high |
| `cohere-api-key` | Cohere API key (keyword-gated) | high |
| `replicate-api-token` | Replicate API token (`r8_`) | high |
| `perplexity-api-key` | Perplexity AI API key (`pplx-`) | high |
| `groq-api-key` | Groq API key (`gsk_`) | high |

### Payments, SaaS, and email

| Rule ID | Description | Severity |
|---------|-------------|----------|
| `stripe-live-secret-key` | Stripe live secret key (`sk_live_`) | high |
| `stripe-live-restricted-key` | Stripe live restricted key (`rk_live_`) | high |
| `stripe-webhook-secret` | Stripe webhook signing secret (`whsec_`) | high |
| `sendgrid-api-key` | SendGrid API key (`SG.`) | high |
| `twilio-api-key` | Twilio API key or account SID | high |
| `shopify-access-token` | Shopify access token (`shpat_`/`shpca_`/`shpss_`) | high |
| `heroku-api-key` | Heroku API key | high |
| `mailgun-api-key` | Mailgun API key (keyword-gated `key-`) | high |
| `mailchimp-api-key` | Mailchimp API key | high |
| `postman-api-token` | Postman API token (`PMAK-`) | high |

### Package registries

| Rule ID | Description | Severity |
|---------|-------------|----------|
| `npm-token` | npm access token (`npm_`) | high |
| `pypi-token` | PyPI upload token (`pypi-`) | high |
| `nuget-api-key` | NuGet API key (`oy2`) | high |
| `dockerhub-token` | Docker Hub personal access token (`dckr_pat_`) | high |
| `rubygems-api-token` | RubyGems API token (`rubygems_`) | high |

### Platforms, observability, and infrastructure

| Rule ID | Description | Severity |
|---------|-------------|----------|
| `vercel-token` | Vercel API token (`vcp_`) | high |
| `linear-api-key` | Linear API key (`lin_api_`) | high |
| `planetscale-token` | Planetscale service token (`pscale_tkn_`) | high |
| `datadog-api-key` | Datadog API key | high |
| `okta-api-token` | Okta API token | medium |
| `grafana-service-account-token` | Grafana service account token (`glsa_`) | high |
| `grafana-cloud-api-token` | Grafana Cloud API token (`glc_`) | high |
| `newrelic-user-api-key` | New Relic user API key (`NRAK-`) | high |
| `newrelic-insert-key` | New Relic insert/browser key (`NRI[IJS]-`) | high |
| `sentry-org-token` | Sentry organization auth token (`sntrys_`) | high |
| `sentry-user-token` | Sentry user auth token (`sntryu_`) | high |
| `doppler-api-token` | Doppler API token (`dp.pt.`) | high |
| `terraform-cloud-token` | Terraform Cloud API token (`atlasv1-`) | high |
| `hashicorp-vault-token` | HashiCorp Vault token (`hvs.`) | high |
| `tailscale-key` | Tailscale auth/API/client key (`tskey-`) | high |
| `dynatrace-api-token` | Dynatrace API token (`dt0c01.`) | high |
| `azure-storage-account-key` | Azure storage account key (keyword-gated) | high |
| `pulumi-api-token` | Pulumi API token (`pul-`) | high |
| `flyio-access-token` | Fly.io access token (`fo1_`) | high |
| `mapbox-api-token` | Mapbox public API token (`pk.`) | medium |
| `databricks-pat` | Databricks personal access token (`dapi`) | high |
| `firebase-web-api-key` | Firebase web API key | medium |
| `supabase-service-key` | Supabase service role key (`sbp_`) | high |

### Cryptographic material and generic patterns

| Rule ID | Description | Severity |
|---------|-------------|----------|
| `private-key-header` | PEM private key block (RSA, EC, DSA, OPENSSH, PGP, encrypted PKCS#8, SSH2) | high |
| `putty-private-key` | PuTTY private key file header (PPK) | high |
| `pem-certificate-with-key` | PEM certificate block | low |
| `age-secret-key` | age encryption secret key (`AGE-SECRET-KEY-1`) | high |
| `jwt-token` | JSON Web Token (`eyJ...`) | medium |
| `generic-secret-assignment` | Generic secret assignment in code (entropy-gated) | medium |
| `unquoted-secret-assignment` | Unquoted secret assignment, `.env`/YAML style (entropy-gated) | medium |
| `hardcoded-password-string` | Hardcoded password in string literal (entropy-gated) | medium |
| `authorization-bearer` | Authorization header with Bearer token | high |
| `db-connection-string` | Database connection string with credentials | high |
| `url-basic-auth` | Credentials embedded in a URL, any scheme | medium |
| `high-entropy-string` | High-entropy string of 40+ characters (entropy >= 4.7) | low |

### Filename rules

These fire on the file's name alone, so binary key material (PKCS#12 keystores, DER keys) gets caught even though content scanning skips binaries.

| Rule ID | Flags | Severity |
|---------|-------|----------|
| `ssh-private-key-file` | `id_rsa`, `id_dsa`, `id_ecdsa`, `id_ed25519` | high |
| `key-material-file` | `*.key`, `*.p12`, `*.pfx`, `*.jks`, `*.keystore`, `*.ppk`, `*.p8` | high |
| `pem-file` | `*.pem` | medium |
| `dotenv-file` | `.env`, `.env.*` (except `.env.example`, `.env.sample`, `.env.template`, `.env.dist`) | medium |
| `credential-store-file` | `.netrc`, `_netrc`, `.git-credentials`, `.htpasswd`, `.pypirc` | high |
| `terraform-state-file` | `*.tfstate`, `*.tfstate.backup` | high |
| `kubeconfig-file` | `kubeconfig` | high |

## Configuration

Suspenders reads its config from `~/.config/suspenders/config.yaml` (or `$XDG_CONFIG_HOME/suspenders/config.yaml`); `--config <path>` and `$SUSPENDERS_CONFIG` point it somewhere else. The file is optional: with none, the defaults below apply in memory, and suspenders never creates one on its own. `suspenders config init` writes it explicitly. A file that exists but fails to parse is an error, so a broken config blocks commits rather than letting them through unchecked.

```yaml
# Directories to scan for git repositories (used by --all)
dirs:
  - ~/code/src
  - ~/workspace

# Repo name patterns to exclude (glob syntax): skipped by discovery (--all)
# and exempt from the guard — commits in a matching repo are never
# guard-blocked, though its name still contributes to the block list
exclude:
  - "*/vendor/*"

# Built-in scanner toggle (enabled by default)
scan:
  enabled: true

# Internal-reference guard
guard:
  enabled: true
  workspace_dirs:
    - ~/workspace
  blocked_words:
    - acmecorp
    - "*.acmecorp.net"
    - docs.acmecorp.net/runbooks
  allowlist:
    - grpc/grpc-go
  file_patterns:
    - "*.go"
    - "*.md"
    - "*.yaml"

# Custom detection rules
watch:
  - id: "internal-api-token"
    description: "Internal API token"
    pattern: "INTERNAL_TOKEN\\s*=\\s*(.+)"
    severity: "high"

# Known-safe values to suppress globally
allowlist:
  - match: "AKIAIOSFODNN7EXAMPLE"
    description: "AWS example key from documentation"
  - match: "sk_test_1234567890"
    paths: ["**/Makefile"]

# History clean settings
history:
  replace_table:
    old.internal.net: new.example.com
  redact_files:
    - id_rsa
    - "certs/*.p12"

# External hook scripts grouped by event
hooks:
  pre_commit:
    - name: go-vet
      command: go vet ./...
      file_patterns: ["*.go"]
      repos: ["mad01/ralph"]

  post_merge:
    - name: csl-reindex
      command: |
        mkdir -p "${HOME}/.config/csl" &&
        printf '%s\n' "$(git rev-parse --show-toplevel)" >> "${HOME}/.config/csl/reindex.queue"
```

### Built-in scan

The secret scanner runs by default on every pre-commit. To disable it (while keeping the guard and external hooks), set `scan.enabled` to `false`. When the field is omitted, scanning is enabled.

### Internal-reference guard

The guard prevents internal repository names, and any other string you list (a brand name, an internal domain, a docs link), from leaking into public repos. When `guard.enabled` is `true`, it:

1. Walks `workspace_dirs` to discover repos and derive their names (see below)
2. Adds each entry from `blocked_words` (matched regardless of workspace scan)
3. Removes entries in `allowlist`
4. Checks the staged diff (added/modified lines only) in files matching `file_patterns` for case-insensitive matches

Any match blocks the commit with a message listing the matched terms.

#### How blocked names are collected

The block list is derived from your filesystem, not maintained by hand. Enumerating internal repo names in a config file is itself a leak waiting to happen: the config would be the one file that lists everything it is supposed to protect. So the guard recomputes the list on every run from what is actually checked out:

1. Each directory in `workspace_dirs` is walked (`~` expands to your home; repo inspection fans out to 32 workers). Hidden directories are skipped, discovery doesn't recurse into nested repos, and unreadable entries are silently passed over.
2. A directory counts as a repo when it contains `.git`. For each repo found, the guard derives separate names:
   - the org name and the repo name, each on its own, parsed from the `origin` remote URL, in both SSH (`git@host:org/repo.git`) and HTTPS (`https://host/org/repo.git`) forms. The combined `org/repo` string is never a name of its own: a nested checkout like `~/workspace/foo/bar` blocks `foo` and `bar`, and allowlisting a segment works without spelling out every combination.
   - the repo's directory basename, so a repo checked out under a local name that differs from its remote name is blocked under both
3. When a repo has no `origin` remote or the URL can't be parsed, discovery falls back to `parentdir/repodir` from the filesystem path.
4. Safe references (`guard.allowlist`) are dropped, `blocked_words` entries are appended, and the result is deduplicated.

Because the list is recomputed per run and never persisted, a freshly cloned internal repo is guarded from the very next commit with zero configuration. The trade-off is coverage-by-checkout: a repo that only exists on a colleague's machine contributes nothing to your block list. Internal *hostnames* in particular never appear as repo checkouts, which is what `blocked_words` wildcard entries (`*.acmecorp.net`) are for.

Two details worth knowing:

- The top-level `exclude` globs exempt a repo from the guard *running in it*: a repo whose `org/repo` name matches an exclude pattern can be committed to freely, like a repo inside `workspace_dirs`. They **never** apply to name collection: an excluded repo checked out under a workspace dir still contributes its name to the block list for other repos.
- Names whose edges are word characters are matched with word-boundary guards, so a short repo name like `hig` can't match inside "higher". Entries with wildcard or punctuation edges keep their full reach.

Blocked words are matched case-insensitively as literal strings, so an entry can be a single word (`acmecorp`), an internal domain (`internal.acmecorp.net`), or a docs link (`docs.acmecorp.net/runbooks`). A `*` in an entry matches any run of non-whitespace characters: `*.acmecorp.net` blocks every subdomain, and the match extends over the URL scheme so history cleanup replaces the whole reference. Overlapping entries match longest-first, so a docs link wins over its bare domain.

The same block list runs in three other places: `suspenders scan` checks every tracked file in the working tree (reported with file and line), `history scan` checks every commit's added lines and message, and `history clean` collects the matches as replacement strings when rewriting history. Repos inside `workspace_dirs` and repos matching the top-level `exclude` globs are skipped everywhere; internal and explicitly excluded repos may reference internal names.

### External hooks

External hooks run shell commands as part of the hook pipeline. Each entry supports these fields:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | required | Identifier shown in logs and error messages |
| `command` | string | required | Shell command executed via `sh -c` |
| `file_patterns` | string[] | `[]` (all files) | Only run if staged/changed files match these globs (a gate; the matching files are not passed to the command) |
| `repos` | string[] | `[]` (all repos) | Only run in repos whose `org/repo` name matches these globs |
| `enabled` | bool | `true` | Toggle without removing config |

Hooks are grouped under `hooks.pre_commit` and `hooks.post_merge`.

### Keeping a code index fresh (csl)

If you run code-search-local (csl), keep its index current by reindexing each repo after a merge. The recommended way is a `csl-reindex` external hook under `hooks.post_merge`:

```yaml
hooks:
  post_merge:
    - name: csl-reindex
      command: |
        mkdir -p "${HOME}/.config/csl" &&
        printf '%s\n' "$(git rev-parse --show-toplevel)" >> "${HOME}/.config/csl/reindex.queue"
```

On every merge this appends the repo's path to `~/.config/csl/reindex.queue`. The queue is drained later by `csl sync`, which reindexes each queued repo and clears the file. Queuing keeps the post-merge hook fast; the actual reindex happens out of band.

### Suspenders owns git hooks

csl can install its own `post-merge` hook to reindex on merge. Once suspenders manages the `post-merge` event with the `csl-reindex` entry above, it owns that reindex; running csl's hook as well would queue the same repo twice per merge. Disable csl's own post-merge hook so suspenders is the single owner:

```sh
csl hooks uninstall
```

and set `hooks.post_merge.enabled: false` in csl's config.

When `suspenders hook install` finds an existing csl-managed `post-merge` hook (it carries a `# csl-managed-hook:` marker), it doesn't chain to it. The csl hook is set aside under `<event>.csl-replaced` (a name the generated hook never runs) and a warning is printed, so the reindex runs only via the `csl-reindex` entry, never twice. Generic (non-csl) foreign hooks are still backed up to `<event>.backup` and chained as usual.

### Migrating existing checkouts

If a repo already has csl's post-merge hook installed, migrate it to suspenders ownership:

1. Install the new suspenders and csl binaries.
2. Add the `csl-reindex` entry to `~/.config/suspenders/config.yaml` under `hooks.post_merge` (see above).
3. Disable csl's own hook: run `csl hooks uninstall` and set `hooks.post_merge.enabled: false` in csl's config.
4. Run `suspenders hook install --all` to take over the hooks (csl-managed post-merge hooks are set aside, not chained).
5. Run `suspenders hook status --all` to verify the hooks are `installed`.

### Per-repo ignore file

Place a `.suspenders.yaml` (or `.suspenders.yml`) at the root of any repository to layer local settings over the global config. Both `scan` and the pre-commit hook honor it, and `scan <path>` on a subdirectory resolves it from the enclosing repo's root:

```yaml
rules:
  - jwt-token

paths:
  - "**/*_test.go"

patterns:
  - EXAMPLE

allowlist:
  - match: "test-dummy-key"

guard:
  allowlist:          # names safe to reference in this repo,
    - monitoring      # appended to the global guard allowlist
  blocked_words:      # extra names blocked only in this repo
    - project-x
```

Guard allowlist entries match case-insensitively, like the guard itself: `monitoring` also covers `Monitoring`.

### Inline ignore

For a single false positive, add a `suspenders:ignore` comment on the line instead of editing config:

```go
testKey := "AKIAEXAMPLEEXAMPLE00" // suspenders:ignore
```

All findings on that line are suppressed.

## Where things live

- Config: `~/.config/suspenders/config.yaml` (or `$XDG_CONFIG_HOME/suspenders/config.yaml`, or wherever `--config`/`$SUSPENDERS_CONFIG` points), optional and never created implicitly; `suspenders config init` writes one
- Per-repo overrides: `.suspenders.yaml` (or `.yml`) at a repo's root, holding ignore rules, paths, patterns, allowlist, and guard overrides
- Binary: `~/code/bin/suspenders` (via `make install`)
- Generated hooks: `.git/hooks/pre-commit`, `.git/hooks/post-merge`, both calling `suspenders hook run <event>`
- Foreign hook backups: `.git/hooks/<event>.backup`, restored on `hook uninstall`
- History clean backup bundle: `.git/suspenders-backup-<timestamp>.bundle`, written before every rewrite

## Develop

To work on suspenders by hand (add or tune a detection rule, test it against a real secret, debug a false positive or negative), see [docs/working-on-it.md](docs/working-on-it.md). The rest of this section is the quick reference.

### Prerequisites

- Go 1.26+
- [golangci-lint](https://golangci-lint.run/usage/install/) (for linting)
- Docker (for integration tests)

### Commands

```sh
make build              # build to ./suspenders
make install            # install to ~/code/bin
make test               # run unit tests
make test-integration   # run Docker-based integration tests
make lint               # run golangci-lint
make fmt                # format source with golines and gofumpt
make clean              # remove the built binary
```

### Project structure

```
cmd/suspenders/
  main.go                       # entrypoint
  commands/
    root.go                     # cobra root command
    scan.go                     # scan command (standalone secret scanning)
    hook.go                     # hook install/update/status/uninstall/run
    version.go                  # version command
internal/
  config/config.go              # global config (~/.config/suspenders/config.yaml)
  guard/guard.go                # internal-reference guard (workspace name scanning)
  scanner/
    rules.go                    # built-in detection rules
    scanner.go                  # file/directory/staged scanning, allowlist
    ignore.go                   # per-repo .suspenders.yaml suppression
    result.go                   # Finding type and Redact function
  hook/hook.go                  # hook script generation, install/update/uninstall
  repo/finder.go                # concurrent repository discovery
tests/integration/              # Docker-based integration test scripts
```

### Integration tests

Integration tests run inside Docker containers to isolate git operations:

```sh
make test-integration
```

| Test | Scenario |
|------|----------|
| `test_hook_install` | `hook install` writes the hook file with the marker and makes it executable |
| `test_hook_allows_clean` | Clean commits pass through the hook |
| `test_hook_blocks_secret` | Commits containing secrets are blocked |
| `test_hook_chain` | Foreign hooks are backed up and chained |
| `test_hook_csl_takeover` | `hook install` takes over a csl-managed post-merge hook without chaining, queuing exactly one reindex per merge |
| `test_hook_update` | Outdated hooks are refreshed |
| `test_hook_uninstall` | Hook is removed and backup is restored |
| `test_hook_install_all` | Hooks are installed across multiple repos |
| `test_scan_staged` | Only staged files are scanned in `--staged` mode |

See [`CLAUDE.md`](CLAUDE.md) for the module layout and domain vocabulary in [`CONTEXT.md`](CONTEXT.md).

## License

BSD-3-Clause via the repo root [LICENSE](../../LICENSE); no per-file
headers, the root license covers the whole monorepo.

## Docs

- [architecture](architecture.md): internal structure and data flow
- [operating](operating.md): runtime behavior, failure modes, first moves
- [why](why.md): why this component exists
- [config](config.md): configuration reference
- [CONTEXT](CONTEXT.md): domain vocabulary (allowlist vs safe references vs blocked name)
- [history-clean](docs/history-clean.md): walkthrough of the history rewrite
- [working-on-it](docs/working-on-it.md): hands-on guide to adding and tuning detection rules
