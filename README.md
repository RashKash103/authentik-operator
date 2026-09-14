> [!WARNING]
> **This project is fully AI generated.** Review it carefully before running it
> against anything you care about, and use it at your own risk.

# authentik-operator

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
  This table is generated from supported-versions.yaml, which is the single
  source of truth. CI verifies the table below against that file; if you edit
  one, edit the other. authentik ships CalVer minor series (YYYY.M) and this
  project supports the three most recent series, pinning an exact patch tag per
  series so CI is reproducible.
-->

<!-- BEGIN SUPPORTED-VERSIONS -->
| authentik series | Tested image                          | Status    |
| ---------------- | ------------------------------------- | --------- |
| `2026.8`         | `ghcr.io/goauthentik/server:2026.8.2` | Supported |
| `2026.5`         | `ghcr.io/goauthentik/server:2026.5.7` | Supported |
| `2026.2`         | `ghcr.io/goauthentik/server:2026.2.7` | Supported |
<!-- END SUPPORTED-VERSIONS -->

- **Minimum:** `2026.2`. The operator refuses to reconcile against anything older.
- **Maximum tested:** `2026.8`. Newer versions still reconcile, but the operator
  logs a warning that it is running untested.

## Implementation status

This project is early. The API group is `authentik.k8s.rka.sh`, the API version is
`v1alpha1`, and **nothing below is implemented yet** — the table is the plan of
record, not a feature list. Treat every row marked _Planned_ as design
documentation.

| Kind                          | Scope      | Status  |
| ----------------------------- | ---------- | ------- |
| `AuthentikConnection`         | Namespaced | Planned |
| `ClusterAuthentikConnection`  | Cluster    | Planned |
| `OAuth2Provider`              | Namespaced | Planned |
| `SAMLProvider`                | Namespaced | Planned |
| `ProxyProvider`               | Namespaced | Planned |
| `Application`                 | Namespaced | Planned |
| `Outpost`                     | Namespaced | Planned |
| `KubernetesServiceConnection` | Namespaced | Planned |
| `DockerServiceConnection`     | Namespaced | Planned |

The manager binary builds and runs today, but it registers no controllers. The
`v1alpha1` API is unstable: expect breaking changes without conversion webhooks
until it graduates to `v1beta1`.

## Installation

Both paths install the CRDs and a single-replica controller-manager `Deployment`
that runs as non-root with a read-only root filesystem and all capabilities
dropped.

### Helm

The chart lives in this repository at [`charts/authentik-operator`](charts/authentik-operator).

```sh
helm install authentik-operator ./charts/authentik-operator \
  --namespace authentik-operator-system \
  --create-namespace
```

<!-- PLACEHOLDER: no chart repository is published yet. Once one exists, add the
     `helm repo add` instructions here and replace the local-path example. -->

### Raw manifests

Render a single `install.yaml` from the kustomize configuration and apply it:

```sh
make bundle          # writes dist/install.yaml
kubectl apply -f dist/install.yaml
```

Set a different image with `IMG`:

```sh
make bundle IMG=ghcr.io/rashkash103/authentik-operator:v0.1.0
```

### From a checkout

```sh
make install         # CRDs only
make deploy          # CRDs + controller-manager
make undeploy        # remove the controller-manager
make uninstall       # remove the CRDs
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

> [!NOTE]
> The CRDs below are not implemented yet. This section shows the intended shape
> of the API so the design can be reviewed; the manifests will not reconcile
> against the current build.

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

**3. Declare a provider and an application.**

```yaml
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
  redirectURIs:
    - https://grafana.example.com/login/generic_oauth
  # The operator writes the generated client ID and secret here.
  credentialsSecretRef:
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
  displayName: Grafana
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
        key: clientID
  - name: GF_AUTH_GENERIC_OAUTH_CLIENT_SECRET
    valueFrom:
      secretKeyRef:
        name: grafana-oidc
        key: clientSecret
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

- [ADR 0001 — Connection scope](docs/decisions/0001-connection-scope.md): why both a namespaced and a cluster-scoped connection kind ship.
- [ADR 0002 — Adoption policy](docs/decisions/0002-adoption-policy.md): why `FailOnConflict` is the default.
- [SECURITY.md](SECURITY.md): threat model, credential blast radius, hardening, and how to report a vulnerability.
- [CONTRIBUTING.md](CONTRIBUTING.md): development setup, test tiers, and the `make verify` contract.

<!-- PLACEHOLDER: no published documentation site exists yet. Add the link here
     if one is set up. -->

## Contributing

Bug reports, design feedback and pull requests are welcome. Start with
[CONTRIBUTING.md](CONTRIBUTING.md) — it covers the development environment, the
real Makefile targets, and the three test tiers (unit, envtest, E2E).

Because every line of this project was AI generated, careful human review is the
most valuable contribution available. Reviews of the design documents in
`docs/decisions/` are as welcome as code.

## Licence

Apache License 2.0. See [LICENSE](LICENSE).
