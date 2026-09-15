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

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` | Affinity/anti-affinity rules. With `replicaCount: 2` add pod anti-affinity so both replicas do not land on the same node, defeating the point. |
| allowCrossNamespaceReferences | bool | `false` | Allow a Flow, PropertyMapping, CertificateKeyPair, provider or service connection reference to name another namespace. Off by default.  Turning it on makes one namespace's configuration another's dependency, which is a cluster-level decision rather than one a manifest author should make for themselves. It does NOT apply to `connectionRef`: credentials never cross a namespace boundary this way. Share a connection with a ClusterAuthentikConnection instead, where the owner grants access through `allowedNamespaces`. |
| dnsConfig | object | `{}` | Custom DNS settings, used when `dnsPolicy: None`. |
| dnsPolicy | string | `""` | DNS policy for the Pod. Change to `None` (plus `dnsConfig`) only in clusters with unusual DNS topology. |
| enableHTTP2 | bool | `false` | Enable HTTP/2 on the metrics endpoint (`--enable-http2`). Off by default: HTTP/2 has a history of DoS-shaped CVEs (Rapid Reset) and the operator's endpoints gain nothing from it. Turn on only if a scraper requires it. |
| extraArgs | list | `[]` | Extra command-line flags appended to `/manager`, e.g. `["--kube-api-qps=50"]`. The chart already renders every flag it models; use this as an escape hatch for flags added after this chart version. |
| extraEnv | list | `[]` | Extra environment variables for the manager container, in full Kubernetes `env` form (so `valueFrom` works). Typical uses: `HTTPS_PROXY`/`NO_PROXY` for egress to authentik, or `SSL_CERT_FILE` pointing at a mounted CA bundle. Example:   extraEnv:     - name: HTTPS_PROXY       value: http://proxy.internal:3128     - name: SSL_CERT_FILE       value: /etc/ssl/custom/ca.crt |
| extraObjects | list | `[]` | Extra arbitrary Kubernetes manifests rendered with the release. Strings are passed through `tpl`, so Helm templating works inside them. Use for a NetworkPolicy or PodDisruptionBudget you want to live with the release. |
| extraVolumeMounts | list | `[]` | Extra volume mounts on the manager container, matching `extraVolumes`. |
| extraVolumes | list | `[]` | Extra volumes on the Pod, in full Kubernetes `volumes` form. Most often a Secret or ConfigMap holding a private CA bundle for the authentik endpoint, or an `emptyDir` to give a writable path alongside the read-only rootfs. |
| fullnameOverride | string | `""` | Replace the generated `<release>-<chart>` resource name entirely. Use when you must match an existing naming convention or shorten names that would otherwise exceed the 63-character label limit. |
| health.port | int | `8081` | Container port for the health/readiness endpoints (`/healthz`, `/readyz`). Change only on a port collision with an injected sidecar. |
| hostAliases | list | `[]` | Host aliases injected into /etc/hosts. Occasionally needed when the authentik URL resolves only via a private DNS the cluster cannot reach. |
| image.digest | string | `""` | Image digest (e.g. `sha256:abc...`). When set it wins over `tag` and the image is referenced as `repository@digest`. Use this for immutable, supply-chain-verifiable deploys. |
| image.pullPolicy | string | `"IfNotPresent"` | Image pull policy. `IfNotPresent` is right for released, immutable tags. Use `Always` only when you deploy a moving tag such as `main` or `latest`. |
| image.repository | string | `"ghcr.io/rashkash103/authentik-operator"` | Operator image repository. Change only when mirroring into a private registry (air-gapped clusters, or a pull-through cache). |
| image.tag | string | `""` | Image tag. Empty string means "use the chart's appVersion", which keeps chart and operator in lockstep. Pin an explicit tag - or better, a digest via `image.digest` - if you promote images independently of chart versions. |
| imagePullSecrets | list | `[]` | Names of pre-existing image pull Secrets, e.g. `[{name: ghcr-creds}]`. Needed only when `image.repository` is private. The Secret must already exist in the release namespace; this chart does not create it. |
| leaderElection.enabled | bool | `true` | Enable leader election (`--leader-elect`). Keep true: with it off, two replicas - including the brief overlap during a rolling update - would reconcile the same objects and fight each other. |
| livenessProbe | object | `{"failureThreshold":3,"initialDelaySeconds":15,"periodSeconds":20,"successThreshold":1,"timeoutSeconds":1}` | Liveness probe for the manager. Tune `initialDelaySeconds` upward on very slow nodes where the cache takes a while to sync. |
| log.format | string | `"json"` | Log encoding: `json` for log pipelines (Loki, ELK, Datadog), `console` for human-readable output while debugging locally (maps to `--zap-encoder`). |
| log.level | string | `"info"` | Log verbosity: `debug`, `info` or `error` (maps to `--zap-log-level`). Use `debug` when diagnosing reconcile behaviour; it is noisy in steady state. |
| metrics.enabled | bool | `true` | Serve controller-runtime metrics. When false the metrics bind address is set to `0` (disabled) and no Service is created. |
| metrics.port | int | `8443` | Port the metrics endpoint binds to inside the container. |
| metrics.secure | bool | `true` | Serve metrics over HTTPS with authn/authz (`--metrics-secure`). Keep true in shared clusters: it forces scrapers to present a token that passes a SubjectAccessReview. Setting false serves plaintext HTTP to anything that can reach the Pod - only acceptable on an isolated network. |
| metrics.service.annotations | object | `{}` | Extra annotations on the metrics Service, e.g. annotation-based scrape config for agents that do not use ServiceMonitors. |
| metrics.service.enabled | bool | `true` | Create a Service for the metrics endpoint. Disable if you scrape Pods directly (e.g. a PodMonitor or a DaemonSet agent using Pod annotations). |
| metrics.service.labels | object | `{}` | Extra labels on the metrics Service. Set these when your Prometheus `serviceMonitorSelector` matches on Service labels. |
| metrics.service.port | int | `8443` | Service port. Usually mirrors `metrics.port`. |
| metrics.service.type | string | `"ClusterIP"` | Service type. `ClusterIP` is correct; metrics should never be exposed outside the cluster. |
| metrics.serviceMonitor.annotations | object | `{}` | Annotations on the ServiceMonitor. |
| metrics.serviceMonitor.bearerTokenFile | string | `"/var/run/secrets/kubernetes.io/serviceaccount/token"` | Bearer token file presented to the metrics endpoint. With `metrics.secure` the endpoint authorises via SubjectAccessReview, so Prometheus must send its own ServiceAccount token. |
| metrics.serviceMonitor.enabled | bool | `false` | Create a Prometheus Operator ServiceMonitor. Requires the `monitoring.coreos.com/v1` CRD to exist; leave false otherwise or the install fails on an unknown kind. |
| metrics.serviceMonitor.honorLabels | bool | `false` | Keep label values from the target when they collide with Prometheus'. |
| metrics.serviceMonitor.interval | string | `"30s"` | Scrape interval. |
| metrics.serviceMonitor.labels | object | `{}` | Extra labels on the ServiceMonitor. Must match your Prometheus `serviceMonitorSelector` (e.g. `{release: kube-prometheus-stack}`), or the ServiceMonitor is created and silently never scraped. |
| metrics.serviceMonitor.metricRelabelings | list | `[]` | `metric_relabel_configs` applied after the scrape. Use to drop the high-cardinality controller-runtime histograms if they cost you. |
| metrics.serviceMonitor.namespace | string | `""` | Namespace to create the ServiceMonitor in. Empty means the release namespace. Set when Prometheus only watches a dedicated namespace. |
| metrics.serviceMonitor.path | string | `"/metrics"` | Path to scrape. |
| metrics.serviceMonitor.relabelings | list | `[]` | `relabel_configs` applied before the scrape. |
| metrics.serviceMonitor.scheme | string | `""` | Scheme. Must be `https` when `metrics.secure` is true, `http` otherwise. Empty means the chart derives it from `metrics.secure`. |
| metrics.serviceMonitor.scrapeTimeout | string | `""` | Scrape timeout. Empty means Prometheus' default. Must not exceed `interval`. |
| metrics.serviceMonitor.tlsConfig | object | `{"insecureSkipVerify":true}` | TLS settings for the scrape. The metrics server uses a self-signed certificate by default, hence `insecureSkipVerify`. Replace with a `ca` reference if you front the endpoint with a cert-manager certificate. |
| nameOverride | string | `""` | Override the chart name portion of generated resource names. Rarely needed. Affects names and the `app.kubernetes.io/name` label. |
| nodeSelector | object | `{}` | Node selector for the operator Pod, e.g. `{kubernetes.io/os: linux}`. Use to keep the operator on a control-plane-adjacent or system node pool. |
| podAnnotations | object | `{}` | Extra annotations for the operator Pod. Common uses: service-mesh opt-out (`linkerd.io/inject: disabled`), Vault agent injection, or forcing a rollout by stamping a checksum. |
| podLabels | object | `{}` | Extra labels for the operator Pod. Useful for cost allocation, NetworkPolicy selectors, or scrape-target selection by an agent other than Prometheus. |
| podSecurityContext | object | `{"runAsNonRoot":true,"seccompProfile":{"type":"RuntimeDefault"}}` | Pod-level security context. These defaults mirror `config/manager` and satisfy the Kubernetes `restricted` Pod Security Standard. Change only if a policy engine in your cluster demands different (e.g. explicit `runAsUser`). |
| priorityClassName | string | `""` | PriorityClass for the operator Pod, e.g. `system-cluster-critical`. Set this if you want the operator to survive node pressure eviction ahead of workload Pods. The PriorityClass must already exist. |
| rbac.create | bool | `true` | Create the ClusterRole/ClusterRoleBinding and the namespaced leader-election Role/RoleBinding. Set to false only when RBAC is managed out-of-band by a cluster admin; the operator will not function without equivalent permissions. |
| readinessProbe | object | `{"failureThreshold":3,"initialDelaySeconds":5,"periodSeconds":10,"successThreshold":1,"timeoutSeconds":1}` | Readiness probe for the manager. Governs when the Pod starts counting as available during a rolling update. |
| replicaCount | int | `1` | Number of operator Pods. Leave at 1. The operator is a singleton by design: leader election (see `leaderElection.enabled`) means only one replica ever reconciles, so extra replicas buy faster failover at the cost of an idle Pod. Raise to 2 only if you need sub-second takeover during node drains. |
| resources | object | `{"limits":{"cpu":"500m","memory":"256Mi"},"requests":{"cpu":"10m","memory":"64Mi"}}` | CPU/memory requests and limits. The operator is event-driven and mostly idle; requests are deliberately small so it schedules anywhere. Raise limits if you manage thousands of resources or see OOMKills / CPU throttling. |
| securityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true}` | Container-level security context. The operator writes nothing to disk, so the root filesystem is read-only and all capabilities are dropped. If you add an `extraVolumeMounts` entry that needs writing, mount an `emptyDir` rather than relaxing `readOnlyRootFilesystem`. |
| serviceAccount.annotations | object | `{}` | Annotations for the ServiceAccount. The usual reason to set these is cloud workload identity (`eks.amazonaws.com/role-arn`, `iam.gke.io/...`). |
| serviceAccount.automountServiceAccountToken | bool | `true` | Mount the ServiceAccount token into the Pod. The operator talks to the API server, so this must stay true unless you inject credentials another way (e.g. a projected token from a sidecar). |
| serviceAccount.create | bool | `true` | Create the ServiceAccount. Set to false if your platform team pre-provisions ServiceAccounts (e.g. with IRSA / Workload Identity annotations managed elsewhere); then set `serviceAccount.name`. |
| serviceAccount.name | string | `""` | Name of the ServiceAccount. Empty means the generated fullname when `create` is true, or `default` when it is false. Required when `create: false` and you are not using the `default` ServiceAccount. |
| terminationGracePeriodSeconds | int | `10` | Grace period before SIGKILL. The manager stops its controllers and releases the leader lease on SIGTERM; 10s is ample. Raise only if you see reconciles being cut off mid-flight. |
| tolerations | list | `[]` | Tolerations for the operator Pod. Needed when the only nodes it may run on carry taints (dedicated system pools, control-plane nodes). |
| topologySpreadConstraints | list | `[]` | Topology spread constraints. An alternative to `affinity` for spreading replicas across zones or nodes. |
| watchNamespaces | list | `[]` | Namespaces the operator watches, as a list. Empty means all namespaces (cluster-wide) and is the default. Restricting this shrinks cache memory and blast radius, but the operator will silently ignore resources elsewhere, and ClusterAuthentikConnection remains cluster-scoped regardless. Example: `watchNamespaces: ["apps", "platform"]` |

<!-- END GENERATED: helm-values -->
