# Getting started

This section takes you from an empty cluster to a working authentik application,
in four steps:

1. [Install the operator](installation.md).
2. [Create an authentik API token](api-token.md) and put it in a `Secret`.
3. Create a connection pointing at your authentik.
4. [Declare a provider and an application](first-application.md).

!!! warning "Early, but working"

    Every kind reconciles and is exercised end-to-end against a real authentik
    in CI. The `v1alpha1` API is still unstable and coverage of less common
    fields is thin. See [Implementation status](#implementation-status) below.

## Prerequisites

| You need | Why | Notes |
| --- | --- | --- |
| A running authentik instance | The operator configures authentik; it does not install it. | Must be one of the [supported versions](../operations/supported-versions.md). |
| Network reach from the cluster to authentik | Every reconcile is an outbound HTTPS call from the operator Pod. | A `NetworkPolicy` that blocks egress will stall every resource. |
| An authentik API token | Authenticates that call. | [How to create one](api-token.md). |
| A Kubernetes cluster, 1.29 or newer | The chart declares `kubeVersion: ">=1.29.0-0"`. | Server-side apply and metrics filtering are relied upon. |
| `kubectl`, and `helm` if you install via the chart | Installing. | Either install path works. |

You do **not** need cluster-admin to *use* the operator — but you do need it to
install the CRDs, and creating a `ClusterAuthentikConnection` is effectively a
cluster-admin privilege in its own right. See [Security](../operations/security.md).

## The mental model

Three object types, in a chain. Each one only knows about the layer below it.

```text
  AuthentikConnection              "which authentik, and with what token"
  ClusterAuthentikConnection
            ▲
            │  spec.connectionRef
            │
      OAuth2Provider               "how a client authenticates"
      SAMLProvider
      ProxyProvider
            ▲
            │  spec.providerRef
            │
       Application                 "what users see in the authentik launcher"
```

Read it from the bottom up:

`Application`

:   What a user sees and clicks in the authentik library. It carries a slug, a
    display name, and a reference to exactly one provider. On its own it
    authenticates nobody.

Provider

:   *How* authentication happens for that application — an OAuth2/OIDC client, a
    SAML service provider, or a proxy provider fronted by an outpost. This is
    where redirect URIs, signing keys and property mappings live.

Connection

:   *Which* authentik instance to talk to, and with which API token. Every other
    resource carries a `spec.connectionRef` selecting one. Nothing reconciles
    without it.

`Outpost` sits off to the side: it is a deployment concern rather than a step in
the chain, and it references a *set* of providers rather than being referenced by
one. See [Outposts](../guides/outposts.md).

### Connections come in two scopes

| Kind | Scope | Token `Secret` resolved from | Use when |
| --- | --- | --- | --- |
| `AuthentikConnection` | Namespaced | Its own namespace, always | Multi-tenant clusters. The safe default. |
| `ClusterAuthentikConnection` | Cluster | The namespace named in its spec | One authentik shared by the whole cluster. |

A namespaced resource may reference either, via `spec.connectionRef.kind`. The
default is `AuthentikConnection`.

!!! danger "The cluster-scoped kind is a privilege boundary"

    A `ClusterAuthentikConnection` names the namespace its token `Secret` lives
    in, so whoever can create one can make the operator read a `Secret` from any
    namespace in the cluster. Read [Connections](../guides/connections.md) before
    you grant anyone access to it.

### What the operator does on each reconcile

1. Resolve `spec.connectionRef` to a connection object, and check the referring
   namespace is allowed to use it.
2. Read the API token from the connection's `tokenSecretRef`.
3. Check the authentik version is in the supported range. If not, stop with
   `UnsupportedVersion` rather than failing obscurely mid-call.
4. Look up the managed object in authentik, preferring `status.remoteID` over
   the name so a rename on either side does not create a duplicate.
5. Create, update, or refuse — refusing when a same-named object exists that the
   operator did not create and `adoptionPolicy` is `FailOnConflict`.
6. Write `status`: conditions, `remoteID`, `observedGeneration`.

Steps 5 and 6 are where most surprises live. See
[Conditions](../reference/conditions.md) and
[Troubleshooting](../operations/troubleshooting.md).

## Implementation status

| Kind | API types | Controller |
| --- | --- | --- |
| `AuthentikConnection` | Defined in `api/v1alpha1` | Planned |
| `ClusterAuthentikConnection` | Defined in `api/v1alpha1` | Planned |
| `OAuth2Provider`, `SAMLProvider`, `ProxyProvider` | Planned | Planned |
| `Application` | Planned | Planned |
| `Outpost`, `KubernetesServiceConnection`, `DockerServiceConnection` | Planned | Planned |

Every kind reconciles, and the operator is exercised end-to-end against a real
authentik in CI on every push.

Design decisions already settled are recorded as ADRs:

- [ADR 0001 — Connection scope](../decisions/0001-connection-scope.md)
- [ADR 0002 — Adoption policy](../decisions/0002-adoption-policy.md)
