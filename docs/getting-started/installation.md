# Installation

Both install paths give you the same thing: the CRDs for the
`authentik.k8s.rka.sh` API group, plus a single-replica controller-manager
`Deployment` that runs as non-root with a read-only root filesystem and every
Linux capability dropped.

!!! info "Installing the operator does not install authentik"

    This chart and these manifests install the operator only. No authentik
    server, no worker, no PostgreSQL, no Redis. Point the operator at an
    authentik instance you already run.

## Helm

The chart is published to GHCR as an OCI artifact on every tagged release, so
there is no chart repository to add.

=== "From the registry (recommended)"

    ```sh
    helm install authentik-operator \
      oci://ghcr.io/rashkash103/charts/authentik-operator \
      --version 0.3.0 \
      --namespace authentik-operator-system \
      --create-namespace
    ```

    Inspect it first if you would rather:

    ```sh
    helm show values oci://ghcr.io/rashkash103/charts/authentik-operator --version 0.3.0
    helm show crds   oci://ghcr.io/rashkash103/charts/authentik-operator --version 0.3.0
    ```

=== "From a checkout"

    ```sh
    helm install authentik-operator ./charts/authentik-operator \
      --namespace authentik-operator-system \
      --create-namespace
    ```

!!! warning "Pin the version to your authentik"

    One operator release targets one authentik version. Always pass
    `--version`, and pick the release matching the authentik you run — see
    [Supported versions](../operations/supported-versions.md).

!!! danger "Helm never upgrades CRDs"

    Helm installs everything in a chart's `crds/` directory and then leaves it
    alone forever. `helm upgrade` will not touch your CRDs, so a chart upgrade
    can silently leave you on a stale API. Apply them yourself:

    ```sh
    helm show crds oci://ghcr.io/rashkash103/charts/authentik-operator \
      --version <new-version> | kubectl apply --server-side -f -
    ```

    [Flux](../guides/gitops-flux.md) handles this with
    `upgrade.crds: CreateReplace`.

A plain install needs no overrides: the defaults are production-sane. The values
below are the ones people actually reach for; the full list is in
[Helm values](../reference/helm-values.md).

### Common overrides

| Value | Default | Why you would change it |
| --- | --- | --- |
| `image.repository` | the project's GHCR image | Mirroring into a private registry or an air-gapped cluster. |
| `image.tag` | `""`, meaning the chart's `appVersion` | Pinning the operator independently of the chart. |
| `image.digest` | `""` | Immutable, supply-chain-verifiable deploys. When set it wins over `image.tag`. |
| `watchNamespaces` | `[]`, meaning all namespaces | Shrinking cache memory and blast radius. |
| `log.level` | `info` | `debug` while diagnosing a reconcile. Noisy in steady state. |
| `log.format` | `json` | `console` for human-readable local debugging. |
| `metrics.enabled` | `true` | Turning metrics off entirely. |
| `metrics.secure` | `true` | Leave it on. `false` serves plaintext metrics to anything that can reach the Pod. |
| `metrics.serviceMonitor.enabled` | `false` | You run Prometheus Operator. Requires the `monitoring.coreos.com/v1` CRDs to exist first. |
| `leaderElection.enabled` | `true` | Leave it on. Without it, the overlap during a rolling update has two managers fighting. |
| `rbac.create` | `true` | RBAC is managed out-of-band by a cluster admin. |
| `resources` | 10m/64Mi requests, 500m/256Mi limits | You manage thousands of resources, or see OOMKills. |
| `extraArgs` | `[]` | Escape hatch for a manager flag the chart does not yet model. |
| `extraEnv` | `[]` | `HTTPS_PROXY` for egress to authentik, or `SSL_CERT_FILE` for a mounted CA bundle. |
| `extraVolumes` / `extraVolumeMounts` | `[]` | Mounting a private CA bundle for the authentik endpoint. |

Restricting the operator to two namespaces:

```sh
helm install authentik-operator ./charts/authentik-operator \
  --namespace authentik-operator-system --create-namespace \
  --set 'watchNamespaces={apps,platform}'
```

!!! warning "`watchNamespaces` fails silently by design"

    Resources outside the watched set are ignored with no event and no
    condition — the operator never sees them. If a resource seems inert and its
    `status` is completely empty, check this value first.

    `ClusterAuthentikConnection` is cluster-scoped and is watched regardless.

### Upgrading the chart: CRDs are your job

