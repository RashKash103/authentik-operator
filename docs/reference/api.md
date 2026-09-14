# API reference

Complete field-level reference for every kind in the `authentik.k8s.rka.sh/v1alpha1`
API group.

!!! danger "Do not edit this page by hand"

    Everything between the generated markers below is produced from the Go types
    in `api/v1alpha1/*.go`. Hand edits are overwritten by the next run and fail
    `hack/gen-docs.py --check` in between.

    To change what appears here, **edit the doc comments on the Go types** and
    regenerate. A field's documentation is the comment above it; its validation
    is its `+kubebuilder` markers.

## Regenerating

```sh
make docs-api
```

!!! warning "`make docs-api` does not exist yet"

    This target is planned and is **not** in the Makefile today, so the
    generated block below is empty. It is a stub rather than a stale copy on
    purpose: a hand-written API reference is wrong within a week of the types
    changing, and a wrong reference is worse than a missing one.

    Until the target lands, read the types directly. They are short, thoroughly
    commented, and are the actual source of truth:

    - `api/v1alpha1/authentikconnection_types.go`
    - `api/v1alpha1/clusterauthentikconnection_types.go`
    - `api/v1alpha1/common_types.go`

    Or ask the cluster, which serves the generated OpenAPI schema:

    ```sh
    kubectl explain authentikconnection.spec --recursive
    kubectl explain clusterauthentikconnection.spec.tokenSecretRef
    ```

## What exists today

| Kind | Scope | Short name | API types |
| --- | --- | --- | --- |
| `AuthentikConnection` | Namespaced | `akconn` | Defined |
| `ClusterAuthentikConnection` | Cluster | `clakconn` | Defined |
| `OAuth2Provider` | Namespaced | — | Planned |
| `SAMLProvider` | Namespaced | — | Planned |
| `ProxyProvider` | Namespaced | — | Planned |
| `Application` | Namespaced | — | Planned |
| `Outpost` | Namespaced | — | Planned |
| `KubernetesServiceConnection` | Namespaced | — | Planned |
| `DockerServiceConnection` | Namespaced | — | Planned |

Shared types used across kinds — `ConnectionReference`, `LocalSecretKeyReference`,
`SecretKeyReference`, `AdoptionPolicy`, `DeletionPolicy` and
`ManagedResourceStatus` — live in `api/v1alpha1/common_types.go`. The prose
guides describe them: [Connections](../guides/connections.md),
[Providers](../guides/providers.md), [Applications](../guides/applications.md),
[Outposts](../guides/outposts.md).

## Generated reference

<!-- BEGIN GENERATED: api -->
<!-- END GENERATED: api -->
