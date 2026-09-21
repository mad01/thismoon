# why d-man

## The problem

A machine running the `*.this` platform accumulates local services, each on
its own port: present on 7423, csl on 7424, and so on. Reaching them as
`127.0.0.1:<port>` means memorizing ports and breaking every saved link when
one moves. The obvious fixes fail on macOS 26 (Tahoe): `mDNSResponder`
hijacks DNS queries for any non-IANA TLD and answers them itself, so a local
DNS server registered under `/etc/resolver/this` never receives the query,
and `*.localhost` names resolve only in Chrome and Firefox, not in Safari or
`curl`. There was no way to type `http://present.this/` and have it work in
every client.

## Why its own service

Name resolution and port routing span every service on the machine, so they
cannot live inside any one of them. The job also needs root twice over:
binding `:80` and `:443`, and writing `/etc/hosts`. Concentrating those
privileges in a single root process means no other component ever needs sudo.
d-man is a platform foundation of the `.this` stack: recipes may take a hard
`depends_on` on it, an exception granted only to it and t-man
(docs/adr/0006). The off-the-shelf answer, a local DNS server, is exactly
what `mDNSResponder` defeats.

## Why this shape

Three decisions carry the design. `/etc/hosts` instead of DNS:
`getaddrinfo` reads the hosts file before `mDNSResponder` gets a say, so
`.this` names resolve in every client. The cost is that every name must be
listed explicitly (no wildcard), acceptable because the routes file is
already an explicit list. One `routes.toml` drives both jobs, the managed
hosts block and the Host-header reverse proxy, so names and backends can't
drift apart; the hosts writer is fail-safe (it only replaces text between
its own markers, backs up first, writes atomically), and an invalid edit is
logged and ignored while the last good routes stay live. And sudo is
confined to one-time setup: after the root launchd registration via t-man,
the service watches `routes.toml` and its own executable, so route edits
apply live and upgrades relaunch through launchd's KeepAlive with no
further privilege.

## Non-goals

d-man is not a DNS server and offers no wildcard resolution. It listens on
loopback only and never exposes services beyond the machine. It does not
start, stop, or supervise the backends it routes to; that is t-man's job,
and a dead backend gets a 502. The blocklist pins individual hostnames to a
local block page; it is not network-wide content filtering and does not
match subdomains.