!!! danger "Helm never upgrades CRDs"

    Helm applies everything in the chart's `crds/` directory exactly once, on
    **first install**. On `helm upgrade` it does nothing with them; on
    `helm uninstall` it leaves them behind.

    The failure mode is silent. The Deployment rolls, the operator starts,
    nothing logs an error — but the API server still validates against the old
    schema, so new fields are pruned from objects you apply and the operator
    sees requests it cannot act on.

Apply the CRDs yourself, **before** `helm upgrade`:

=== "From a packaged chart"

    ```sh
    helm show crds authentik-operator/authentik-operator --version <new-version> \
      | kubectl apply --server-side -f -
    ```

=== "From a checkout"

    ```sh
    kubectl apply --server-side -f charts/authentik-operator/crds/
    ```

Use `--server-side`, and add `--force-conflicts` if an earlier client-side apply
left a large `last-applied-configuration` annotation. Client-side apply can fail
on these CRDs with `metadata.annotations: Too long`.

The chart's `crds/` directory may be empty in a fresh checkout where
`just manifests` has not run. That is expected, not an error — Helm treats it as
"no CRDs to install".

A release that changes the API needs more than new CRDs — see
[Upgrading](../operations/upgrading.md) for the per-release migration. Both
v0.2.0 and v0.3.0 require manifest changes.

## Raw manifests

Render a single `install.yaml` from the kustomize configuration:

```sh
just bundle                 # writes dist/install.yaml
kubectl apply -f dist/install.yaml
```

Override the image with `IMG`:

```sh
IMG=ghcr.io/example/authentik-operator:v0.3.0 just bundle
```

!!! note

    `just bundle` and `just deploy` both run `kustomize edit set image` inside
    `config/manager`, which modifies `config/manager/kustomization.yaml` in your
    working tree. Check `git status` afterwards.

## From a checkout

```sh
just install     # CRDs only
just deploy      # CRDs + controller-manager
just undeploy    # remove the controller-manager
just uninstall   # remove the CRDs
```

`just run` runs the manager outside the cluster against your current kubeconfig,
which is the fastest loop while developing. Install the CRDs first.

## Manager flags

The `Deployment` runs `/manager`. The Helm chart renders most of these from
values; the raw manifests set `--leader-elect`,
`--health-probe-bind-address=:8081` and `--metrics-bind-address=:8443`.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--metrics-bind-address` | `0` | Metrics endpoint address. `:8443` for HTTPS, `:8080` for HTTP, `0` to disable. |
| `--health-probe-bind-address` | `:8081` | Address for `/healthz` and `/readyz`. |
| `--leader-elect` | `false` | Enable leader election so only one manager is active. |
| `--metrics-secure` | `true` | Serve metrics over HTTPS with authn/authz. |
| `--enable-http2` | `false` | Enable HTTP/2 on the metrics and webhook servers. Off deliberately. |
| `--watch-namespaces` | `""` | Comma-separated namespaces to watch. Empty means all. |

The controller-runtime zap flags — `--zap-log-level`, `--zap-encoder`,
`--zap-devel`, `--zap-stacktrace-level`, `--zap-time-encoding` — are registered
too. The chart drives the first two through `log.level` and `log.format`.

!!! note "Why HTTP/2 is off by default"

    HTTP/2 has a history of denial-of-service CVEs — Rapid Reset and relatives —
    and the operator's own endpoints gain nothing from it. Turn it on only if a
    scraper demands it.

## Verifying the install

```sh
kubectl -n authentik-operator-system rollout status deploy/authentik-operator --timeout=120s
kubectl -n authentik-operator-system logs deploy/authentik-operator -f
```

A healthy start logs `starting manager`, and with leader election enabled,
`successfully acquired lease`. Confirm the CRDs registered:

```sh
kubectl get crd | grep authentik.k8s.rka.sh
```

You should see `authentikconnections.authentik.k8s.rka.sh` and
`clusterauthentikconnections.authentik.k8s.rka.sh`. Both kinds have short names:
`akconn` and `clakconn`.

## Uninstalling

```sh
helm uninstall authentik-operator -n authentik-operator-system
```

CRDs are deliberately left behind. Deleting a CRD deletes every object of that
kind cluster-wide, with no undo. Remove them only when you are certain:

```sh
kubectl delete crd authentikconnections.authentik.k8s.rka.sh
kubectl delete crd clusterauthentikconnections.authentik.k8s.rka.sh
```

Deleting the operator does **not** delete anything in authentik. Objects it
created stay exactly as they were.

## Next

[Create an authentik API token](api-token.md) and give the operator something to
talk to.
