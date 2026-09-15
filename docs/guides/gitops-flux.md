# GitOps with Flux

The operator's Helm chart is published to GHCR as an OCI artifact, so Flux can
consume it directly — no chart repository to host, one registry and one
credential for both the chart and the image.

Runnable manifests for everything below live in
[`examples/flux`](https://github.com/RashKash103/authentik-operator/tree/main/examples/flux).

## Two reconciliations, not one

Install the operator and declare authentik resources as **separate** Flux
resources. They fail differently: the operator is infrastructure that changes on
release cadence, while the resources change whenever someone onboards an
application. Reconciled as one unit, a bad application manifest blocks an
operator upgrade — and an operator upgrade drags every application with it.

## Installing the operator

```yaml title="operator/ocirepository.yaml"
apiVersion: source.toolkit.fluxcd.io/v1
kind: OCIRepository
metadata:
  name: authentik-operator
  namespace: flux-system
spec:
  interval: 30m
  url: oci://ghcr.io/rashkash103/charts/authentik-operator
  ref:
    tag: "0.2.0"
  layerSelector:
    mediaType: application/vnd.cncf.helm.chart.content.v1.tar+gzip
    operation: copy
```

```yaml title="operator/helmrelease.yaml"
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: authentik-operator
  namespace: flux-system
spec:
  interval: 30m
  targetNamespace: authentik-operator-system
  install:
    createNamespace: true
  upgrade:
    crds: CreateReplace
  chartRef:
    kind: OCIRepository
    name: authentik-operator
    namespace: flux-system
  values:
    metrics:
      enabled: true
      secure: true
```

!!! warning "`crds: CreateReplace` is doing real work here"

    Helm installs a chart's `crds/` directory but never upgrades it, so a plain
    `helm upgrade` silently leaves you on stale CRDs. `CreateReplace` lets Flux
    apply CRD changes on upgrade, which is what you want for an operator whose
    API is still `v1alpha1`.

!!! note "Pin the tag"

    `ref.tag` pins an exact chart version rather than a semver range. One
    operator release targets one authentik version, so an automatic minor bump
    can move you onto a chart built against an authentik you are not running.
    See [Supported versions](../operations/supported-versions.md).

    If you do want automatic updates inside a series, use
    `ref: {semver: ">=0.2.0 <0.3.0"}`.

## Declaring authentik resources

```yaml title="resources/flux-kustomization.yaml"
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: authentik-resources
  namespace: flux-system
spec:
  interval: 10m
  path: ./examples/flux/resources
  prune: true
  sourceRef:
    kind: GitRepository
    name: flux-system
  dependsOn:
    - name: authentik-operator
  decryption:
    provider: sops
    secretRef:
      name: sops-age
  healthChecks:
    - apiVersion: authentik.k8s.rka.sh/v1alpha1
      kind: OAuth2Provider
      name: grafana
      namespace: identity
```

`dependsOn` is the part worth keeping: without it Flux applies the custom
resources before the operator's CRDs exist, and the Kustomization fails until
the next retry. Naming the dependency turns a transient error into an ordered
rollout.

`healthChecks` works because every resource reports a standard `Ready`
condition with `observedGeneration`, so Flux can tell "reconciled successfully"
from "not looked at yet".

## Ordering between resources

You do **not** need `dependsOn` between a provider and the application that
references it. An `Application` applied first reports `ReferenceNotFound` and
converges as soon as the provider has a primary key, because the operator
watches the provider kinds. Put them in the same Kustomization and let it sort
itself out.

## The API token

An authentik API token must not be committed in plain text. The example
references a Secret it does not create; supply it with SOPS, Sealed Secrets or
External Secrets — whatever the cluster already uses.

```yaml title="resources/secret.sops.example.yaml"
apiVersion: v1
kind: Secret
metadata:
  name: authentik-api-token
  namespace: identity
type: Opaque
stringData:
  token: REPLACE_ME_THEN_ENCRYPT
```

Create the token in authentik under **Directory → Tokens** with the `api`
intent, scoped as narrowly as the resources you manage allow. See
[Security](../operations/security.md).

## Private registries

The chart and image are public. For a private mirror, create a pull secret and
reference it from the `OCIRepository`:

```sh
kubectl create secret docker-registry ghcr-creds \
  --namespace flux-system \
  --docker-server=ghcr.io \
  --docker-username="<user>" \
  --docker-password="<token>"
```

```yaml
spec:
  secretRef:
    name: ghcr-creds
```

and add `imagePullSecrets` to the HelmRelease values so the operator's own image
can be pulled too.
