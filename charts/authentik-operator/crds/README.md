# CRDs

This directory holds the CustomResourceDefinitions for the `authentik.k8s.rka.sh`
API group:

- `AuthentikConnection` (namespaced)
- `ClusterAuthentikConnection` (cluster-scoped)

## Read this before you upgrade the chart

> **Helm does NOT upgrade or delete CRDs in `crds/`.**
>
> Helm applies everything in `crds/` exactly once, on **first install**, before
> any template is rendered. On `helm upgrade` it does nothing with them. On
> `helm uninstall` it leaves them in place.

The failure mode is silent. A chart upgrade that adds a field, a new printer
column, or a new CRD version will appear to succeed: the Deployment rolls, the
operator starts, and nothing logs an error. But the API server is still
validating against the **old** schema, so new fields get pruned from objects you
apply and the operator sees requests it cannot act on. Debugging that from the
symptom end is miserable, which is why it gets a banner here.

### Apply CRD changes yourself, every upgrade

From a packaged chart:

```sh
helm show crds authentik-operator/authentik-operator --version <new-version> \
  | kubectl apply --server-side -f -
```

From a checkout:

```sh
kubectl apply --server-side -f charts/authentik-operator/crds/
```

Use `--server-side` (and `--force-conflicts` if a previous client-side apply
left a large `last-applied-configuration` annotation). Client-side apply can
fail on these CRDs with "metadata.annotations: Too long".

Do the CRD apply **before** `helm upgrade`, so the new schema is in place when
the new operator starts.

### Uninstalling

`helm uninstall` leaves the CRDs behind, deliberately: deleting a CRD deletes
every object of that kind, cluster-wide, with no undo. Remove them explicitly
only when you are certain:

```sh
kubectl delete crd authentikconnections.authentik.k8s.rka.sh
kubectl delete crd clusterauthentikconnections.authentik.k8s.rka.sh
```

### Prefer templated CRDs?

Some teams would rather have CRDs upgrade with the release. Move the CRD YAML
into `templates/` and wrap it in a `{{- if .Values.crds.install }}` guard. The
trade-off: an accidental `helm uninstall` then deletes the CRDs and every
custom resource with them. This chart chose the safer default.

## Where these files come from

They are generated from the Go types by controller-gen and copied here — the
directory is not hand-maintained:

```sh
make manifests
cp config/crd/bases/*.yaml charts/authentik-operator/crds/
```

**This directory may be empty in a working tree where `make manifests` has not
run yet.** That is expected and is not an error: Helm treats an empty (or
README-only) `crds/` directory as "no CRDs to install", so `helm lint`,
`helm template` and `helm install` all succeed. Non-YAML files such as this
README are ignored by Helm.
