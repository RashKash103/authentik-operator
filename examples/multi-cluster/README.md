# Several operators, one authentik

More than one operator can point at the same authentik: one per Kubernetes
cluster, or one reaching the instance directly while another goes through a
proxy.

The problem this creates is ownership. authentik has **no ownership marker** on
providers or applications — objects are keyed by name. So two operators that
both manage a provider called `grafana` are managing *the same object*, and the
second one to arrive either refuses it as an adoption conflict or takes it over
from the first.

Set `spec.cluster` on the connection to keep them apart:

```yaml
spec:
  url: https://authentik.example.com
  cluster: prod-eu
```

The operator then scopes the names of objects it manages with that identity. A
provider declared as `grafana` is created in authentik as `grafana-prod-eu`, and
lookups use the same scoped name — so an operator never finds, adopts or deletes
an object belonging to a different cluster.

## What it looks like in authentik

| Object | `spec.cluster` | Declared | In authentik |
| --- | --- | --- | --- |
| Provider | `prod-eu` | `grafana` | `grafana-prod-eu` |
| Provider | `prod-us` | `grafana` | `grafana-prod-us` |
| Provider | *(unset)* | `grafana` | `grafana` |
| **Application** | *any* | `grafana` | `grafana` — never scoped |

## Applications are deliberately not scoped

An application's **slug appears in the URL** users are sent to when logging in,
and its **name appears in every user's application list**. Scoping either would
publish your cluster naming to anyone who reaches that page, including people
who are not signed in.

So `cluster` scopes provider, outpost and service connection names — visible
only in authentik's admin interface — and leaves applications alone.

Two clusters cannot both own the slug `grafana` regardless: that is one URL on
one authentik. The operator refuses to take over an application it did not
create and reports the collision; resolving it means choosing distinct slugs,
which is a decision about what your users see.

Leaving it unset is the right choice when only one operator talks to the
instance, and it keeps names exactly as declared.

## Changing it later

`cluster` is part of an object's identity in authentik, so changing it **orphans
everything created under the old value**. The operator does not rename those
objects; it creates new ones under the new scope and leaves the old ones behind
for you to remove.

Decide it when you first point an operator at a shared authentik, not after.

## A proxy and a direct connection

The same mechanism covers one operator reaching authentik through a proxy while
another reaches it directly. They are separate operators managing separate
objects, so give them separate identities — the URLs differing is not by itself
enough to keep their objects apart, because the objects live in one authentik
either way.

```sh
kubectl apply -k examples/multi-cluster
```
