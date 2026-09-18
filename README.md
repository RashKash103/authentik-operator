# authentik-operator

[![CI](https://github.com/RashKash103/authentik-operator/actions/workflows/ci.yaml/badge.svg?branch=main)](https://github.com/RashKash103/authentik-operator/actions/workflows/ci.yaml)
[![E2E](https://github.com/RashKash103/authentik-operator/actions/workflows/e2e.yaml/badge.svg?branch=main)](https://github.com/RashKash103/authentik-operator/actions/workflows/e2e.yaml)
[![Charts](https://github.com/RashKash103/authentik-operator/actions/workflows/charts.yaml/badge.svg?branch=main)](https://github.com/RashKash103/authentik-operator/actions/workflows/charts.yaml)
[![Documentation](https://img.shields.io/badge/docs-rashkash103.github.io-3f51b5?logo=materialformkdocs&logoColor=white)](https://rashkash103.github.io/authentik-operator/)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)

> [!WARNING]
> **This project is fully AI generated.** Review it carefully before running it
> against anything you care about, and use it at your own risk.

A Kubernetes operator that manages objects **inside an existing
[authentik](https://goauthentik.io/) instance** — providers, applications and
outposts — declaratively, as Kubernetes custom resources. It talks to authentik
over its REST API using a token you supply.

> [!IMPORTANT]
> **This operator does not deploy, install, upgrade or otherwise manage
> authentik itself.** The name invites the opposite assumption, so to be blunt:
> you bring your own authentik (Helm chart, Docker Compose, hosted, whatever),
> and this operator configures what lives *inside* it. If you want authentik
> deployed on Kubernetes, use the [official authentik Helm
> chart](https://goauthentik.io/docs/installation/kubernetes) and point this
> operator at the result.

## What it does

- Reconciles authentik **providers** (OAuth2/OIDC, SAML, Proxy) from Kubernetes manifests.
- Reconciles authentik **applications** and binds them to a provider.
- Reconciles authentik **outposts** and their service connections (Kubernetes and Docker).
- Writes generated credentials — notably OAuth2 client IDs and secrets — back
  into Kubernetes `Secret`s so workloads can consume them.
- Targets an authentik instance through a **connection object** that holds the
  API URL and a reference to a `Secret` containing an API token.

## What it does **not** do

- It does **not** install, upgrade, back up, or operate the authentik server,
  worker, PostgreSQL or Redis.
- It does **not** manage authentik users, groups or sources (not in scope for
  `v1alpha1`).
- It does **not** manage authentik flows, stages, policies or property mappings
  (not in scope for `v1alpha1`).
- It does **not** import your entire existing authentik configuration. It only
  touches objects it created, unless you explicitly opt in to adoption — see
  [ADR 0002](docs/decisions/0002-adoption-policy.md).

## Supported authentik versions

<!--
  The table and bounds below are generated from supported-versions.yaml, which
  is the single source of truth. `just sync-versions` rewrites them and
  `just verify` fails if they drift.
-->

<!-- BEGIN SUPPORTED-VERSIONS -->
| authentik series | Tested image                          | Status    |
| ---------------- | ------------------------------------- | --------- |
| `2026.8`         | `ghcr.io/goauthentik/server:2026.8.2` | Supported |
<!-- END SUPPORTED-VERSIONS -->

<!-- BEGIN SUPPORTED-VERSIONS-BOUNDS -->
- **Minimum:** `2026.8`. The operator refuses to reconcile against anything older, and says so in a condition.
- **Maximum tested:** `2026.8`. Newer versions still reconcile, but the operator logs a warning that it is running untested.
<!-- END SUPPORTED-VERSIONS-BOUNDS -->

### Version policy

**One operator release targets one authentik version.** Each release is built
against a specific authentik series and pinned to the API client generated from
it. When authentik ships a release the current client cannot serve, that is not
a patch — it is a **new versioned release of the operator and its Helm chart**,
targeting the new authentik.

So:

- **Track the latest.** The newest operator release always targets the newest
  supported authentik.
- **Pin to match your authentik.** Running an older authentik means staying on
  the operator release built for it. Both the image and the chart are versioned
  and immutable, so an older pairing keeps working; it just stops receiving new
  features.
- **Upgrade in step.** Moving authentik across a boundary means moving the
  operator with it. The compatibility matrix below says which pairs.

<!-- BEGIN COMPATIBILITY-MATRIX -->
| Operator / chart | authentik | Notes                                                             |
| ---------------- | --------- | ----------------------------------------------------------------- |
| `0.1.x`          | `2026.8`  | First release. API version `v1alpha1`.                            |
| `0.2.x`          | `2026.8`  | References became objects; `signingKey` became `signingKeyPair`.  |
| `0.3.x`          | `2026.8`  | Providers declare their outposts; `Outpost.providerRefs` removed. |
<!-- END COMPATIBILITY-MATRIX -->

The operator checks the version it is talking to at runtime. Below the
supported floor it refuses to reconcile and says so in a condition, rather than
failing obscurely somewhere inside an API call. Above the tested ceiling it
proceeds but warns.

> [!NOTE]
> **Why a release targets one version rather than a range**, in case it looks
> unnecessarily strict: `goauthentik.io/api/v3` is generated from a single
> authentik release and enforces *that* release's required properties when
> decoding. The client pinned here marks `Application.pbm_uuid` and
> `SAMLProvider.url_issuer` as required, and neither exists in 2026.5 or
> 2026.2, so every list call against those versions fails outright. The E2E
> matrix found this the hard way: 2026.8 passed while 2026.5 and 2026.2 failed.
>
> Supporting a genuine range would mean pinning a client generated from the
> *oldest* version to support, or decoding tolerantly instead of through the
> generated models. Until then, pinning per release is the honest arrangement.
>
> `just check-api-drift` compares authentik's published schema against the
> pinned series and reports exactly which fields would break, so the next one
> is found before a release carries it rather than by the E2E matrix failing. A
> [scheduled workflow](.github/workflows/api-drift.yaml) runs it weekly against
> authentik's unreleased tip.

## Implementation status

This project is early. The API group is `authentik.k8s.rka.sh` and the API
version is `v1alpha1`, which is unstable: expect breaking changes without
conversion webhooks until it graduates to `v1beta1`.

<!-- BEGIN IMPLEMENTATION-STATUS -->
| Kind                          | Scope      | Short names  |
| ----------------------------- | ---------- | ------------ |
| `ClusterAuthentikConnection`  | Cluster    | `clakconn`   |
| `Application`                 | Namespaced | `akapp`      |
| `AuthentikConnection`         | Namespaced | `akconn`     |
| `CertificateKeyPair`          | Namespaced | `akkeypair`  |
| `DockerServiceConnection`     | Namespaced | `akdockersc` |
| `Flow`                        | Namespaced | `akflow`     |
| `KubernetesServiceConnection` | Namespaced | `akk8ssc`    |
| `OAuth2Provider`              | Namespaced | `akoauth2`   |
| `Outpost`                     | Namespaced | `akoutpost`  |
| `PropertyMapping`             | Namespaced | `akmapping`  |
| `ProxyProvider`               | Namespaced | `akproxy`    |
| `SAMLProvider`                | Namespaced | `aksaml`     |
<!-- END IMPLEMENTATION-STATUS -->

Every kind reconciles and is exercised against a real authentik in CI. What is
still thin is coverage of the less common fields, and there is no conversion
webhook story yet.

## Installation

Both paths install the CRDs and a single-replica controller-manager `Deployment`
that runs as non-root with a read-only root filesystem and all capabilities
dropped.

### Helm (OCI)

The chart is published to GHCR as an OCI artifact on every tagged release.
There is no chart repository to add — Helm pulls it from the registry directly:

```sh
helm install authentik-operator \
  oci://ghcr.io/rashkash103/charts/authentik-operator \
  --version 0.3.0 \
  --namespace authentik-operator-system \
  --create-namespace
```

Pin `--version` to the release matching your authentik; see
[Version policy](#version-policy). Inspect before installing with:

```sh
helm show values oci://ghcr.io/rashkash103/charts/authentik-operator --version 0.3.0
helm show crds   oci://ghcr.io/rashkash103/charts/authentik-operator --version 0.3.0
```

The chart is also attached to each GitHub release as a `.tgz`, and the source
lives at [`charts/authentik-operator`](charts/authentik-operator) if you would
rather install from a checkout:

```sh
helm install authentik-operator ./charts/authentik-operator \
  --namespace authentik-operator-system --create-namespace
```

> [!IMPORTANT]
> Helm installs everything in a chart's `crds/` directory but **never upgrades
> or deletes it**. Upgrading the chart therefore leaves CRDs untouched. Apply
> them yourself on upgrade:
>
> ```sh
> helm show crds oci://ghcr.io/rashkash103/charts/authentik-operator \
>   --version <new-version> | kubectl apply --server-side -f -
> ```
>
> Flux handles this for you with `upgrade.crds: CreateReplace`; see
> [Deploying with Flux](examples/flux).

### GitOps with Flux

Runnable manifests live in [`examples/flux`](examples/flux): an `OCIRepository`
and `HelmRelease` for the operator, and a `Kustomization` for the authentik
resources it manages, kept as separate reconciliations so a bad application
manifest cannot block an operator upgrade.

### Raw manifests

Render a single `install.yaml` from the kustomize configuration and apply it:

```sh
just bundle          # writes dist/install.yaml
kubectl apply -f dist/install.yaml
```

Set a different image with `IMG`:

```sh
IMG=ghcr.io/rashkash103/authentik-operator:v0.3.0 just bundle
```

### From a checkout

```sh
just install         # CRDs only
just deploy          # CRDs + controller-manager
just undeploy        # remove the controller-manager
just uninstall       # remove the CRDs
```

### Manager flags

The `Deployment` runs `/manager` with `--leader-elect`,
`--health-probe-bind-address=:8081` and `--metrics-bind-address=:8443`. The full
set the binary accepts:

| Flag                          | Default | Meaning                                                                  |
| ----------------------------- | ------- | ------------------------------------------------------------------------ |
| `--metrics-bind-address`      | `0`     | Metrics endpoint address. `:8443` for HTTPS, `:8080` for HTTP, `0` off.   |
| `--health-probe-bind-address` | `:8081` | Address for `/healthz` and `/readyz`.                                     |
| `--leader-elect`              | `false` | Enable leader election so only one manager is active.                     |
| `--metrics-secure`            | `true`  | Serve metrics over HTTPS with authn/authz.                                |
| `--enable-http2`              | `false` | Enable HTTP/2 on the metrics and webhook servers. Off by default on purpose. |
| `--watch-namespaces`          | `""`    | Comma-separated namespaces to watch. Empty means all namespaces.          |

The controller-runtime zap logging flags (`--zap-log-level`, `--zap-encoder`,
`--zap-devel`, `--zap-stacktrace-level`, `--zap-time-encoding`) are also
registered.

## Quickstart



**1. Store an authentik API token.** Create one in authentik under
_Directory → Tokens and App passwords_, scoped as narrowly as your use case
allows (see [SECURITY.md](SECURITY.md)).

```sh
kubectl create secret generic authentik-api-token \
  --namespace my-apps \
  --from-literal=token='ak-...'
```

**2. Point the operator at your authentik.**

```yaml
apiVersion: authentik.k8s.rka.sh/v1alpha1
kind: AuthentikConnection
metadata:
  name: default
  namespace: my-apps
spec:
  url: https://authentik.example.com
  tokenSecretRef:
    name: authentik-api-token
    key: token
```

**3. Declare the flows a provider needs, then the provider and application.**

A provider references `Flow` resources rather than naming slugs inline, so that
swapping which flow every provider authorises against is one edit. See
[References between resources](https://rashkash103.github.io/authentik-operator/guides/references/).

```yaml
apiVersion: authentik.k8s.rka.sh/v1alpha1
kind: Flow
metadata:
  name: provider-authorization
  namespace: my-apps
spec:
  connectionRef:
    name: default
  existingSlug: default-provider-authorization-implicit-consent
---
apiVersion: authentik.k8s.rka.sh/v1alpha1
kind: Flow
metadata:
  name: provider-invalidation
  namespace: my-apps
spec:
  connectionRef:
    name: default
  existingSlug: default-provider-invalidation-flow
---
apiVersion: authentik.k8s.rka.sh/v1alpha1
kind: OAuth2Provider
metadata:
  name: grafana
  namespace: my-apps
spec:
  connectionRef:
    kind: AuthentikConnection
    name: default
  adoptionPolicy: FailOnConflict
  authorizationFlow:
    name: provider-authorization
  invalidationFlow:
    name: provider-invalidation
  redirectURIs:
    - matchingMode: strict
      url: https://grafana.example.com/login/generic_oauth
  # The operator writes the generated client ID and secret here.
  writeCredentialsTo:
    name: grafana-oidc
---
apiVersion: authentik.k8s.rka.sh/v1alpha1
kind: Application
metadata:
  name: grafana
  namespace: my-apps
spec:
  connectionRef:
    kind: AuthentikConnection
    name: default
  slug: grafana
  name: Grafana
  providerRef:
    kind: OAuth2Provider
    name: grafana
```

**4. Consume the generated credentials.**

```yaml
env:
  - name: GF_AUTH_GENERIC_OAUTH_CLIENT_ID
    valueFrom:
      secretKeyRef:
        name: grafana-oidc
        key: client-id
  - name: GF_AUTH_GENERIC_OAUTH_CLIENT_SECRET
    valueFrom:
      secretKeyRef:
        name: grafana-oidc
        key: client-secret
```

## CRD overview

All kinds live in the `authentik.k8s.rka.sh/v1alpha1` API group.

### Connections

| Kind                         | Scope      | Purpose                                                                                       |
| ---------------------------- | ---------- | --------------------------------------------------------------------------------------------- |
| `AuthentikConnection`        | Namespaced | An authentik endpoint plus a token `Secret` in the **same namespace**. The safe default.        |
| `ClusterAuthentikConnection` | Cluster    | A cluster-wide authentik endpoint whose token `Secret` lives in an explicitly named namespace.  |

Every other kind references one of these via `spec.connectionRef`, which carries
a `kind` so a namespaced resource can choose either variant.

> [!CAUTION]
> `ClusterAuthentikConnection` names the namespace its token `Secret` lives in,
> so anyone who can create one can cause the operator to read a `Secret` from
> **any namespace in the cluster**. Permission to create these objects is
> effectively cluster-admin-level. See [SECURITY.md](SECURITY.md) and
> [ADR 0001](docs/decisions/0001-connection-scope.md).

### Providers and applications

| Kind             | Purpose                                                                          |
| ---------------- | -------------------------------------------------------------------------------- |
| `OAuth2Provider` | An OAuth2/OIDC provider. Writes the generated client ID and secret to a `Secret`. |
| `SAMLProvider`   | A SAML provider, including its signing configuration.                             |
| `ProxyProvider`  | A forward-auth/proxy provider fronted by an outpost.                              |
| `Application`    | An authentik application, bound to exactly one provider.                          |

### Outposts

| Kind                          | Purpose                                                                    |
| ----------------------------- | -------------------------------------------------------------------------- |
| `Outpost`                     | An authentik outpost and the set of providers assigned to it.               |
| `KubernetesServiceConnection` | Lets authentik deploy a managed outpost into a Kubernetes cluster.          |
| `DockerServiceConnection`     | Lets authentik deploy a managed outpost onto a Docker host.                 |

### Common `spec` fields

| Field            | Applies to      | Notes                                                                                   |
| ---------------- | --------------- | --------------------------------------------------------------------------------------- |
| `connectionRef`  | all but connections | Selects the target authentik. `kind` is `AuthentikConnection` or `ClusterAuthentikConnection`. |
| `adoptionPolicy` | all but connections | `FailOnConflict` (default) or `AdoptExisting`. See [ADR 0002](docs/decisions/0002-adoption-policy.md). |

## Documentation

**📖 [rashkash103.github.io/authentik-operator](https://rashkash103.github.io/authentik-operator/)** — installation, usage guides, reference schemas and troubleshooting.

Built with [Zensical](https://zensical.org/) from `docs/`, and published by the
`Documentation` workflow on every push to `main`.

- [ADR 0001 — Connection scope](docs/decisions/0001-connection-scope.md): why both a namespaced and a cluster-scoped connection kind ship.
- [ADR 0002 — Adoption policy](docs/decisions/0002-adoption-policy.md): why `FailOnConflict` is the default.
- [SECURITY.md](SECURITY.md): threat model, credential blast radius, hardening, and how to report a vulnerability.
- [CONTRIBUTING.md](CONTRIBUTING.md): development setup, test tiers, and the `just verify` contract.

## Contributing

Bug reports, design feedback and pull requests are welcome. Start with
[CONTRIBUTING.md](CONTRIBUTING.md) — it covers the development environment, the
real `just` recipes, and the three test tiers (unit, envtest, E2E).

Because every line of this project was AI generated, careful human review is the
most valuable contribution available. Reviews of the design documents in
`docs/decisions/` are as welcome as code.

## Licence

Apache License 2.0. See [LICENSE](LICENSE).
