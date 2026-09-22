# deploying a shared present instance

These manifests run `present serve --shared --store k8s` in a Kubernetes
namespace: stateless replicas behind one hostname, keeping every page as a
`Page` custom resource. None of it touches the local loopback present; that
side is the service [README](../README.md).

## what gets deployed

`deploy/base` is namespace-agnostic and holds seven objects:

- the `pages.present.thismoon.mad01.dev` custom resource definition (CRD),
  cluster-scoped, one `Page` object per page
- a `present` ServiceAccount, plus a Role and RoleBinding granting get,
  list, create, update, and delete on `pages` in this namespace only
- a Deployment of 2 replicas whose rolling update never drops below full
  strength (`maxUnavailable: 0`, `maxSurge: 1`), with soft anti-affinity
  across nodes and a 15 second termination grace period
- a ClusterIP Service on port 7423
- a PodDisruptionBudget of `minAvailable: 1`, because `maxUnavailable: 0`
  covers rollouts only and a drain could otherwise take both replicas at once

Pods run as user 65532 under `runAsNonRoot`, the `RuntimeDefault` seccomp
profile, a read-only root filesystem, and no privilege escalation, with every
capability dropped. Both probes hit `/version`, readiness every 5 seconds and
liveness every 20, and a container requests 50m CPU and 64Mi of memory, capped
at 256Mi. Service links are off: the Service is named `present`, so kubelet
would otherwise inject `PRESENT_PORT` and collide with the app's own prefix.

## prerequisites

- Rights to install a CRD, once per cluster. Everything else is namespaced.
- A namespace. Each overlay creates `present`; rename it in your own overlay.
- A pullable image. `ghcr.io/mad01/present` carries `X.Y.Z` and `latest` from
  a release and `main` plus `sha-<short>` from every push to main that touches
  present; pin a semver tag or a digest. Verifying an image and the package's
  ghcr.io visibility are in [docs/RELEASING.md](../../../docs/RELEASING.md).

## choosing an overlay

`overlays/kind` is for local testing: it pins the `ci` tag, never pulls, and
exposes nothing. `make -C services/present kind-test` builds the image, loads
it, applies this overlay, and runs the cluster tests over a port-forward.

`overlays/ingress` adds an Ingress driven by two `present-host` ConfigMap
literals. `host` is replaced into the Ingress host, the `tls` hosts, and the
`external-dns.alpha.kubernetes.io/hostname` annotation that creates the DNS
record; `ingressClass` goes into `spec.ingressClassName` and has to name a
class your cluster runs, or no controller claims the Ingress. The `tls` block
names `present-tls`, which cert-manager or a pre-created secret supplies, and
the controller sets `X-Forwarded-Proto` and `X-Forwarded-Host` for page URLs.

`overlays/istio` routes through an existing Gateway instead, reading the same
`present-host` ConfigMap. The value to patch is the gateway reference in
`virtualservice.yaml`, which ships pointing at `istio-system/internal-gateway`;
Istio sets the forwarded headers already.

Redefining the ConfigMap from an overlay layered on top of these changes
nothing, because the replacements already ran here: re-declare them beside
your own literals, or patch the Ingress and VirtualService fields directly.

## applying

```sh
kubectl apply -k services/present/deploy/overlays/ingress
kubectl -n present rollout status deploy/present
```

## verifying

```sh
kubectl -n present get pages
kubectl -n present logs deploy/present
```

`GET /version` on the hostname returns the four build keys (`version`,
`commit`, `tag`, `build_time`), the quickest way to tell which build serves.
`get pages` prints title, version, ephemeral flag, expiry, and age per page,
so the cluster is the debugging surface; a kept page carries `ephemeral:
false`, so that column reads `false` rather than blank.

Two log lines say a replica came up whole: `present: serving k8s pages in
namespace present on http://0.0.0.0:7423 (shared mode)` and `present:
sweeping expired pages every 10m0s`. Each sweep logs after that, one that
deleted nothing included: `present: sweep deleted=0 next in 10m0s`. Silence
past one interval means the sweeper goroutine is gone. Probe traffic stays
out of the access log, because a request whose User-Agent starts with
`kube-probe/` is skipped; a `/version` from a person or from `present doctor`
still shows up.

## upgrading

Bump the image tag in your overlay and apply again. The rolling update starts
a new pod before retiring an old one, so the hostname keeps answering, and
pages live in the API server rather than in a pod, so a roll loses nothing.

CRD changes stay additive within `v1alpha1`: apply `deploy/base` first, then
roll the Deployment, and objects written by the old build keep deserializing.

## removing

Delete the namespace. The CRD stays on purpose: removing it deletes every
`Page` object in every namespace, not only this instance's.

```sh
kubectl delete namespace present
```

## troubleshooting

`pages.present.thismoon.mad01.dev is forbidden` in the logs is the
RoleBinding, not the image: the service account lost its Role, or the pod is
running under a different account.

`the server could not find the requested resource` means the CRD is missing.
It ships in `deploy/base`, so apply that first on a fresh cluster.

`413 page too large` is the 1 MiB cap on a single page. The cluster store
keeps each page as one object and the API server caps those, so an
oversized page is refused at the API instead of failing mid-write.

`401` means no author key reached the server. `403` means one did and it was
not the key that created the page. Reads need neither.

`PRESENT_PORT: unparseable value "tcp://..."` at startup means service links
are still on, so kubelet injected the Service address under a name present
reads as its own. `enableServiceLinks: false` in the pod template fixes it.
