# ADR-0029: moondb gets a shared instance and a cloud store backend

**Date:** 2026-10-29
**Status:** Accepted

**Scope:** `services/moondb`: the store interface and its two backends, the
shared mode of `moondb serve`, and the replicate loop that feeds it. It
revisits the bucket that ADR-0018 turned down for present. Ticket MAD-469.
The ticket asked for this record as ADR-0027, a number taken by then.

## Context

moondb records every file edit an agent session makes between commits, with
the content before and after, the turn and prompt behind it, and the commit
it landed in. Until now each machine kept its own store under its workdir,
and `moondb serve` on loopback was the only process that wrote it. The goal
is one moondb for sessions from several machines, served from Kubernetes,
with the same CLI, MCP tools and page.

Some things had to survive that move:

- Capture can't wait on a network. The Edit hook is synchronous and has
  200 ms. A session must not slow down, or lose records, because a remote
  service is down.
- moondb writes nothing into the user's git: no trailers, notes or refs.
  Commit links come from matching blob ids.
- One process writes a store. That rule is why the hooks write a spool and
  why ingest can replay an envelope without harm.

## Decision

### Local first

Each machine keeps its store and its serve. The hooks still write the local
spool, and the local serve stays the one writer of that store. A shared
instance is a copy that local serves feed. No hook ever talks to it, and
`--base-url` stays a display setting.

### Records in a log, content in chunks

The records are one append-only JSONL log. File content, prompts,
transcripts and snapshot maps are chunks named by the SHA-256 of their
bytes, and records point at them by key. The same bytes are stored once,
and a chunk never changes after it is written. That makes copying cheap to
reason about: sending a chunk twice is harmless, and asking whether the
other side has one is a key lookup.

### Two backends behind one interface

`store.Store` has two implementations, and the `storetest` conformance
suite is the contract both pass:

- `local`: the log and gzipped chunk files on disk, as before.
- `cloud`: the same log and in-memory index on the persistent volume of a
  one-replica StatefulSet. Its chunks go to a Cloud Storage bucket at
  `objects/<sha>`, each created with `ifGenerationMatch=0`, so an object
  is written once and never replaced. A copy of the log sits in the same
  bucket at `log/<segment>`: a segment once the log rolls past it, the
  open one every hour and again on shutdown.

The cloud backend talks to the bucket through the JSON API with application
default credentials. It uses `golang.org/x/oauth2/google` and no storage
SDK. The client is a few hundred lines, and its tests run against an
httptest fake. The interface gained no method.

### Still one writer

`moondb serve --shared` drains no spool. A local serve started with
`--replicate-to` reads its own log from a saved cursor. It asks
`/api/sync/missing` which chunks those records point at that the instance
lacks, sends those chunks to `/api/sync`, then sends the records in log
order. The instance writes them through the same `Store` methods ingest
uses, so its log stays a log with one writer. Records keep their ids, so a
batch sent twice changes nothing, and a cursor that falls behind only costs
a resend.

### Keys and bind

Every route that holds a record needs a bearer key. The instance stores the
SHA-256 of each key it accepts and never the key. The page shell and its
assets need none, so a browser can load the page and ask for its key. The
MCP tools are mounted at `/mcp` as streamable HTTP and call the instance's
own API in process. Shared mode refuses to start until `--bind` names where
to listen.

### No checkout on the instance

The pod holds no worktree, and a synced record must not point a query at
the pod's own disk. On a shared instance `why` matches the line text the
caller sends, and `annotate` is refused.

### Why a bucket now

ADR-0018 weighed a bucket for present and chose custom resources. They add
no dependency beyond the cluster, they test in kind, and `kubectl get
pages` is the debugging surface. Their cost was the object size cap: a page
over 1 MiB is refused. Present's pages are small and most of them expire,
so that cap was cheap.

moondb's chunks are the reverse. One turn's transcript can be 4 MiB and one
subagent transcript 8 MiB. A snapshot holds up to a thousand files, and the
volume grows with every edit, since nothing deletes a chunk yet. A custom
resource cannot hold one such chunk. Etcd is also the wrong place for
gigabytes of file content, because it backs the whole cluster.

The persistent volume could hold the chunks too. But a volume belongs to
one zone and one pod, and losing it would lose every machine's history at
once. In a bucket the chunks outlive the volume, and the copy of the log
beside them means a new volume can be rebuilt from the bucket. The price is
the one ADR-0018 named: cloud credentials, which workload identity covers,
and a fake for tests.

Three other designs were weighed and dropped:

- Each machine writes straight to the bucket. That makes many writers of
  one store, puts bucket credentials on every laptop, and puts the network
  inside capture.
- The log itself lives in the bucket. Object storage has no append, so
  each record would cost a request and a new object, on the write path.
- A relational database: the most infrastructure for the least gain, as
  ADR-0018 found for present.

## Consequences

- `go.mod` takes `golang.org/x/oauth2` as a direct dependency and gains
  one module, `cloud.google.com/go/compute/metadata`.
- The shared instance is one replica with its index in memory, like a
  local serve. A restart replays the whole log, and memory bounds how much
  history it can serve. Read replicas are out of scope.
- Chunks are never deleted. The bucket grows until a retention decision
  exists, and it needs one before the bill matters.
- Restoring is manual: copy `log/` from the bucket onto a fresh volume
  before the pod starts. The open segment's copy can be an hour old. The
  records written in that hour come back when the machines send their logs
  again: deleting a machine's `replicate.json` makes it resend everything,
  which the ids make safe.
- `/healthz` on the cloud backend reads the bucket's metadata, so a
  liveness probe shouldn't use it, or a storage outage restarts the pod.
- A leaked key reads and writes every record until its hash leaves the
  keys file and the instance restarts. Keys are per machine so one can go
  without the rest.
- A local serve under srt reaches the instance only once its manifest
  allows the host. The host differs per machine, so it comes from
  `SRT_RUN_ALLOW_DOMAINS` (docs/SANDBOXING.md), and that wiring lands with
  the first machine that turns replication on.
- The Kubernetes manifests and the container image follow in a separate
  change, mirroring present's (ADR-0018). Bucket name, keys, hostnames and
  project stay in the consuming repo, per ADR-0006.
