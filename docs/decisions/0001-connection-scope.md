# 0001 — Ship both a namespaced and a cluster-scoped connection kind

- **Status:** Accepted
- **Date:** <!-- PLACEHOLDER: date the owner accepted this decision -->
- **Deciders:** Project owner
- **Supersedes:** —
- **Superseded by:** —

## Context

Every resource this operator manages — `OAuth2Provider`, `SAMLProvider`,
`ProxyProvider`, `Application`, `Outpost`, `KubernetesServiceConnection`,
`DockerServiceConnection` — needs to know two things before it can reconcile:
which authentik instance to talk to, and which API token to talk to it with.
That pairing is modelled as a separate *connection* object rather than repeated
on every resource, so a token rotation is one edit and a URL change is one edit.

The design question is what scope the connection object has, and the two
plausible answers serve genuinely different deployments:

**The multi-tenant platform.** Several teams share a cluster. Each has its own
namespace, its own authentik service account, and its own token. A team must not
be able to read another team's token, and the platform team does not want to
mediate every provider a tenant creates. Here a connection has to be namespaced,
and its token `Secret` has to be resolved within that namespace.

**The single-operator homelab or single-tenant cluster.** One authentik, one
token, twenty namespaces full of applications. Copying the same
`AuthentikConnection` and the same token `Secret` into every namespace is
duplication that rots: rotating the token means finding and updating every copy,
and a missed copy is a silent reconcile failure in whichever namespace was
forgotten. Here a single cluster-scoped connection is obviously right.

Kubernetes has no scope that is both. The precedent in the ecosystem is to ship
two kinds: cert-manager has `Issuer` and `ClusterIssuer`, external-secrets has
`SecretStore` and `ClusterSecretStore`, Gateway API splits along similar lines.
Users arriving from those projects already have the mental model.

The genuine hazard is the one those projects also carry. A cluster-scoped
connection must name a namespace to read its token `Secret` from, which means
the operator needs cluster-wide `Secret` read permission. If the resolution
rules are sloppy, a tenant in namespace `A` can point a provider at a connection
and cause the operator to read a `Secret` in namespace `B` on their behalf. That
is the classic operator privilege-escalation bug — it is the same shape as the
confused deputy that cert-manager and external-secrets both had to write
explicit rules around, and it is easy to reintroduce by accident when adding a
"convenience" default later.

## Options considered

### Option A — Namespaced only (`AuthentikConnection`)

Ship exactly one kind, namespaced, whose token `Secret` is always in its own
namespace. The operator never needs cluster-wide `Secret` access.

- **For:** The smallest possible blast radius. RBAC is trivially correct: if you
  can create a connection in a namespace, you could already read that
  namespace's `Secret`s, so no privilege is manufactured. No cross-namespace
  resolution code exists, so the confused-deputy bug is structurally impossible.
- **Against:** Punishes the single-tenant case, which is the majority of
  realistic installations. Token rotation becomes an N-namespace operation, and
  users route around it — by syncing `Secret`s with a third-party tool, which
  reintroduces cross-namespace credential flow outside our control and outside
  our audit trail. Making the safe path tedious does not make deployments safer.

### Option B — Cluster-scoped only (`ClusterAuthentikConnection`)

Ship exactly one kind, cluster-scoped, naming its token `Secret`'s namespace.

- **For:** One object, one token, no duplication. Simple for the single-tenant
  case, which is most of them.
- **Against:** Unusable in a multi-tenant cluster. Either the platform team
  becomes a ticket queue for every provider a tenant wants, or tenants get
  permission to create cluster-scoped objects that read arbitrary `Secret`s —
  which is cluster-admin with extra steps. It also forces the operator to hold
  cluster-wide `Secret` read permission in installations that have no need for
  it, which is the wrong default for a security-adjacent component.

### Option C — Both kinds, user's choice (chosen)

Ship `AuthentikConnection` (namespaced) and `ClusterAuthentikConnection`
(cluster-scoped). Every managed resource's `spec.connectionRef` carries a `kind`
so it can select either.

- **For:** Each deployment shape gets the model that fits it. Matches
  established ecosystem precedent, so the semantics are already familiar. The
  namespaced kind can be the documented default, leaving the cluster-scoped kind
  as a deliberate, auditable opt-in.
- **Against:** Two kinds to implement, document, test and keep in sync. The
  cluster-scoped kind carries the dangerous privilege, so its resolution rules
  have to be stated precisely and defended in tests forever.

