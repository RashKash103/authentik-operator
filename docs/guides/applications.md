# Applications

An `Application` is what a user sees and clicks in the authentik library. It
carries a slug, a display name, some presentation metadata, and a reference to
exactly one [provider](providers.md). On its own it authenticates nobody.

## Shape

```yaml
apiVersion: authentik.k8s.rka.sh/v1alpha1
kind: Application
metadata:
  name: grafana
  namespace: my-apps
spec:
  connectionRef:
    kind: AuthentikConnection      # default
    name: default

  adoptionPolicy: FailOnConflict   # default
  deletionPolicy: Delete           # default

  slug: grafana
  name: Grafana
  group: Observability

  providerRef:
    kind: OAuth2Provider
    name: grafana

  metaDescription: Dashboards and alerting
  metaLaunchUrl: https://grafana.example.com
  metaPublisher: Platform team
  metaIcon: https://grafana.example.com/public/img/grafana_icon.svg

  policyEngineMode: any            # any | all
  openInNewTab: false
```

## The slug is the identity

`spec.slug` is authentik's identifier for the application. It appears in
authentik's own URLs, and it is what a name collision collides on.

It is **not** derived from `metadata.name`. Deriving it would make a Kubernetes
rename silently rename the authentik object — and Kubernetes names are
namespace-scoped while authentik slugs are global, so two namespaces each with
an `Application` named `grafana` would fight over one authentik object without
either manifest saying anything about the other.

!!! warning "Two namespaces, one slug, one loser"

    Nothing in Kubernetes prevents `my-apps/grafana` and `staging/grafana` both
    declaring `slug: grafana`. They resolve to the same authentik application.

    With the default `adoptionPolicy: FailOnConflict`, the second one to
    reconcile reports `AdoptionConflict` and stops — which is the point of the
    default. With `AdoptExisting` on both, they overwrite each other on every
    reconcile, forever.

    Namespace your slugs (`staging-grafana`) or use separate authentik
    instances.

Changing `slug` on a live resource renames the object in authentik. Bookmarks
and any hard-coded URL referencing the old slug break.

## The provider is an int32 primary key

This is the interesting part of the design.

In authentik's API, an application's `provider` field is a **numeric primary
key** — an `int32` that only exists once the provider row has actually been
created. There is no way to express "the provider that will be called grafana"
in that field.

So an `Application` applied before its provider cannot be fully created. The
design choice is what to do about it.

!!! success "It waits and converges; it does not fail"

    1. The `Application` is admitted and reconciled.
    2. `spec.providerRef` does not resolve to a provider with a published
       `status.remoteID` yet.
    3. The operator sets `Ready=False`, reason `ReferenceNotFound`, writes
       nothing to authentik, and requeues with backoff.
    4. When the provider becomes ready and publishes its numeric
       `status.remoteID`, the application reconciles again and converges.

The alternative — a hard error — would make the operator unusable with GitOps,
where a directory is applied in whatever order the tool chose. `kubectl apply -f .`
converges. You do not have to sequence the files, and Argo CD or Flux do not
need sync waves for this.

```sh
$ kubectl -n my-apps apply -f .
$ kubectl -n my-apps get application grafana -w
NAME      SLUG      PROVIDER   READY   AGE
grafana   grafana              False   2s
grafana   grafana   OAuth2     True    9s
```

!!! tip "Distinguishing a slow start from a stuck one"

    `ReferenceNotFound` for a few seconds after a bulk apply is normal. For
    minutes, it means the provider is not becoming ready — so investigate the
    provider, not the application:

    ```sh
    kubectl -n my-apps describe oauth2provider grafana
    ```

    Chasing the application's condition here wastes time: it is faithfully
    reporting a problem one layer down.

### Why not reference the numeric ID directly?

You could write `provider: 42` and skip resolution entirely. That would be:

- **not portable** — primary keys differ between your staging and production
  authentik, so the same manifest cannot serve both;
- **unreviewable** — `42` in a pull request tells a reviewer nothing;
- **unstable** — recreating a provider allocates a new key, silently pointing
  the application at whatever now holds the old one.

