# authentik-operator

Helm chart for the **authentik-operator** — a Kubernetes operator that manages
[authentik](https://goauthentik.io/) resources declaratively from custom
resources in your cluster.

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational)
![AppVersion: 0.1.0](https://img.shields.io/badge/AppVersion-0.1.0-informational)
![Type: application](https://img.shields.io/badge/Type-application-informational)

## What this chart installs — and what it does not

The chart name invites the wrong assumption, so to be explicit:

**This chart installs the operator only.**

| Installed by this chart | **NOT** installed by this chart |
| --- | --- |
| The operator Deployment (`/manager`) | authentik server or worker |
| CRDs for `authentik.k8s.rka.sh` (first install only, see below) | PostgreSQL |
| ServiceAccount, ClusterRole + binding | Redis / valkey |
| Leader-election Role + binding | Ingress, TLS certificates, DNS |
| Metrics Service, optional ServiceMonitor | An authentik API token |

You bring your own authentik — self-hosted, or run via
[the official authentik Helm chart](https://github.com/goauthentik/helm). After
installing this chart you tell the operator where that instance lives by
creating an `AuthentikConnection` (namespaced) or `ClusterAuthentikConnection`
(cluster-scoped). See [Getting started](#getting-started).

## Requirements

- Kubernetes `>=1.29.0-0`
- Helm 3.8+
- A reachable authentik instance and an API token for it
- Optional: the Prometheus Operator CRDs, if you set
  `metrics.serviceMonitor.enabled=true`

## Install

The chart is published to GHCR as an OCI artifact, so there is no repository to
add:

```sh
helm install authentik-operator oci://ghcr.io/rashkash103/charts/authentik-operator \
  --version 0.1.0 \
  --namespace authentik-operator-system --create-namespace
```

**Always pass `--version`.** One operator release targets one authentik version,
so the chart version you install has to match the authentik you run. See the
version policy in the project README.

For GitOps, `examples/flux` has a ready `OCIRepository` + `HelmRelease` pair.

From a checkout:

```sh
helm install authentik-operator ./charts/authentik-operator \
  --namespace authentik-system --create-namespace
```

Verify:

```sh
kubectl -n authentik-system rollout status deploy/authentik-operator
kubectl -n authentik-system logs deploy/authentik-operator -f
```

## Getting started

1. Create a Secret holding an authentik API token (authentik UI:
   *Directory → Tokens → Create*, intent **API Token**):

   ```sh
   kubectl -n authentik-system create secret generic authentik-api \
     --from-literal=token='<your-authentik-api-token>'
   ```

2. Create a connection:

   ```yaml
   apiVersion: authentik.k8s.rka.sh/v1alpha1
   kind: AuthentikConnection
   metadata:
     name: authentik
     namespace: authentik-system
   spec:
     url: https://authentik.example.com
     tokenSecretRef:
       name: authentik-api
       key: token
   ```

3. Check it reconciled:

   ```sh
   kubectl -n authentik-system get authentikconnection authentik -o wide
   ```

## Upgrade

> [!IMPORTANT]
> **CRDs are not upgraded by `helm upgrade`.** Helm applies the files in a
> chart's `crds/` directory on **first install only**. It never updates or
> deletes them afterwards. A chart upgrade that changes a CRD schema will
> appear to succeed while the API server keeps validating against the old
> schema — new fields are silently pruned from objects you apply, and the
> operator never sees them. Nothing logs an error.

Apply the CRDs yourself **before** every `helm upgrade`:

```sh
# 1. CRDs first
helm show crds oci://ghcr.io/rashkash103/charts/authentik-operator --version <new-version> \
  | kubectl apply --server-side -f -

# 2. then the release
helm upgrade authentik-operator oci://ghcr.io/rashkash103/charts/authentik-operator \
  --namespace authentik-system --version <new-version>
```

Use `--server-side`; a client-side apply can fail on these CRDs with
`metadata.annotations: Too long`. Full detail in
[`crds/README.md`](./crds/README.md).

## Uninstall

```sh
helm uninstall authentik-operator --namespace authentik-system
```

This leaves the CRDs — and therefore every `AuthentikConnection` — in place,
deliberately: deleting a CRD deletes every object of that kind cluster-wide,
with no undo. Remove them only when you are certain:

```sh
kubectl delete crd authentikconnections.authentik.k8s.rka.sh
kubectl delete crd clusterauthentikconnections.authentik.k8s.rka.sh
```

## Values validation

The chart ships a `values.schema.json`, so Helm rejects bad input at
`install`/`upgrade`/`template` time rather than producing a broken Deployment.
Structured objects use `additionalProperties: false`, which catches the failure
mode a schema is really for — a misspelled key that would otherwise be silently
ignored:

```console
$ helm template authentik-operator ./charts/authentik-operator --set-json 'resource={"limits":{"cpu":"500m"}}'
Error: values don't meet the specifications of the schema(s) in the following chart(s):
authentik-operator:
- (root): Additional property resource is not allowed
```

## Security posture

Defaults match the Kubernetes `restricted` Pod Security Standard and the
upstream `config/manager` manifests:

- `runAsNonRoot: true`, `seccompProfile: RuntimeDefault`
- `readOnlyRootFilesystem: true`, `allowPrivilegeEscalation: false`
- all Linux capabilities dropped
- metrics served over HTTPS with `TokenReview`/`SubjectAccessReview`
  authorisation (`metrics.secure=true`)
- HTTP/2 disabled on the metrics endpoint (`enableHTTP2=false`)

## Metrics

With `metrics.secure=true` (the default) a scraper must present a token
authorised for `GET /metrics`. The chart creates a `…-metrics-reader`
ClusterRole for exactly that; bind it to your Prometheus ServiceAccount:

```sh
kubectl create clusterrolebinding authentik-operator-metrics-reader \
  --clusterrole=authentik-operator-metrics-reader \
  --serviceaccount=monitoring:prometheus-k8s
```

Then set `metrics.serviceMonitor.enabled=true` and give it labels your
Prometheus `serviceMonitorSelector` matches, e.g.
`metrics.serviceMonitor.labels.release=kube-prometheus-stack`.

## Values

### Workload

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `replicaCount` | int | `1` | Number of operator Pods. Leader election means only one replica reconciles; raise to 2 only for faster failover. |
| `image.repository` | string | `ghcr.io/rashkash103/authentik-operator` | Operator image repository. Change when mirroring into a private registry. |
| `image.tag` | string | `""` | Image tag. Empty means the chart's `appVersion`. |
| `image.digest` | string | `""` | Image digest (`sha256:…`). Wins over `tag`; use for immutable deploys. |
| `image.pullPolicy` | string | `IfNotPresent` | One of `Always`, `IfNotPresent`, `Never`. |
| `imagePullSecrets` | list | `[]` | Pre-existing pull Secrets for a private registry. Must already exist in the namespace. |
| `nameOverride` | string | `""` | Override the chart-name portion of generated names. |
| `fullnameOverride` | string | `""` | Replace the generated `<release>-<chart>` name entirely. |
| `priorityClassName` | string | `""` | PriorityClass, e.g. `system-cluster-critical`. Must already exist. |
| `terminationGracePeriodSeconds` | int | `10` | Grace period before SIGKILL; the manager releases its lease on SIGTERM. |
| `resources.limits.cpu` | string | `500m` | CPU limit. |
| `resources.limits.memory` | string | `256Mi` | Memory limit. Raise if you see OOMKills at scale. |
| `resources.requests.cpu` | string | `10m` | CPU request; kept small so the operator schedules anywhere. |
| `resources.requests.memory` | string | `64Mi` | Memory request. |

### Scheduling

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `nodeSelector` | object | `{}` | Node selector, e.g. to pin to a system node pool. |
| `tolerations` | list | `[]` | Tolerations, needed when eligible nodes are tainted. |
| `affinity` | object | `{}` | Affinity rules. With `replicaCount: 2` add pod anti-affinity. |
| `topologySpreadConstraints` | list | `[]` | Spread replicas across zones or nodes. |

### Pod metadata, networking and extras

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `podAnnotations` | object | `{}` | Extra Pod annotations (mesh opt-out, secret injectors). |
| `podLabels` | object | `{}` | Extra Pod labels (cost allocation, NetworkPolicy selectors). |
| `dnsPolicy` | string | `""` | Pod DNS policy; `None` requires `dnsConfig`. |
| `dnsConfig` | object | `{}` | Custom DNS settings, used with `dnsPolicy: None`. |
| `hostAliases` | list | `[]` | Extra `/etc/hosts` entries, when authentik's URL resolves via unreachable DNS. |
| `extraEnv` | list | `[]` | Extra env vars in full Kubernetes form (`valueFrom` supported): proxies, `SSL_CERT_FILE`. |
| `extraArgs` | list | `[]` | Extra `/manager` flags; escape hatch for flags newer than this chart. |
| `extraVolumes` | list | `[]` | Extra Pod volumes, typically a CA bundle or an `emptyDir`. |
| `extraVolumeMounts` | list | `[]` | Mounts matching `extraVolumes`. |
| `extraObjects` | list | `[]` | Extra manifests rendered with the release; strings pass through `tpl`. |

### Security

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `podSecurityContext.runAsNonRoot` | bool | `true` | Refuse to start as UID 0. |
| `podSecurityContext.seccompProfile.type` | string | `RuntimeDefault` | Required by the `restricted` PSS. |
| `securityContext.allowPrivilegeEscalation` | bool | `false` | Keep false. |
| `securityContext.readOnlyRootFilesystem` | bool | `true` | The operator writes nothing to disk; mount an `emptyDir` rather than relaxing this. |
| `securityContext.capabilities.drop` | list | `["ALL"]` | Drop all Linux capabilities. |

### Identity and RBAC

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `serviceAccount.create` | bool | `true` | Create the ServiceAccount. Disable when pre-provisioned elsewhere. |
| `serviceAccount.name` | string | `""` | ServiceAccount name; empty means the generated fullname. |
| `serviceAccount.annotations` | object | `{}` | Annotations, typically cloud workload identity. |
| `serviceAccount.automountServiceAccountToken` | bool | `true` | Must stay true; the operator talks to the API server. |
| `rbac.create` | bool | `true` | Create ClusterRole/Binding and leader-election Role/Binding. |

### Operator behaviour

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `leaderElection.enabled` | bool | `true` | `--leader-elect`. Keep true so overlapping replicas during a rollout do not fight. |
| `watchNamespaces` | list | `[]` | Namespaces to watch; empty means all. Restricting shrinks cache and blast radius, but resources elsewhere are silently ignored. |
| `log.level` | string | `info` | `--zap-log-level`: `debug`, `info` or `error`. |
| `log.format` | string | `json` | `--zap-encoder`: `json` or `console`. |
| `enableHTTP2` | bool | `false` | `--enable-http2`. Off by default given HTTP/2's DoS-shaped CVE history. |

### Health probes

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `health.port` | int | `8081` | Container port for `/healthz` and `/readyz`. |
| `livenessProbe.initialDelaySeconds` | int | `15` | Delay before the first liveness probe. |
| `livenessProbe.periodSeconds` | int | `20` | Liveness probe interval. |
| `livenessProbe.timeoutSeconds` | int | `1` | Liveness probe timeout. |
| `livenessProbe.successThreshold` | int | `1` | Successes needed after a failure. |
| `livenessProbe.failureThreshold` | int | `3` | Failures before the container is restarted. |
| `readinessProbe.initialDelaySeconds` | int | `5` | Delay before the first readiness probe. |
| `readinessProbe.periodSeconds` | int | `10` | Readiness probe interval. |
| `readinessProbe.timeoutSeconds` | int | `1` | Readiness probe timeout. |
| `readinessProbe.successThreshold` | int | `1` | Successes needed after a failure. |
| `readinessProbe.failureThreshold` | int | `3` | Failures before the Pod is marked unready. |

### Metrics

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `metrics.enabled` | bool | `true` | Serve controller-runtime metrics; false sets the bind address to `0`. |
| `metrics.port` | int | `8443` | Container port for metrics. |
| `metrics.secure` | bool | `true` | HTTPS with authn/authz. False serves plaintext to anything that can reach the Pod. |
| `metrics.service.enabled` | bool | `true` | Create the metrics Service. |
| `metrics.service.type` | string | `ClusterIP` | `ClusterIP`, `NodePort` or `LoadBalancer`. Keep `ClusterIP`. |
| `metrics.service.port` | int | `8443` | Service port. |
| `metrics.service.annotations` | object | `{}` | Extra Service annotations. |
| `metrics.service.labels` | object | `{}` | Extra Service labels, for `serviceMonitorSelector` matching. |
| `metrics.serviceMonitor.enabled` | bool | `false` | Create a ServiceMonitor. Requires the Prometheus Operator CRDs. |
| `metrics.serviceMonitor.namespace` | string | `""` | ServiceMonitor namespace; empty means the release namespace. |
| `metrics.serviceMonitor.labels` | object | `{}` | Labels that must match your `serviceMonitorSelector`, or it is never scraped. |
| `metrics.serviceMonitor.annotations` | object | `{}` | ServiceMonitor annotations. |
| `metrics.serviceMonitor.interval` | string | `30s` | Scrape interval. |
| `metrics.serviceMonitor.scrapeTimeout` | string | `""` | Scrape timeout; empty means Prometheus' default. |
| `metrics.serviceMonitor.path` | string | `/metrics` | Scrape path. |
| `metrics.serviceMonitor.scheme` | string | `""` | `http`/`https`; empty derives it from `metrics.secure`. |
| `metrics.serviceMonitor.bearerTokenFile` | string | `/var/run/secrets/kubernetes.io/serviceaccount/token` | Token Prometheus presents when `metrics.secure` is on. |
| `metrics.serviceMonitor.tlsConfig` | object | `{insecureSkipVerify: true}` | TLS settings; the metrics server uses a self-signed certificate. |
| `metrics.serviceMonitor.honorLabels` | bool | `false` | Keep target labels on collision. |
| `metrics.serviceMonitor.relabelings` | list | `[]` | `relabel_configs` applied before the scrape. |
| `metrics.serviceMonitor.metricRelabelings` | list | `[]` | `metric_relabel_configs`; use to drop high-cardinality histograms. |

Every value is also documented inline in [`values.yaml`](./values.yaml), which
is the authoritative reference.

## Development

Test value sets live in [`ci/`](./ci) and are what CI renders:

```sh
helm lint --strict charts/authentik-operator
helm lint --strict charts/authentik-operator -f charts/authentik-operator/ci/full-values.yaml
helm template t charts/authentik-operator -f charts/authentik-operator/ci/full-values.yaml

# must FAIL — these files exist to prove values.schema.json bites
helm template t charts/authentik-operator -f charts/authentik-operator/ci/invalid-values.yaml.txt
helm template t charts/authentik-operator -f charts/authentik-operator/ci/invalid-typo-only.yaml.txt
```

The `crds/` directory is generated, not hand-written:

```sh
make manifests
cp config/crd/bases/*.yaml charts/authentik-operator/crds/
```

An empty `crds/` directory is valid — `helm lint`, `helm template` and
`helm package` all succeed without CRD files present.

## License

Apache-2.0