## Decision

**Ship both.** `AuthentikConnection` is namespaced and is the documented
default. `ClusterAuthentikConnection` is cluster-scoped and exists for
single-tenant clusters that want one connection for the whole cluster.
`spec.connectionRef.kind` selects between them.

The following rules are binding on the implementation and are not negotiable
conveniences:

1. **`AuthentikConnection` resolves its token `Secret` only from its own
   namespace.** `spec.tokenSecretRef` has no `namespace` field. There is no
   mechanism by which a namespaced connection reads a `Secret` elsewhere.

2. **`ClusterAuthentikConnection` resolves its token `Secret` only from the
   namespace named in `spec.tokenSecretRef.namespace`.** That field is
   **required**. It has no default, and in particular it never defaults to the
   namespace of the resource that referenced the connection, nor to the
   operator's own namespace. A connection whose spec does not name a namespace
   is rejected at admission; it does not fall back.

   This is the rule that prevents the privilege escalation. A tenant referencing
   a cluster connection gets exactly the `Secret` the cluster administrator who
   wrote that connection chose — never one selected by the tenant, and never one
   selected implicitly by where the tenant's own resource happens to live.

3. **A namespaced resource may reference a `ClusterAuthentikConnection`, and
   this is a grant, not a leak.** The tenant obtains the *use* of a token whose
   identity and scope a cluster administrator chose; they never obtain the
   token's bytes. The operator does not project the token into the tenant's
   namespace, echo it in status, or log it.

4. **A `ClusterAuthentikConnection` may restrict which namespaces may reference
   it.** An optional `spec.allowedNamespaces` selector, empty meaning all, lets
   an administrator share one connection with a subset of the cluster without
   sharing it with everything. This is defence in depth on top of rule 2, not a
   substitute for it.

5. **Creating a `ClusterAuthentikConnection` is a cluster-admin-level
   privilege** and must be documented as such wherever RBAC for this operator is
   discussed. It is not shipped in any default role bound to non-administrators.

## Consequences

### Positive

- Multi-tenant clusters can use the operator without any cross-namespace
  credential reach: install the namespaced kind, decline to grant the
  cluster-scoped one, and the escalation path does not exist for tenants.
- Single-tenant clusters get one connection object and one token to rotate.
- Rule 2 kills the confused-deputy bug by construction rather than by review
  vigilance. There is no code path that consults the referencing resource's
  namespace when resolving a cluster connection's `Secret`, so a future
  contributor cannot weaken it by omission — only by deliberately adding one,
  which is a reviewable change.
- The `kind` discriminator in `connectionRef` means adding a third connection
  shape later (a token from a workload-identity provider, say) does not break
  existing manifests.

### Negative

- The operator's `ClusterRole` must include `get` on `secrets` cluster-wide
  whenever `ClusterAuthentikConnection` is installed. Installations that do not
  use it should strip that permission, and the Helm chart needs a values flag to
  make that easy.
- Two kinds means duplicated validation, duplicated status handling and
  duplicated tests. Shared internal code will need care to keep the two
  resolution paths genuinely distinct where it matters — a shared resolver
  taking a "namespace" argument is precisely how rule 2 gets accidentally
  softened.
- Users must understand the difference to pick correctly, and the wrong choice
  is silently more dangerous rather than loudly broken. The README and
  `SECURITY.md` both have to carry the warning, and warnings decay.
- `spec.allowedNamespaces` is additional surface that has to be enforced on
  every reconcile, not only at admission, since a namespace's labels can change
  after a reference is established.

### Follow-up work

- Envtest coverage asserting that a cluster connection's `Secret` is **not**
  read from the referencing resource's namespace, including the case where a
  `Secret` of the same name exists in both. This test is the executable form of
  rule 2 and should be treated as load-bearing.
- A Helm values flag to omit `ClusterAuthentikConnection` and its cluster-wide
  `Secret` RBAC entirely.
- Guidance, and ideally a shipped example, for a ValidatingAdmissionPolicy that
  constrains `spec.url` and `spec.tokenSecretRef.namespace` on cluster
  connections.

## References

- [`SECURITY.md`](../../SECURITY.md) — blast radius of each credential and the
  hardening guidance that follows from this decision.
- [ADR 0002](0002-adoption-policy.md) — the other safety default in the API.
- Prior art: cert-manager's `Issuer`/`ClusterIssuer` split, and
  external-secrets' `SecretStore`/`ClusterSecretStore` split.