`providerRef` names a Kubernetes resource. The numeric key stays an
implementation detail in `status.remoteID`.

### `providerRef.kind`

Required — unlike `connectionRef.kind`, there is no sensible default, since the
three provider kinds are equally valid choices:

```yaml
providerRef:
  kind: OAuth2Provider     # or SAMLProvider, ProxyProvider
  name: grafana
```

The reference resolves in the `Application`'s own namespace unless it names a
`namespace`, which the operator refuses unless it was started with
[cross-namespace references](references.md#namespaces) enabled. It is gated
because attaching to another namespace's provider makes that namespace's
configuration yours to break.

!!! note "One provider, one application"

    authentik models this as one application referencing one provider. Two
    `Application` resources pointing at the same provider is not a supported
    configuration, and the second will typically find authentik has already
    bound the provider elsewhere.

## Presentation metadata

Everything under `spec.meta` is cosmetic — it affects the library page and
nothing else.

| Field | Effect |
| --- | --- |
| `meta.description` | Subtitle on the application card. |
| `meta.launchURL` | Where clicking the card sends the user. Empty means authentik derives one where it can. |
| `meta.publisher` | Shown on the card. Useful for attributing ownership in a large library. |
| `meta.icon` | Icon URL. Must be reachable by the user's browser, not by the operator. |
| `spec.group` | Groups cards under a heading in the library. Free text; matching strings group together. |
| `spec.openInNewTab` | Whether the launch URL opens in a new tab. |

!!! tip "`group` is just a string"

    There is no `Group` object. Two applications are in the same group when
    their `group` strings match exactly — so `Observability` and `observability`
    produce two headings. Keep them in sync via your templating, not by hand.

## Policy engine mode

`spec.policyEngineMode` decides how multiple policy bindings combine when
authentik evaluates whether a user may see and use the application:

`any`

:   The user passes if **any** bound policy passes. The default, and the usual
    choice for "members of one of these groups".

`all`

:   The user passes only if **every** bound policy passes.

!!! warning "This operator does not manage policy bindings"

    Policies, policy bindings, flows and stages are out of scope for
    `v1alpha1`. `policyEngineMode` sets how existing bindings combine; it
    creates none.

    An application with no bindings at all is visible to **every** authenticated
    user regardless of this field. If you expect an application to be
    restricted, bind a policy to it in authentik — the operator will not do it
    and will not warn you.

## Status

```sh
$ kubectl -n my-apps get application -o wide
NAME      SLUG      PROVIDER   READY   AGE
grafana   grafana   OAuth2     True    4h
```

| Field | Meaning |
| --- | --- |
| `status.conditions` | `Ready` and `Synced`. See [Conditions](../reference/conditions.md). |
| `status.observedGeneration` | The `metadata.generation` this status reflects. |
| `status.remoteID` | authentik's numeric primary key for the application. |
| `status.remoteName` | The slug last observed in authentik. |
| `status.adopted` | Whether this took over a pre-existing application. |
| `status.lastSyncedTime` | Last successful reconcile. |

## Deletion

`deletionPolicy: Delete` (the default) removes the authentik application when
the Kubernetes resource is deleted. `Orphan` leaves it in place.

!!! note "Deleting an application does not delete its provider"

    They are separate resources with separate lifecycles. Deleting only the
    `Application` leaves the provider — and its generated client credentials —
    configured in authentik, just no longer reachable from the library.

    Use `Orphan` when handing an application back to manual management, or when
    tearing down a cluster whose authentik outlives it.

## Adoption

`adoptionPolicy` defaults to `FailOnConflict`. An existing authentik application
with the same slug that the operator cannot prove it created causes
`AdoptionConflict`: the reconcile fails, **nothing in authentik is modified**,
and the resource requeues.

Because slugs are global and short — `grafana`, `gitlab`, `proxy` — collisions
here are routine rather than exceptional.
[ADR 0002](../decisions/0002-adoption-policy.md) records why silent adoption was
rejected.

## See also

- [Providers](providers.md) — what `providerRef` points at.
- [Your first application](../getting-started/first-application.md) — the worked example.
- [ADR 0002 — Adoption policy](../decisions/0002-adoption-policy.md)
