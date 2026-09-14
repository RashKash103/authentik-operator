# Deploying with Flux

Two pieces, kept separate on purpose:

1. **`operator/`** installs the operator from the chart published as an OCI
   artifact in GHCR.
2. **`resources/`** declares the authentik objects the operator then manages.

Splitting them matters because they fail differently. The operator is
infrastructure that changes on release cadence; the resources change whenever
someone onboards an application. Reconciling them as one unit means a bad
application manifest blocks an operator upgrade, and vice versa.

```sh
kubectl apply -k examples/flux/operator
kubectl apply -k examples/flux/resources
```

In a real setup these live in a Git repository Flux already watches, rather than
being applied by hand.

## Pinning

`OCIRepository` pins an exact chart tag rather than a semver range. The operator
is pinned to one authentik version at a time, so an automatic minor bump can
move you onto a chart built against an authentik release you are not running.
See the version policy in the README.

If you do want automatic updates within a series, swap `ref.tag` for:

```yaml
  ref:
    semver: ">=0.1.0 <0.2.0"
```

## Registry credentials

The chart and image are public, so no pull secret is needed. For a private
mirror, create one and reference it:

```sh
kubectl create secret docker-registry ghcr-creds \
  --namespace flux-system \
  --docker-server=ghcr.io \
  --docker-username="<user>" \
  --docker-password="<token>"
```

then add `secretRef: {name: ghcr-creds}` to the `OCIRepository` spec and
`imagePullSecrets` to the HelmRelease values.

## The API token

The operator needs an authentik API token, and it must not be committed in
plain text. The example references a Secret named `authentik-api-token` without
creating it. Supply it with SOPS, Sealed Secrets, or External Secrets — whatever
the cluster already uses. `resources/secret.sops.example.yaml` shows the shape.
