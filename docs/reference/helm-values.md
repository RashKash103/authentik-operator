# Helm values

Complete reference for every value accepted by the `authentik-operator` chart.

!!! danger "Do not edit this page by hand"

    Everything between the generated markers below is produced from
    `charts/authentik-operator/values.yaml` by
    [helm-docs](https://github.com/norwoodj/helm-docs). Hand edits are
    overwritten by the next run.

    To change what appears here, **edit the `# --` comment above the value** in
    `values.yaml`. Every key in that file already carries one explaining what it
    does and when you would change it.

## Regenerating

```sh
bin/helm-docs --chart-search-root charts
```

The `helm-docs` binary is installed into `bin/` by:

```sh
make helm-docs
```

!!! warning "No Makefile target runs helm-docs yet"

    `make helm-docs` installs the tool; it does not generate anything. No target
    wires its output into this page, so the generated block below is empty.

    Until that lands, the authoritative reference is `values.yaml` itself —
    every key is documented inline — and the chart's own generated
    `charts/authentik-operator/README.md`. You can also ask Helm:

    ```sh
    helm show values ./charts/authentik-operator
    ```

## The values you will actually set

A plain `helm install` needs no overrides; the defaults are production-sane.
These are the ones worth knowing about. The full list is in `values.yaml`.

| Value | Default | Why change it |
| --- | --- | --- |
| `replicaCount` | `1` | Leave it. Leader election means only one replica reconciles; a second buys faster failover and an idle Pod. |
| `image.repository` | the project's GHCR image | Mirroring into a private registry, or an air-gapped cluster. |
| `image.tag` | `""` (chart `appVersion`) | Pinning the operator independently of the chart version. |
| `image.digest` | `""` | Immutable, verifiable deploys. Wins over `image.tag` when set. |
| `watchNamespaces` | `[]` (all) | Smaller cache and blast radius. **Resources elsewhere are ignored silently.** |
| `log.level` | `info` | `debug` when diagnosing a reconcile. Noisy otherwise. |
| `log.format` | `json` | `console` for readable local output. |
| `leaderElection.enabled` | `true` | Leave it on — without it, a rolling update briefly runs two managers that fight. |
| `rbac.create` | `true` | RBAC managed out-of-band by a cluster admin. |
| `serviceAccount.create` | `true` | Your platform pre-provisions ServiceAccounts, e.g. for workload identity. |
| `serviceAccount.annotations` | `{}` | Cloud workload identity (`eks.amazonaws.com/role-arn`, `iam.gke.io/...`). |
| `metrics.enabled` | `true` | Turning metrics off; also sets the bind address to `0`. |
| `metrics.secure` | `true` | **Leave it on.** `false` serves plaintext metrics to anything that can reach the Pod. |
| `metrics.serviceMonitor.enabled` | `false` | You run Prometheus Operator. Requires the `monitoring.coreos.com/v1` CRDs, or the install fails on an unknown kind. |
| `metrics.serviceMonitor.labels` | `{}` | **Must match your `serviceMonitorSelector`**, or the ServiceMonitor is created and silently never scraped. |
| `enableHTTP2` | `false` | Leave it off — HTTP/2 has a history of DoS CVEs and these endpoints gain nothing from it. |
| `resources` | 10m/64Mi → 500m/256Mi | Thousands of managed resources, or visible OOMKills and CPU throttling. |
| `extraArgs` | `[]` | Escape hatch for a manager flag the chart does not model yet. |
| `extraEnv` | `[]` | `HTTPS_PROXY` for egress to authentik; `SSL_CERT_FILE` for a mounted CA bundle. |
| `extraVolumes` / `extraVolumeMounts` | `[]` | Mounting a private CA bundle. Use an `emptyDir` rather than relaxing `readOnlyRootFilesystem`. |
| `extraObjects` | `[]` | A `NetworkPolicy` or `PodDisruptionBudget` you want to live with the release. Strings pass through `tpl`. |
| `nodeSelector`, `tolerations`, `affinity` | `{}` / `[]` | Keeping the operator on a system node pool. With `replicaCount: 2`, add anti-affinity. |
| `priorityClassName` | `""` | Surviving node-pressure eviction ahead of workload Pods. The class must already exist. |

!!! warning "`watchNamespaces` is the one that surprises people"

    Resources outside the watched set get no event, no condition, and an empty
    `status` — the operator never sees them. If a resource seems completely
    inert, check this value before anything else.

    `ClusterAuthentikConnection` is cluster-scoped and watched regardless.

### Security-relevant defaults

These mirror `config/manager` and satisfy the Kubernetes `restricted` Pod
Security Standard. Change them only if a policy engine in your cluster demands
something different.

```yaml
podSecurityContext:
  runAsNonRoot: true
  seccompProfile:
    type: RuntimeDefault

securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop:
      - ALL
```

!!! danger "CRDs are not upgraded by Helm"

    No value changes this. Helm applies `crds/` on first install only. See
    [Installation](../getting-started/installation.md#upgrading-the-chart-crds-are-your-job).

## Generated reference

<!-- BEGIN GENERATED: helm-values -->
<!-- END GENERATED: helm-values -->
