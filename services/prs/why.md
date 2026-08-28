# why prs exists

Open pull requests scatter across repos and hosts. Knowing whether anything
needs action means visiting each host's notification page and trusting it to
surface the right things. The repos that matter are already on this machine —
their checkouts name every host and repo worth watching — so the dashboard
derives its scope from the filesystem instead of a hand-maintained list.

prs is the second take on this idea. The first (`services/pr`, retired)
worked but carried lessons:

- **Shelling out to `gh api` per call** bought free multi-host auth and cost
  sandboxing plus a subprocess per request. prs keeps gh as the credential
  source but execs it once per host to mint a token, then uses a real API
  client.
- **A hand-maintained repo list in config** drifted from reality. prs
  discovers repos the way csl does and reflects what is actually checked out.
- **A detail page with server-rendered diffs** doubled the code for a view
  GitHub already renders better. prs links out instead.

## boundaries (non-goals)

- **Read-only.** No approve, no merge, no comment. Acting on a PR happens on
  GitHub, one click away. This keeps the service credential-light and the
  failure modes boring.
- **No webhooks, no GitHub App.** Push delivery needs a publicly reachable
  endpoint; this is a localhost service. Polling with a sane interval is
  enough for a personal dashboard.
- **Only open, non-draft PRs.** A draft is not asking for attention; a
  closed or merged PR no longer can be acted on. The cache never stores them.
- **A cache, not a record.** The store can be deleted at any time; GitHub
  holds the truth and one poll cycle rebuilds the view.
